package httpapi

// Email-to-task intake: POST /api/intake?token=<intakeToken> (or
// Authorization: Bearer <intakeToken>) with
// {subject, text, date?} creates a task. Any forwarding service (or a local
// MTA pipe) can hit it — a procmail one-liner to curl covers self-hosters.

import (
	"net/http"
	"strings"
	"time"

	"cal/apps/api/internal/store"

	"github.com/gin-gonic/gin"
)

func (s *Server) intake(c *gin.Context) {
	// The intake token can only create tasks here; services that can't set
	// headers pass it in the query.
	token := c.Query("token")
	if h := c.GetHeader("Authorization"); strings.HasPrefix(h, "Bearer ") {
		token = strings.TrimSpace(strings.TrimPrefix(h, "Bearer "))
	}
	user, err := s.store.UserByToken(c.Request.Context(), store.TokenIntake, token)
	if err != nil {
		c.String(http.StatusUnauthorized, "invalid token")
		return
	}
	var body struct {
		Subject string `json:"subject"`
		Text    string `json:"text"`
		Date    string `json:"date"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.String(http.StatusBadRequest, "expected {subject, text?, date?}")
		return
	}
	title := strings.TrimSpace(body.Subject)
	if title == "" {
		title = "(no subject)"
	}
	if len(title) > 200 {
		title = title[:200]
	}
	date := body.Date
	if _, err := time.Parse(time.DateOnly, date); err != nil {
		date = time.Now().Format(time.DateOnly)
	}
	text := strings.TrimSpace(body.Text)
	if len(text) > 4000 {
		text = text[:4000]
	}
	entry, err := s.store.CreateEntry(c.Request.Context(), user.ID, store.EntryInput{
		Title:   title,
		Content: text,
		Type:    "task",
		Date:    date,
		Tags:    []string{"inbox"},
	})
	if err != nil {
		c.String(http.StatusInternalServerError, "create failed")
		return
	}
	c.JSON(http.StatusCreated, entry)
}
