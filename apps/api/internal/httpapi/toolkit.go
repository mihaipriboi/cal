package httpapi

// Toolkit endpoints: focus timer, card activity, trash, board meta/share,
// tags, agenda export.

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"cal/apps/api/internal/store"

	"context"
	"encoding/json"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
	"io"
	"regexp"
	"time"
)

// --- Focus timer (one running per user; solidtime model) ---

func (s *Server) startTimer(c *gin.Context) {
	var body struct {
		EntryID   *string  `json:"entryId"`
		Note      string   `json:"note"`
		Planned   int      `json:"planned"`
		Billable  bool     `json:"billable"`
		Rate      *float64 `json:"rate"`
		ProjectID *string  `json:"projectId"`
		Tags      []string `json:"tags"`
	}
	_ = c.ShouldBindJSON(&body)
	if body.EntryID != nil && *body.EntryID == "" {
		body.EntryID = nil
	}
	if body.ProjectID != nil && *body.ProjectID == "" {
		body.ProjectID = nil
	}
	if body.Rate == nil {
		if st, err := s.store.Settings(c.Request.Context(), currentUser(c).ID); err == nil {
			body.Rate = st.DefaultRate
		}
	}
	t, err := s.store.StartTimer(c.Request.Context(), currentUser(c).ID, body.EntryID, body.Note, body.Planned, body.Billable, body.Rate, body.ProjectID, body.Tags)
	if err != nil {
		c.String(http.StatusConflict, "a timer is already running")
		return
	}
	c.JSON(http.StatusCreated, t)
}

func (s *Server) stopTimer(c *gin.Context) {
	t, err := s.store.StopTimer(c.Request.Context(), currentUser(c).ID)
	if errors.Is(err, pgx.ErrNoRows) {
		c.String(http.StatusNotFound, "no running timer")
		return
	}
	if err != nil {
		c.String(http.StatusInternalServerError, "failed")
		return
	}
	c.JSON(http.StatusOK, t)
}

func (s *Server) currentTimer(c *gin.Context) {
	t, err := s.store.CurrentTimer(c.Request.Context(), currentUser(c).ID)
	if err != nil {
		c.String(http.StatusInternalServerError, "failed")
		return
	}
	if t == nil {
		c.Status(http.StatusNoContent)
		return
	}
	c.JSON(http.StatusOK, t)
}

func (s *Server) timeSummary(c *gin.Context) {
	sum, err := s.store.TimeSummary(c.Request.Context(), currentUser(c).ID)
	if err != nil {
		c.String(http.StatusInternalServerError, "failed")
		return
	}
	c.JSON(http.StatusOK, sum)
}

// timeLog — the timesheet: finished sessions in a range.
func (s *Server) timeLog(c *gin.Context) {
	from := c.DefaultQuery("from", time.Now().AddDate(0, 0, -14).Format("2006-01-02"))
	to := c.DefaultQuery("to", time.Now().Format("2006-01-02"))
	items, err := s.store.TimeLog(c.Request.Context(), currentUser(c).ID, from, to)
	if err != nil {
		c.String(http.StatusInternalServerError, "failed")
		return
	}
	c.JSON(http.StatusOK, items)
}

func (s *Server) deleteTimeEntry(c *gin.Context) {
	if err := s.store.DeleteTimeEntry(c.Request.Context(), currentUser(c).ID, c.Param("id")); errors.Is(err, store.ErrNotFound) {
		c.String(http.StatusNotFound, "not found")
		return
	} else if err != nil {
		c.String(http.StatusInternalServerError, "failed")
		return
	}
	c.Status(http.StatusNoContent)
}

// timeExport — solidtime-compatible CSV/JSON: description, project, start,
// end, duration, billable, rate, amount.
func (s *Server) timeExport(c *gin.Context) {
	from := c.DefaultQuery("from", time.Now().AddDate(0, 0, -90).Format("2006-01-02"))
	to := c.DefaultQuery("to", time.Now().Format("2006-01-02"))
	items, err := s.store.TimeLog(c.Request.Context(), currentUser(c).ID, from, to)
	if err != nil {
		c.String(http.StatusInternalServerError, "failed")
		return
	}
	type row struct {
		Description string   `json:"description"`
		Project     string   `json:"project"`
		Start       string   `json:"start"`
		End         string   `json:"end"`
		Duration    int      `json:"duration"` // seconds
		Billable    bool     `json:"billable"`
		Rate        *float64 `json:"rate,omitempty"`
		Amount      float64  `json:"amount"`
	}
	rows := make([]row, 0, len(items))
	for _, t := range items {
		if t.EndAt == nil {
			continue
		}
		dur := int(t.EndAt.Sub(t.StartAt).Seconds())
		var amount float64
		if t.Billable && t.Rate != nil {
			amount = float64(dur) / 3600 * *t.Rate
		}
		desc := t.Title
		if desc == "" {
			desc = t.Note
		}
		rows = append(rows, row{desc, t.Project, t.StartAt.Format(time.RFC3339), t.EndAt.Format(time.RFC3339), dur, t.Billable, t.Rate, amount})
	}
	if c.DefaultQuery("format", "csv") == "json" {
		c.JSON(http.StatusOK, rows)
		return
	}
	var b strings.Builder
	b.WriteString("description,project,start,end,duration,billable,rate,amount\n")
	for _, r := range rows {
		rate := ""
		if r.Rate != nil {
			rate = strconv.FormatFloat(*r.Rate, 'f', 2, 64)
		}
		b.WriteString(fmt.Sprintf("%q,%q,%s,%s,%d,%t,%s,%.2f\n",
			r.Description, r.Project, r.Start, r.End, r.Duration, r.Billable, rate, r.Amount))
	}
	c.Header("Content-Type", "text/csv; charset=utf-8")
	c.Header("Content-Disposition", `attachment; filename="cal-time.csv"`)
	c.String(http.StatusOK, b.String())
}

// --- Card activity ---

func (s *Server) entryActivity(c *gin.Context) {
	items, err := s.store.EntryActivity(c.Request.Context(), currentUser(c).ID, c.Param("id"))
	if err != nil {
		c.String(http.StatusInternalServerError, "failed")
		return
	}
	c.JSON(http.StatusOK, items)
}

// --- Trash ---

func (s *Server) listTrash(c *gin.Context) {
	items, err := s.store.ListTrash(c.Request.Context(), currentUser(c).ID)
	if err != nil {
		c.String(http.StatusInternalServerError, "failed")
		return
	}
	c.JSON(http.StatusOK, items)
}

func (s *Server) restoreEntry(c *gin.Context) {
	if err := s.store.RestoreEntry(c.Request.Context(), currentUser(c).ID, c.Param("id")); errors.Is(err, store.ErrNotFound) {
		c.String(http.StatusNotFound, "not found")
		return
	} else if err != nil {
		c.String(http.StatusInternalServerError, "failed")
		return
	}
	c.Status(http.StatusNoContent)
}

func (s *Server) purgeEntry(c *gin.Context) {
	if err := s.store.PurgeEntry(c.Request.Context(), currentUser(c).ID, c.Param("id")); errors.Is(err, store.ErrNotFound) {
		c.String(http.StatusNotFound, "not found")
		return
	} else if err != nil {
		c.String(http.StatusInternalServerError, "failed")
		return
	}
	c.Status(http.StatusNoContent)
}

// --- Board meta + public share ---

func (s *Server) updateBoard(c *gin.Context) {
	var body struct {
		Description *string `json:"description"`
		TargetDate  *string `json:"targetDate"`
	}
	_ = c.ShouldBindJSON(&body)
	if err := s.store.SetBoardMeta(c.Request.Context(), currentUser(c).ID, c.Param("id"), body.Description, body.TargetDate); errors.Is(err, store.ErrNotFound) {
		c.String(http.StatusNotFound, "not found")
		return
	} else if err != nil {
		c.String(http.StatusInternalServerError, "failed")
		return
	}
	c.Status(http.StatusNoContent)
}

func (s *Server) shareBoard(c *gin.Context) {
	var body struct {
		On   bool `json:"on"`
		Edit bool `json:"edit"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.String(http.StatusBadRequest, "invalid")
		return
	}
	token, err := s.store.SetBoardShare(c.Request.Context(), currentUser(c).ID, c.Param("id"), body.On, body.Edit)
	if errors.Is(err, store.ErrNotFound) {
		c.String(http.StatusNotFound, "not found")
		return
	} else if err != nil {
		c.String(http.StatusInternalServerError, "failed")
		return
	}
	c.JSON(http.StatusOK, gin.H{"shareToken": token, "edit": body.Edit && body.On})
}

// sharedBoardMove lets a write-tier share link move a card between columns.
// No session — the token is the capability.
func (s *Server) sharedBoardMove(c *gin.Context) {
	userID, board, err := s.store.SharedBoardOwner(c.Request.Context(), c.Param("token"))
	if errors.Is(err, store.ErrNotFound) {
		c.String(http.StatusNotFound, "not found")
		return
	} else if err != nil {
		c.String(http.StatusInternalServerError, "failed")
		return
	}
	if !board.Editable {
		c.String(http.StatusForbidden, "view only")
		return
	}
	var body struct {
		ColumnID *string `json:"columnId"`
		Position float64 `json:"position"`
	}
	if err := c.ShouldBindJSON(&body); err != nil || body.ColumnID == nil {
		c.String(http.StatusBadRequest, "invalid")
		return
	}
	entryID := c.Param("id")
	entry, err := s.store.Entry(c.Request.Context(), userID, entryID)
	if err != nil || entry.BoardID == nil || *entry.BoardID != board.ID {
		c.String(http.StatusNotFound, "card not found")
		return
	}
	if !s.store.ColumnInBoard(c.Request.Context(), userID, board.ID, *body.ColumnID) {
		c.String(http.StatusBadRequest, "column not on this board")
		return
	}
	if err := s.store.MoveCard(c.Request.Context(), userID, entryID, board.ID, body.ColumnID, body.Position); err != nil {
		c.String(http.StatusInternalServerError, "failed")
		return
	}
	// Mirror the authed move: done-ish columns complete, others reopen.
	cols, _ := s.store.BoardColumns(c.Request.Context(), userID, board.ID)
	targetDone, sourceDone := false, false
	for _, col := range cols {
		if col.ID == *body.ColumnID {
			targetDone = doneishColumn(col.Name)
		}
		if entry.ColumnID != nil && col.ID == *entry.ColumnID {
			sourceDone = doneishColumn(col.Name)
		}
	}
	if targetDone != sourceDone {
		done := targetDone
		_, _ = s.store.UpdateEntry(c.Request.Context(), userID, entryID, store.EntryPatch{Completed: &done})
	}
	c.Status(http.StatusNoContent)
}

func (s *Server) serveSharedBoard(c *gin.Context) {
	board, err := s.store.SharedBoard(c.Request.Context(), c.Param("token"))
	if errors.Is(err, store.ErrNotFound) {
		c.String(http.StatusNotFound, "not found")
		return
	} else if err != nil {
		c.String(http.StatusInternalServerError, "failed")
		return
	}
	c.JSON(http.StatusOK, board)
}

// --- Tags ---

func (s *Server) tagCounts(c *gin.Context) {
	tags, err := s.store.TagCounts(c.Request.Context(), currentUser(c).ID)
	if err != nil {
		c.String(http.StatusInternalServerError, "failed")
		return
	}
	c.JSON(http.StatusOK, tags)
}

// --- Agenda export ---

func (s *Server) agendaMarkdown(c *gin.Context) {
	days, _ := strconv.Atoi(c.DefaultQuery("days", "7"))
	if days < 1 || days > 60 {
		days = 7
	}
	from := time.Now().Format("2006-01-02")
	to := time.Now().AddDate(0, 0, days).Format("2006-01-02")
	entries, err := s.store.ListEntriesScoped(c.Request.Context(), currentUser(c).ID, from, to, "", "", true)
	if err != nil {
		c.String(http.StatusInternalServerError, "failed")
		return
	}
	var b strings.Builder
	b.WriteString(fmt.Sprintf("# Agenda — %s → %s\n\n", from, to))
	day := ""
	for _, e := range entries {
		heading := e.Date
		if heading == "" {
			// Undated tasks sort last; only the open ones belong on an agenda.
			if e.Completed {
				continue
			}
			heading = "No date"
		}
		if heading != day {
			day = heading
			b.WriteString(fmt.Sprintf("## %s\n\n", day))
		}
		box := "- "
		if e.Type == "task" {
			if e.Completed {
				box = "- [x] "
			} else {
				box = "- [ ] "
			}
		}
		when := ""
		if e.StartTime != nil && *e.StartTime != "" {
			when = *e.StartTime + " "
		}
		b.WriteString(fmt.Sprintf("%s%s%s\n", box, when, e.Title))
	}
	c.Header("Content-Type", "text/markdown; charset=utf-8")
	c.String(http.StatusOK, b.String())
}

// --- Link enrichment: YouTube oEmbed or generic unfurl, stored on the entry ---

var reYouTube = regexp.MustCompile(`(?:youtube\.com/(?:watch\?[^ ]*v=|shorts/|embed/)|youtu\.be/)([A-Za-z0-9_-]{6,20})`)

type oembedResp struct {
	Title      string `json:"title"`
	AuthorName string `json:"author_name"`
	Thumbnail  string `json:"thumbnail_url"`
}

// enrichLink fills link_desc/link_image/link_favicon/link_video_id on a link
// entry. YouTube URLs go through oEmbed (no API key); everything else unfurls.
func (s *Server) enrichLink(userID, entryID, raw string) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if m := reYouTube.FindStringSubmatch(raw); m != nil {
		vid := m[1]
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet,
			"https://www.youtube.com/oembed?url=https://www.youtube.com/watch?v="+vid+"&format=json", nil)
		if resp, err := unfurlClient.Do(req); err == nil && resp.StatusCode == 200 {
			defer resp.Body.Close()
			var oe oembedResp
			if json.NewDecoder(io.LimitReader(resp.Body, 64*1024)).Decode(&oe) == nil {
				s.store.SetLinkMeta(ctx, userID, entryID, oe.AuthorName, oe.Thumbnail, "", vid, oe.Title)
				return
			}
		}
		// oEmbed failed — still record the video id so the card can build the thumb.
		s.store.SetLinkMeta(ctx, userID, entryID, "", "https://i.ytimg.com/vi/"+vid+"/hqdefault.jpg", "", vid, "")
		return
	}

	p, err := unfurlURL(ctx, raw)
	if err != nil {
		// Record the attempt — link_meta_at tells the UI enrichment finished
		// (with nothing found) instead of waiting forever.
		s.store.SetLinkMeta(ctx, userID, entryID, "", "", "", "", "")
		return
	}
	s.store.SetLinkMeta(ctx, userID, entryID, p.Description, p.Image, p.Favicon, "", p.Title)
}

// --- YouTube search via the user's own Invidious instance ---

// The instance base URL comes from settings.invidious_url - user-configured
// like a CalDAV server address, so private/LAN hosts are allowed (the SSRF
// guard is for attacker-influenced URLs, not the operator's own services).
var ytClient = &http.Client{Timeout: 10 * time.Second}

type ytResult struct {
	VideoID   string `json:"videoId"`
	Title     string `json:"title"`
	Author    string `json:"author"`
	URL       string `json:"url"`
	Thumbnail string `json:"thumbnail"`
	Seconds   int    `json:"seconds"`
	Views     int64  `json:"views"`
}

// invidiousVideo is the subset of /api/v1/search items we consume.
type invidiousVideo struct {
	Type      string `json:"type"`
	VideoID   string `json:"videoId"`
	Title     string `json:"title"`
	Author    string `json:"author"`
	LengthSec int    `json:"lengthSeconds"`
	Views     int64  `json:"viewCount"`
}

func (s *Server) youtubeSearch(c *gin.Context) {
	q := strings.TrimSpace(c.Query("q"))
	if q == "" {
		c.JSON(http.StatusOK, []ytResult{})
		return
	}
	st, err := s.store.Settings(c.Request.Context(), currentUser(c).ID)
	base := strings.TrimRight(strings.TrimSpace(st.InvidiousURL), "/")
	if err != nil || base == "" {
		c.String(http.StatusBadRequest, "no Invidious instance configured - set one in Settings")
		return
	}
	endpoint := base + "/api/v1/search?q=" + url.QueryEscape(q) + "&type=video&page=1"
	req, err := http.NewRequestWithContext(c.Request.Context(), http.MethodGet, endpoint, nil)
	if err != nil {
		c.String(http.StatusBadRequest, "invalid Invidious URL")
		return
	}
	resp, err := ytClient.Do(req)
	if err != nil {
		c.String(http.StatusBadGateway, "Invidious instance unreachable")
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		c.String(http.StatusBadGateway, "Invidious search failed")
		return
	}
	var items []invidiousVideo
	if json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(&items) != nil {
		c.String(http.StatusBadGateway, "unexpected Invidious response")
		return
	}
	out := make([]ytResult, 0, len(items))
	for _, v := range items {
		if v.Type != "video" || v.VideoID == "" {
			continue
		}
		out = append(out, ytResult{
			VideoID:   v.VideoID,
			Title:     v.Title,
			Author:    v.Author,
			URL:       "https://www.youtube.com/watch?v=" + v.VideoID,
			Thumbnail: "https://i.ytimg.com/vi/" + v.VideoID + "/hqdefault.jpg",
			Seconds:   v.LengthSec,
			Views:     v.Views,
		})
		if len(out) == 24 {
			break
		}
	}
	c.JSON(http.StatusOK, out)
}

// globalSearch — one endpoint across entries, files, boards, and people.
func (s *Server) globalSearch(c *gin.Context) {
	q := strings.TrimSpace(c.Query("q"))
	if q == "" {
		c.JSON(http.StatusOK, gin.H{"entries": []any{}, "files": []any{}})
		return
	}
	ctx := c.Request.Context()
	uid := currentUser(c).ID
	entries, err := s.store.GlobalSearch(ctx, uid, q)
	if err != nil {
		c.String(http.StatusInternalServerError, "failed")
		return
	}
	files, _ := s.store.SearchFiles(ctx, uid, q)
	boards, _ := s.store.SearchBoards(ctx, uid, q)
	people, _ := s.store.SearchPeople(ctx, uid, q)
	c.JSON(http.StatusOK, gin.H{"entries": entries, "files": files, "boards": boards, "people": people})
}
