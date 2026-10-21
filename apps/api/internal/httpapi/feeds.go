package httpapi

import (
	"context"
	"errors"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"cal/apps/api/internal/ical"
	"cal/apps/api/internal/rss"
	"cal/apps/api/internal/store"

	"github.com/gin-gonic/gin"
)

// feedEvent is the wire shape for external calendar events: read-only,
// keyed by feed so the client can color them.
type feedEvent struct {
	ID        string  `json:"id"`
	FeedID    string  `json:"feedId"`
	FeedName  string  `json:"feedName"`
	Title     string  `json:"title"`
	Date      string  `json:"date"`
	StartTime *string `json:"startTime,omitempty"`
	EndTime   *string `json:"endTime,omitempty"`
	Color     string  `json:"color"`
	Location  string  `json:"location,omitempty"`
	URL       string  `json:"url,omitempty"`
	Image     string  `json:"image,omitempty"`
	Details   string  `json:"details,omitempty"`
}

// privateFeedClient serves CAL_ALLOW_PRIVATE_FEEDS: same limits as
// unfurlClient, no address guard.
var privateFeedClient = newOutboundClient(8*time.Second, nil)

// fetchICS downloads a feed body — SSRF-guarded like unfurl — and converts
// RSS/Atom documents to ICS so the cache pipeline stays format-agnostic.
func fetchICS(rawURL string) (string, error) {
	u, err := url.Parse(rawURL)
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" {
		return "", errors.New("feed URL must be http(s)")
	}
	// SSRF guard — refused for private/loopback unless opted into for local
	// testing (CAL_ALLOW_PRIVATE_FEEDS=1). Also enforced on every connection,
	// redirects included, by unfurlClient's dialer.
	client := unfurlClient
	if os.Getenv("CAL_ALLOW_PRIVATE_FEEDS") == "" {
		if err := checkURL(u); err != nil {
			return "", errors.New("feed URL not allowed: " + err.Error())
		}
	} else {
		client = privateFeedClient
	}
	resp, err := client.Get(u.String())
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return "", errors.New("feed returned " + resp.Status)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return "", err
	}
	if rss.LooksLikeFeed(string(body)) {
		ics, err := rss.ToICS(string(body))
		if err != nil {
			return "", errors.New("feed not parseable as RSS/Atom")
		}
		return ics, nil
	}
	return string(body), nil
}

func (s *Server) listFeeds(c *gin.Context) {
	feeds, err := s.store.ListFeeds(c.Request.Context(), currentUser(c).ID)
	if err != nil {
		c.String(http.StatusInternalServerError, "failed to list feeds")
		return
	}
	c.JSON(http.StatusOK, feeds)
}

func (s *Server) createFeed(c *gin.Context) {
	var body struct {
		Name  string `json:"name"`
		URL   string `json:"url"`
		Color string `json:"color"`
		Kind  string `json:"kind"` // "calendar" (default) | "links"
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.String(http.StatusBadRequest, "invalid json")
		return
	}
	body.URL = strings.TrimSpace(body.URL)
	if body.Name = strings.TrimSpace(body.Name); body.Name == "" {
		body.Name = "Calendar"
	}
	ics, err := fetchICS(body.URL)
	if err != nil {
		c.String(http.StatusBadGateway, "could not fetch feed: "+err.Error())
		return
	}
	if _, err := ical.Parse(ics); err != nil {
		c.String(http.StatusBadRequest, "not a valid iCalendar feed")
		return
	}
	if body.Color == "" {
		body.Color = "sky"
	}
	if body.Kind != "links" {
		body.Kind = "calendar"
	}
	userID := currentUser(c).ID
	feed, err := s.store.CreateFeed(c.Request.Context(), userID, body.Name, body.URL, body.Color, body.Kind, ics)
	if err != nil {
		c.String(http.StatusInternalServerError, "failed to save feed")
		return
	}
	if feed.Kind == "links" {
		go syncLinkFeed(context.Background(), s.store, userID, feed.ID, ics)
	}
	c.JSON(http.StatusCreated, feed)
}

func (s *Server) deleteFeed(c *gin.Context) {
	if err := s.store.DeleteFeed(c.Request.Context(), currentUser(c).ID, c.Param("id")); errors.Is(err, store.ErrNotFound) {
		c.String(http.StatusNotFound, "feed not found")
		return
	} else if err != nil {
		c.String(http.StatusInternalServerError, "failed to delete feed")
		return
	}
	c.Status(http.StatusNoContent)
}

func (s *Server) refreshFeed(c *gin.Context) {
	user := currentUser(c)
	feedID := c.Param("id")
	feeds, err := s.store.ListFeeds(c.Request.Context(), user.ID)
	if err != nil {
		c.String(http.StatusInternalServerError, "failed to load feeds")
		return
	}
	var target *store.Feed
	for i := range feeds {
		if feeds[i].ID == feedID {
			target = &feeds[i]
		}
	}
	if target == nil {
		c.String(http.StatusNotFound, "feed not found")
		return
	}
	ics, err := fetchICS(target.URL)
	if err != nil {
		c.String(http.StatusBadGateway, "could not fetch feed: "+err.Error())
		return
	}
	if err := s.store.RefreshFeedCache(c.Request.Context(), user.ID, feedID, ics); err != nil {
		c.String(http.StatusInternalServerError, "failed to refresh feed")
		return
	}
	if target.Kind == "links" {
		syncLinkFeed(c.Request.Context(), s.store, user.ID, feedID, ics)
	}
	c.Status(http.StatusNoContent)
}

// syncLinkFeed turns a "links"-kind feed's cached ICS into link entries:
// each unseen item URL becomes a bookmark entry carrying its feed image
// (YouTube channel videos keep their media:thumbnail as the card image).
func syncLinkFeed(ctx context.Context, s *store.Store, userID, feedID, ics string) {
	events, err := ical.Parse(ics)
	if err != nil {
		return
	}
	for _, e := range events {
		url := e.URL
		if url == "" {
			continue
		}
		seen, err := s.LinkEntryExists(ctx, userID, url)
		if err != nil || seen {
			continue
		}
		input := store.EntryInput{
			Title:   strings.TrimSpace(e.Summary),
			Type:    "link",
			LinkURL: url,
			Date:    e.Start.Format("2006-01-02"),
		}
		if input.Title == "" {
			input.Title = url
		}
		entry, err := s.CreateEntry(ctx, userID, input)
		if err != nil {
			continue
		}
		vid := ""
		if m := reYouTube.FindStringSubmatch(url); m != nil {
			vid = m[1]
		}
		image := e.Image
		if image == "" && vid != "" {
			image = "https://i.ytimg.com/vi/" + vid + "/hqdefault.jpg"
		}
		if image != "" || vid != "" {
			s.SetLinkMeta(ctx, userID, entry.ID, "", image, "", vid, "")
		}
	}
}

// feedEvents expands every subscribed feed's cache into occurrences inside
// the requested date range.
func (s *Server) feedEvents(c *gin.Context) {
	from, to := c.Query("from"), c.Query("to")
	if from == "" || to == "" {
		c.String(http.StatusBadRequest, "from and to are required")
		return
	}
	fromT, err1 := time.Parse(time.DateOnly, from)
	toT, err2 := time.Parse(time.DateOnly, to)
	if err1 != nil || err2 != nil {
		c.String(http.StatusBadRequest, "invalid dates")
		return
	}
	caches, err := s.store.FeedCaches(c.Request.Context(), currentUser(c).ID)
	if err != nil {
		c.String(http.StatusInternalServerError, "failed to load feeds")
		return
	}
	out := []feedEvent{}
	for feed, ics := range caches {
		if feed.Kind == "links" {
			continue // items live as link entries, not calendar events
		}
		events, err := ical.Parse(ics)
		if err != nil {
			continue
		}
		for _, e := range ical.Expand(events, fromT, toT.AddDate(0, 0, 1)) {
			fe := feedEvent{
				ID:       feed.ID + ":" + e.UID + ":" + e.Start.Format("20060102T150405"),
				FeedID:   feed.ID,
				FeedName: feed.Name,
				Title:    e.Summary,
				Date:     e.Start.Format("2006-01-02"),
				Color:    feed.Color,
				Location: e.Location,
				URL:      e.URL,
				Image:    e.Image,
				Details:  e.Description,
			}
			if !e.AllDay {
				start := e.Start.Format("15:04")
				end := e.End.Format("15:04")
				fe.StartTime = &start
				fe.EndTime = &end
			}
			out = append(out, fe)
		}
	}
	c.JSON(http.StatusOK, out)
}

// importICS accepts a raw .ics body and converts each VEVENT into an entry.
// Imported events keep their date/times and become non-completable "event"
// entries; recurring rules flatten to their occurrences within ±1 year.
func (s *Server) importICS(c *gin.Context) {
	body, err := io.ReadAll(io.LimitReader(c.Request.Body, 4<<20))
	if err != nil {
		c.String(http.StatusBadRequest, "empty body")
		return
	}
	events, err := ical.Parse(string(body))
	if err != nil || len(events) == 0 {
		c.String(http.StatusBadRequest, "no events found in file")
		return
	}
	now := time.Now()
	flat := ical.Expand(events, now.AddDate(-1, 0, 0), now.AddDate(1, 0, 0))
	user := currentUser(c)
	created := 0
	for _, e := range flat {
		input := store.EntryInput{
			Title:   e.Summary,
			Type:    "event",
			Content: strings.TrimSpace(e.Description + "\n" + e.Location),
			Date:    e.Start.Format("2006-01-02"),
		}
		if !e.AllDay {
			input.StartTime = e.Start.Format("15:04")
			input.EndTime = e.End.Format("15:04")
		}
		if e.AlarmMin != nil {
			input.Remind = e.AlarmMin
		}
		if input.Title = strings.TrimSpace(input.Title); input.Title == "" {
			input.Title = "Untitled event"
		}
		if _, err := s.store.CreateEntry(c.Request.Context(), user.ID, input); err != nil {
			log.Printf("import: %v", err)
			continue
		}
		created++
	}
	c.JSON(http.StatusOK, gin.H{"imported": created})
}

// widgetToday is the token-gated read used by the embeddable Today widget.
func (s *Server) widgetToday(c *gin.Context) {
	user, err := s.store.UserByToken(c.Request.Context(), store.TokenWidget, c.Query("token"))
	if errors.Is(err, store.ErrNotFound) {
		c.String(http.StatusUnauthorized, "invalid widget token")
		return
	} else if err != nil {
		c.String(http.StatusInternalServerError, "failed")
		return
	}
	today := time.Now().Format(time.DateOnly)
	entries, err := s.store.ListEntries(c.Request.Context(), user.ID, today, today, "")
	if err != nil {
		c.String(http.StatusInternalServerError, "failed")
		return
	}
	c.JSON(http.StatusOK, entries)
}

// rotateToken replaces one capability token (store.Token*), answering
// {<field>: newToken}.
func (s *Server) rotateToken(column, field string) gin.HandlerFunc {
	return func(c *gin.Context) {
		token, err := s.store.RegenerateToken(c.Request.Context(), currentUser(c).ID, column)
		if err != nil {
			c.String(http.StatusInternalServerError, "failed")
			return
		}
		c.JSON(http.StatusOK, gin.H{field: token})
	}
}

// RefreshFeedsLoop re-fetches every subscribed feed on an interval so the
// calendar stays current without manual refreshes. Failures are logged and
// skipped — a dead feed keeps its last cache.
func RefreshFeedsLoop(ctx context.Context, s *store.Store, every time.Duration) {
	tick := func() {
		feeds, err := s.AllFeeds(ctx)
		if err != nil {
			log.Printf("feeds sync: %v", err)
			return
		}
		for key, feed := range feeds {
			userID := key[:strings.IndexByte(key, '/')]
			ics, err := fetchICS(feed.URL)
			if err != nil {
				continue
			}
			if err := s.RefreshFeedCache(ctx, userID, feed.ID, ics); err != nil {
				log.Printf("feeds sync %s: %v", feed.Name, err)
				continue
			}
			if feed.Kind == "links" {
				syncLinkFeed(ctx, s, userID, feed.ID, ics)
			}
		}
	}
	tick()
	ticker := time.NewTicker(every)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			tick()
		}
	}
}
