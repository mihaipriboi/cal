package httpapi

// Google Calendar OAuth connect: consent URL → callback → refresh token →
// events sync into the feed pipeline (events cached as ICS, same rendering).
// Env: GOOGLE_CLIENT_ID + GOOGLE_CLIENT_SECRET. Without them the endpoints
// report not-configured rather than failing obscurely.

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"cal/apps/api/internal/ical"
	"cal/apps/api/internal/store"

	"github.com/gin-gonic/gin"
)

var gcalClient = &http.Client{Timeout: 15 * time.Second}

func googleCreds() (string, string, bool) {
	id, secret := os.Getenv("GOOGLE_CLIENT_ID"), os.Getenv("GOOGLE_CLIENT_SECRET")
	return id, secret, id != "" && secret != ""
}

// oauthState signs userID so the callback can trust it without a session
// (Google's redirect is a top-level GET that carries no cookie context we can
// rely on across browsers).
func (s *Server) oauthState(ctx context.Context, userID string) (string, error) {
	key, err := s.store.Encrypt(ctx, "state") // reuse the crypto key's persistence
	if err != nil {
		return "", err
	}
	mac := hmac.New(sha256.New, []byte(key))
	mac.Write([]byte(userID))
	return userID + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil)), nil
}

func (s *Server) verifyState(ctx context.Context, state string) (string, bool) {
	i := strings.LastIndex(state, ".")
	if i < 0 {
		return "", false
	}
	userID := state[:i]
	want, err := s.oauthState(ctx, userID)
	return userID, err == nil && hmac.Equal([]byte(want), []byte(state))
}

// googleConnect returns the consent URL for the signed-in user.
func (s *Server) googleConnect(c *gin.Context) {
	id, _, ok := googleCreds()
	if !ok {
		c.String(http.StatusNotImplemented, "set GOOGLE_CLIENT_ID and GOOGLE_CLIENT_SECRET to enable Google sync")
		return
	}
	state, err := s.oauthState(c.Request.Context(), currentUser(c).ID)
	if err != nil {
		c.String(http.StatusInternalServerError, "failed")
		return
	}
	redirect := c.Query("redirect")
	if redirect == "" {
		redirect = apiOrigin(c) + "/api/google/callback"
	}
	q := url.Values{
		"client_id":     {id},
		"redirect_uri":  {redirect},
		"response_type": {"code"},
		"scope":         {"https://www.googleapis.com/auth/calendar.readonly"},
		"access_type":   {"offline"},
		"prompt":        {"consent"},
		"state":         {state},
	}
	c.JSON(http.StatusOK, gin.H{"url": "https://accounts.google.com/o/oauth2/v2/auth?" + q.Encode()})
}

// apiOrigin derives the public API origin from the request (reverse proxies
// set X-Forwarded-Proto/Host; dev hits localhost directly).
func apiOrigin(c *gin.Context) string {
	proto := c.GetHeader("X-Forwarded-Proto")
	if proto == "" {
		proto = "http"
		if c.Request.TLS != nil {
			proto = "https"
		}
	}
	host := c.GetHeader("X-Forwarded-Host")
	if host == "" {
		host = c.Request.Host
	}
	return proto + "://" + host
}

// googleCallback completes the OAuth exchange, stores tokens, kicks a sync.
func (s *Server) googleCallback(c *gin.Context) {
	if _, _, ok := googleCreds(); !ok {
		c.String(http.StatusNotImplemented, "google not configured")
		return
	}
	userID, valid := s.verifyState(c.Request.Context(), c.Query("state"))
	if !valid {
		c.String(http.StatusForbidden, "invalid state")
		return
	}
	code := c.Query("code")
	if code == "" {
		c.String(http.StatusBadRequest, "code required")
		return
	}
	if err := s.googleExchange(c.Request.Context(), userID, code, apiOrigin(c)+"/api/google/callback"); err != nil {
		c.String(http.StatusBadGateway, "token exchange failed: "+err.Error())
		return
	}
	go func() { _ = s.syncGoogle(context.Background(), userID) }()
	c.Redirect(http.StatusFound, os.Getenv("WEB_ORIGIN")+"/settings?google=connected")
}

func (s *Server) googleExchange(ctx context.Context, userID, code, redirect string) error {
	id, secret, _ := googleCreds()
	form := url.Values{
		"code":          {code},
		"client_id":     {id},
		"client_secret": {secret},
		"redirect_uri":  {redirect},
		"grant_type":    {"authorization_code"},
	}
	var tok struct {
		Access  string `json:"access_token"`
		Refresh string `json:"refresh_token"`
		Expires int    `json:"expires_in"`
	}
	if err := gcalPost(ctx, "https://oauth2.googleapis.com/token", form, &tok); err != nil {
		return err
	}
	if tok.Refresh == "" {
		return fmt.Errorf("no refresh token (revoke access in Google and reconnect)")
	}
	// Ensure a feed row exists for the synced events.
	feedID, err := s.ensureGoogleFeed(ctx, userID)
	if err != nil {
		return err
	}
	refreshEnc, _ := s.store.Encrypt(ctx, tok.Refresh)
	accessEnc, _ := s.store.Encrypt(ctx, tok.Access)
	return s.store.UpsertGoogleToken(ctx, userID, feedID, refreshEnc, accessEnc, time.Now().Add(time.Duration(tok.Expires)*time.Second))
}

func (s *Server) ensureGoogleFeed(ctx context.Context, userID string) (string, error) {
	if t, err := s.store.GoogleToken(ctx, userID); err == nil && t.FeedID != "" {
		return t.FeedID, nil
	}
	feeds, err := s.store.ListFeeds(ctx, userID)
	if err != nil {
		return "", err
	}
	for _, f := range feeds {
		if f.URL == "google:primary" {
			return f.ID, nil
		}
	}
	feed, err := s.store.CreateFeed(ctx, userID, "Google", "google:primary", "iris", "calendar", "")
	if err != nil {
		return "", err
	}
	return feed.ID, nil
}

// syncGoogle refreshes the access token if needed, fetches the events window,
// converts to ICS, and writes the feed cache.
func (s *Server) syncGoogle(ctx context.Context, userID string) error {
	tok, err := s.store.GoogleToken(ctx, userID)
	if err != nil {
		return err
	}
	access, err := s.store.Decrypt(ctx, tok.AccessEnc)
	if err != nil {
		return err
	}
	if time.Now().After(tok.AccessExpires.Add(-time.Minute)) {
		refresh, err := s.store.Decrypt(ctx, tok.RefreshEnc)
		if err != nil {
			return err
		}
		id, secret, _ := googleCreds()
		var out struct {
			Access  string `json:"access_token"`
			Expires int    `json:"expires_in"`
		}
		if err := gcalPost(ctx, "https://oauth2.googleapis.com/token", url.Values{
			"grant_type": {"refresh_token"}, "refresh_token": {refresh},
			"client_id": {id}, "client_secret": {secret},
		}, &out); err != nil {
			return fmt.Errorf("refresh: %w", err)
		}
		access = out.Access
		enc, _ := s.store.Encrypt(ctx, access)
		_ = s.store.UpdateGoogleAccess(ctx, userID, enc, time.Now().Add(time.Duration(out.Expires)*time.Second))
	}
	ics, err := fetchGoogleICS(ctx, access, tok.CalendarID)
	if err != nil {
		return err
	}
	return s.store.RefreshFeedCache(ctx, userID, tok.FeedID, ics)
}

// fetchGoogleICS pulls a ±window of events and serialises them as ICS.
func fetchGoogleICS(ctx context.Context, access, calendarID string) (string, error) {
	now := time.Now()
	q := url.Values{
		"timeMin":      {now.AddDate(0, -2, 0).UTC().Format(time.RFC3339)},
		"timeMax":      {now.AddDate(0, 4, 0).UTC().Format(time.RFC3339)},
		"singleEvents": {"true"},
		"maxResults":   {"2500"},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		"https://www.googleapis.com/calendar/v3/calendars/"+url.PathEscape(calendarID)+"/events?"+q.Encode(), nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+access)
	resp, err := gcalClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("events: %d", resp.StatusCode)
	}
	var data struct {
		Items []gcalItem `json:"items"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 32<<20)).Decode(&data); err != nil {
		return "", err
	}
	return ical.EncodeCalendar(gcalEvents(data.Items)), nil
}

type gcalTime struct {
	Date     string `json:"date"`
	DateTime string `json:"dateTime"`
	TimeZone string `json:"timeZone"`
}

type gcalItem struct {
	ID          string   `json:"id"`
	Summary     string   `json:"summary"`
	Description string   `json:"description"`
	Location    string   `json:"location"`
	Status      string   `json:"status"`
	Start       gcalTime `json:"start"`
	End         gcalTime `json:"end"`
}

// gcalEvents converts Google Calendar API events. A timed event's dateTime
// carries its UTC offset, so it's an exact instant; an all-day event is a
// date with an exclusive end date, as in iCalendar.
func gcalEvents(items []gcalItem) []ical.Event {
	var events []ical.Event
	for _, it := range items {
		if it.Status == "cancelled" {
			continue
		}
		ev := ical.Event{UID: it.ID + "@google", Summary: it.Summary, Description: it.Description, Location: it.Location}
		if it.Start.Date != "" { // all-day: Google end.date is exclusive
			start, err := time.Parse(time.DateOnly, it.Start.Date)
			if err != nil {
				continue
			}
			end, err := time.Parse(time.DateOnly, it.End.Date)
			if err != nil {
				end = start.AddDate(0, 0, 1)
			}
			ev.AllDay = true
			ev.Start = start
			ev.End = end
		} else {
			start, err := time.Parse(time.RFC3339, it.Start.DateTime)
			if err != nil {
				continue
			}
			end, err2 := time.Parse(time.RFC3339, it.End.DateTime)
			if err2 != nil {
				end = start.Add(time.Hour)
			}
			ev.Start = start
			ev.End = end
		}
		events = append(events, ev)
	}
	return events
}

func gcalPost(ctx context.Context, endpoint string, form url.Values, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := gcalClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("%d: %s", resp.StatusCode, body)
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

// GoogleSyncLoop re-syncs every connected Google account on an interval.
func GoogleSyncLoop(ctx context.Context, s *store.Store, every time.Duration) {
	server := &Server{store: s}
	sync := func() {
		tokens, err := s.AllGoogleTokens(ctx)
		if err != nil {
			return
		}
		for _, t := range tokens {
			if err := server.syncGoogle(ctx, t.UserID); err != nil {
				// log only — a revoked token shouldn't stall the loop
			}
		}
	}
	sync()
	ticker := time.NewTicker(every)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			sync()
		}
	}
}

// googleStatus reports whether the user has a connected Google account.
func (s *Server) googleStatus(c *gin.Context) {
	tok, err := s.store.GoogleToken(c.Request.Context(), currentUser(c).ID)
	c.JSON(http.StatusOK, gin.H{"connected": err == nil && tok.RefreshEnc != ""})
}

// googleSyncNow forces a sync for the signed-in user.
func (s *Server) googleSyncNow(c *gin.Context) {
	if err := s.syncGoogle(c.Request.Context(), currentUser(c).ID); err != nil {
		c.String(http.StatusBadGateway, "sync failed: "+err.Error())
		return
	}
	c.Status(http.StatusNoContent)
}

// googleDisconnect removes tokens + the synthetic feed.
func (s *Server) googleDisconnect(c *gin.Context) {
	userID := currentUser(c).ID
	if tok, err := s.store.GoogleToken(c.Request.Context(), userID); err == nil && tok.FeedID != "" {
		_ = s.store.DeleteFeed(c.Request.Context(), userID, tok.FeedID)
	}
	_ = s.store.DeleteGoogleToken(c.Request.Context(), userID)
	c.Status(http.StatusNoContent)
}
