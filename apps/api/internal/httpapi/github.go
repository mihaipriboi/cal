package httpapi

// GitHub: PAT in settings → inbox of open issues/PRs involving you,
// issue→card import, contribution counts. No OAuth app needed for
// personal use — a fine-grained PAT covers it.

import (
	"cal/apps/api/internal/store"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

// ghAPI is a var so tests can point it at a stub.
var ghAPI = "https://api.github.com"

func ghClient() *http.Client { return &http.Client{Timeout: 10 * time.Second} }

func ghGet(ctx context.Context, token, path string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, ghAPI+path, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	resp, err := ghClient().Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return fmt.Errorf("github %d", resp.StatusCode)
	}
	return json.NewDecoder(io.LimitReader(resp.Body, 4*1024*1024)).Decode(out)
}

func (s *Server) ghToken(c *gin.Context) (string, bool) {
	token, err := s.store.GitHubToken(c.Request.Context(), currentUser(c).ID)
	if err != nil || token == "" {
		c.String(http.StatusNotImplemented, "set a GitHub PAT in Settings first")
		return "", false
	}
	return token, true
}

// setGitHubToken stores (PUT) or removes (DELETE) the user's GitHub token.
// It's write-only: settings responses only say whether one is set.
func (s *Server) setGitHubToken(c *gin.Context) {
	var body struct {
		Token string `json:"token"`
	}
	if c.Request.Method == http.MethodPut {
		if err := c.ShouldBindJSON(&body); err != nil {
			c.String(http.StatusBadRequest, "invalid json")
			return
		}
		body.Token = strings.TrimSpace(body.Token)
		if body.Token == "" || len(body.Token) > 255 || strings.ContainsFunc(body.Token, func(r rune) bool { return r <= ' ' || r == 0x7f }) {
			c.String(http.StatusBadRequest, "invalid token")
			return
		}
	}
	if err := s.store.SetGitHubToken(c.Request.Context(), currentUser(c).ID, body.Token); err != nil {
		c.String(http.StatusInternalServerError, "failed to save token")
		return
	}
	c.JSON(http.StatusOK, gin.H{"githubConnected": body.Token != ""})
}

type ghIssue struct {
	Number  int       `json:"number"`
	Title   string    `json:"title"`
	State   string    `json:"state"`
	HTMLURL string    `json:"html_url"`
	RepoURL string    `json:"repository_url"`
	IsPR    bool      `json:"-"`
	PR      *struct{} `json:"pull_request"`
	Labels  []struct {
		Name string `json:"name"`
	} `json:"labels"`
	Updated string `json:"updated_at"`
}

// GET /github/inbox — open issues + PRs that involve you.
func (s *Server) githubInbox(c *gin.Context) {
	token, ok := s.ghToken(c)
	if !ok {
		return
	}
	var res struct {
		Items []ghIssue `json:"items"`
	}
	if err := ghGet(c.Request.Context(), token, "/search/issues?q="+url.QueryEscape("is:open involves:@me")+"&per_page=50", &res); err != nil {
		c.String(http.StatusBadGateway, "github fetch failed")
		return
	}
	out := []gin.H{}
	for _, it := range res.Items {
		repo := ""
		if m := regexp.MustCompile(`/repos/(.+)$`).FindStringSubmatch(it.RepoURL); m != nil {
			repo = m[1]
		}
		labels := []string{}
		for _, l := range it.Labels {
			labels = append(labels, l.Name)
		}
		out = append(out, gin.H{
			"number": it.Number, "title": it.Title, "state": it.State,
			"url": it.HTMLURL, "repo": repo, "isPR": it.PR != nil,
			"labels": labels, "updated": it.Updated,
		})
	}
	c.JSON(http.StatusOK, out)
}

// GET /github/activity — your contribution count this week (events API).
func (s *Server) githubActivity(c *gin.Context) {
	token, ok := s.ghToken(c)
	if !ok {
		return
	}
	var user struct {
		Login string `json:"login"`
	}
	if err := ghGet(c.Request.Context(), token, "/user", &user); err != nil {
		c.String(http.StatusBadGateway, "github fetch failed")
		return
	}
	var events []struct {
		Type      string `json:"type"`
		CreatedAt string `json:"created_at"`
	}
	if err := ghGet(c.Request.Context(), token, "/users/"+user.Login+"/events?per_page=100", &events); err != nil {
		c.String(http.StatusBadGateway, "github fetch failed")
		return
	}
	week := 0
	cut := time.Now().AddDate(0, 0, -7)
	for _, e := range events {
		if t, err := time.Parse(time.RFC3339, e.CreatedAt); err == nil && t.After(cut) {
			week++
		}
	}
	c.JSON(http.StatusOK, gin.H{"login": user.Login, "eventsThisWeek": week})
}

var reGHIssue = regexp.MustCompile(`github\.com/([^/]+)/([^/]+)/(?:issues|pull)/(\d+)`)

// POST /github/import {url, boardId, columnId} — an issue/PR becomes a card.
func (s *Server) githubImport(c *gin.Context) {
	token, ok := s.ghToken(c)
	if !ok {
		return
	}
	var body struct {
		URL      string  `json:"url"`
		BoardID  *string `json:"boardId"`
		ColumnID *string `json:"columnId"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.String(http.StatusBadRequest, "invalid")
		return
	}
	m := reGHIssue.FindStringSubmatch(body.URL)
	if m == nil {
		c.String(http.StatusBadRequest, "not a github issue/PR url")
		return
	}
	owner, repo, num := m[1], m[2], m[3]
	isPR := strings.Contains(body.URL, "/pull/")
	path := fmt.Sprintf("/repos/%s/%s/%s/%s", owner, repo, map[bool]string{true: "pulls", false: "issues"}[isPR], num)
	var it struct {
		Title string `json:"title"`
		State string `json:"state"`
	}
	if err := ghGet(c.Request.Context(), token, path, &it); err != nil {
		c.String(http.StatusBadGateway, "github fetch failed")
		return
	}
	input := store.EntryInput{
		Title:    fmt.Sprintf("%s/%s #%s — %s", owner, repo, num, it.Title),
		Type:     "task",
		LinkURL:  body.URL,
		Date:     todayIn(s.userZone(c)),
		Tags:     []string{"github", owner + "/" + repo},
		BoardID:  body.BoardID,
		ColumnID: body.ColumnID,
	}
	entry, err := s.store.CreateEntry(c.Request.Context(), currentUser(c).ID, input)
	if err != nil {
		c.String(http.StatusInternalServerError, "create failed")
		return
	}
	if it.State == "closed" {
		done := true
		_, _ = s.store.UpdateEntry(c.Request.Context(), currentUser(c).ID, entry.ID, store.EntryPatch{Completed: &done})
	}
	c.JSON(http.StatusCreated, entry)
}

// ghCloseIssue closes a GitHub issue/PR-linked card's remote counterpart.
func ghCloseIssue(ctx context.Context, token, issueURL string) {
	m := reGHIssue.FindStringSubmatch(issueURL)
	if m == nil || strings.Contains(issueURL, "/pull/") {
		return // PRs aren't closed this way — they merge
	}
	body := strings.NewReader(`{"state":"closed"}`)
	req, err := http.NewRequestWithContext(ctx, http.MethodPatch,
		fmt.Sprintf("%s/repos/%s/%s/issues/%s", ghAPI, m[1], m[2], m[3]), body)
	if err != nil {
		return
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("Content-Type", "application/json")
	resp, err := ghClient().Do(req)
	if err == nil {
		resp.Body.Close()
	}
}

// ghIssueState reads one issue's state for the sync poll.
func ghIssueState(ctx context.Context, token, issueURL string) (string, error) {
	m := reGHIssue.FindStringSubmatch(issueURL)
	if m == nil {
		return "", nil
	}
	var it struct {
		State string `json:"state"`
	}
	kind := "issues"
	if strings.Contains(issueURL, "/pull/") {
		kind = "pulls"
	}
	err := ghGet(ctx, token, fmt.Sprintf("/repos/%s/%s/%s/%s", m[1], m[2], kind, m[3]), &it)
	return it.State, err
}

// GitHubSyncLoop — every 15 min, cards linked to github issues that were
// closed remotely get completed + moved to a done-ish column.
func GitHubSyncLoop(ctx context.Context, s *store.Store, every time.Duration) {
	tick := func() {
		users, err := s.GitHubUsers(ctx)
		if err != nil {
			return
		}
		for _, u := range users {
			syncGitHub(ctx, s, u.UserID, u.Token)
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

func syncGitHub(ctx context.Context, s *store.Store, userID, token string) {
	cards, err := s.GitHubLinkedCards(ctx, userID)
	if err != nil {
		return
	}
	for _, e := range cards {
		if e.Completed {
			continue
		}
		state, err := ghIssueState(ctx, token, e.LinkURL)
		if err != nil || state != "closed" {
			continue
		}
		done := true
		updated, err := s.UpdateEntry(ctx, userID, e.ID, store.EntryPatch{Completed: &done})
		if err != nil {
			continue
		}
		s.LogActivity(ctx, userID, e.ID, "completed", "issue closed on github")
		// Move to a done-ish column if on a board.
		if updated.BoardID != nil {
			cols, err := s.BoardColumns(ctx, userID, *updated.BoardID)
			if err == nil {
				for _, col := range cols {
					if doneishColumn(col.Name) {
						_ = s.MoveCard(ctx, userID, e.ID, *updated.BoardID, &col.ID, 1e9)
						break
					}
				}
			}
		}
	}
}
