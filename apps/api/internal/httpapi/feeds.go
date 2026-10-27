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
	"unicode/utf8"

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
	EndDate   string  `json:"endDate,omitempty"` // last day, only when after Date
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
	userID := currentUser(c).ID
	if body.Color == "" {
		existing, _ := s.store.ListFeeds(c.Request.Context(), userID)
		body.Color = nextFeedColor(existing)
	} else if !validColor(body.Color) {
		c.String(http.StatusBadRequest, "unknown color")
		return
	}
	if body.Kind != "links" {
		body.Kind = "calendar"
	}
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

// updateFeed renames and/or recolours a feed. Only name and color are
// accepted; the URL can't change (delete and re-add, so every source goes
// through createFeed's checks).
func (s *Server) updateFeed(c *gin.Context) {
	var body struct {
		Name  *string `json:"name"`
		Color *string `json:"color"`
	}
	if !bind(c, &body) {
		c.String(http.StatusBadRequest, "only name and color can be changed")
		return
	}
	if body.Name != nil {
		name := strings.TrimSpace(*body.Name)
		if name == "" || utf8.RuneCountInString(name) > 80 {
			c.String(http.StatusBadRequest, "name must be 1-80 characters")
			return
		}
		body.Name = &name
	}
	if body.Color != nil && !validColor(*body.Color) {
		c.String(http.StatusBadRequest, "unknown color")
		return
	}
	feed, err := s.store.UpdateFeed(c.Request.Context(), currentUser(c).ID, c.Param("id"), body.Name, body.Color)
	if errors.Is(err, store.ErrNotFound) {
		c.String(http.StatusNotFound, "feed not found")
		return
	}
	if err != nil {
		c.String(http.StatusInternalServerError, "failed to update feed")
		return
	}
	c.JSON(http.StatusOK, feed)
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
	loc := s.UserLocation(ctx, userID)
	events, err := ical.ParseIn(ics, loc)
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
			Date:    e.Start.In(loc).Format(time.DateOnly),
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
// the requested date range. The range and each occurrence's date and times
// are in the user's time zone.
func (s *Server) feedEvents(c *gin.Context) {
	from, to := c.Query("from"), c.Query("to")
	if from == "" || to == "" {
		c.String(http.StatusBadRequest, "from and to are required")
		return
	}
	loc := s.userZone(c)
	fromT, err1 := time.ParseInLocation(time.DateOnly, from, loc)
	toT, err2 := time.ParseInLocation(time.DateOnly, to, loc)
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
		events, err := ical.ParseIn(ics, loc)
		if err != nil {
			continue
		}
		for _, e := range ical.Expand(events, fromT, toT.AddDate(0, 0, 1)) {
			if !e.AllDay {
				e.Start, e.End = e.Start.In(loc), e.End.In(loc)
			}
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
				EndDate:  feedEndDate(e),
			}
			if !e.AllDay {
				start := e.Start.Format("15:04")
				end := e.End.In(e.Start.Location()).Format("15:04")
				fe.StartTime = &start
				fe.EndTime = &end
			}
			out = append(out, fe)
		}
	}
	c.JSON(http.StatusOK, out)
}

// feedEndDate is the last day an event covers, when that's after its start
// day. An all-day DTEND is exclusive, and a timed event that ends at
// midnight doesn't cover the day it ends on.
func feedEndDate(e ical.Event) string {
	end := e.End.In(e.Start.Location())
	if e.AllDay {
		end = end.AddDate(0, 0, -1)
	} else {
		end = end.Add(-time.Nanosecond)
	}
	last := end.Format(time.DateOnly)
	if last <= e.Start.Format(time.DateOnly) {
		return ""
	}
	return last
}

// importICS accepts a raw .ics body and converts each VEVENT into an entry.
// Imported events become non-completable "event" entries with their date and
// times in the user's time zone; recurring rules flatten to their occurrences
// within ±1 year.
func (s *Server) importICS(c *gin.Context) {
	body, err := io.ReadAll(io.LimitReader(c.Request.Body, 4<<20))
	if err != nil {
		c.String(http.StatusBadRequest, "empty body")
		return
	}
	loc := s.userZone(c)
	events, err := ical.ParseIn(string(body), loc)
	if err != nil || len(events) == 0 {
		c.String(http.StatusBadRequest, "no events found in file")
		return
	}
	now := time.Now()
	flat := ical.Expand(events, now.AddDate(-1, 0, 0), now.AddDate(1, 0, 0))
	user := currentUser(c)
	created, guessed := 0, 0
	for _, e := range flat {
		input := store.EntryInput{
			Title:   e.Summary,
			Type:    "event",
			Content: strings.TrimSpace(e.Description + "\n" + e.Location),
		}
		input.Date, input.StartTime, input.EndTime = ical.LocalSlot(e, loc)
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
		if e.UnknownTZID != "" {
			guessed++
		}
	}
	// zoneGuessed counts events whose time zone wasn't recognised, so their
	// times were read in the user's zone.
	c.JSON(http.StatusOK, gin.H{"imported": created, "zoneGuessed": guessed})
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
	today := todayIn(s.store.UserLocation(c.Request.Context(), user.ID))
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
