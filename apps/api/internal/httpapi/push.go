package httpapi

// Web push: VAPID keys are generated once and persisted in server_config so
// subscriptions survive restarts. A 60s scheduler finds due reminders and
// pushes to every subscription of the entry's owner.

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"cal/apps/api/internal/store"

	webpush "github.com/SherClockHolmes/webpush-go"
	"github.com/gin-gonic/gin"
)

// vapidKeys loads or generates the server-wide VAPID keypair.
func vapidKeys(ctx context.Context, s *store.Store) (pub, priv string, err error) {
	pub, err = s.ServerConfig(ctx, "vapid_public")
	if err == nil {
		priv, err = s.ServerConfig(ctx, "vapid_private")
		return pub, priv, err
	}
	priv, pub, err = webpush.GenerateVAPIDKeys()
	if err != nil {
		return "", "", err
	}
	if err := s.SetServerConfig(ctx, "vapid_public", pub); err != nil {
		return "", "", err
	}
	if err := s.SetServerConfig(ctx, "vapid_private", priv); err != nil {
		return "", "", err
	}
	return pub, priv, nil
}

func (s *Server) pushVapid(c *gin.Context) {
	pub, _, err := vapidKeys(c.Request.Context(), s.store)
	if err != nil {
		c.String(http.StatusInternalServerError, "push not available")
		return
	}
	c.JSON(http.StatusOK, gin.H{"publicKey": pub})
}

func (s *Server) subscribePush(c *gin.Context) {
	var body struct {
		Endpoint string `json:"endpoint"`
		Label    string `json:"label"`
		Keys     struct {
			P256dh string `json:"p256dh"`
			Auth   string `json:"auth"`
		} `json:"keys"`
	}
	if err := c.ShouldBindJSON(&body); err != nil || body.Endpoint == "" || body.Keys.P256dh == "" {
		c.String(http.StatusBadRequest, "invalid subscription")
		return
	}
	label := body.Label
	if label == "" {
		label = deviceLabel(c.GetHeader("User-Agent"))
	}
	if err := s.store.UpsertPushSub(c.Request.Context(), currentUser(c).ID, body.Endpoint, body.Keys.P256dh, body.Keys.Auth, label); err != nil {
		c.String(http.StatusInternalServerError, "failed to save subscription")
		return
	}
	c.Status(http.StatusNoContent)
}

// pushSubscriptions lists the user's devices (label + endpoint tail).
func (s *Server) pushSubscriptions(c *gin.Context) {
	subs, err := s.store.PushSubs(c.Request.Context(), currentUser(c).ID)
	if err != nil {
		c.String(http.StatusInternalServerError, "failed")
		return
	}
	type out struct {
		ID       string `json:"id"`
		Label    string `json:"label"`
		Endpoint string `json:"endpoint"`
	}
	list := make([]out, 0, len(subs))
	for _, sub := range subs {
		ep := sub.Endpoint
		if len(ep) > 60 {
			ep = ep[:57] + "…"
		}
		list = append(list, out{ID: sub.ID, Label: sub.Label, Endpoint: ep})
	}
	c.JSON(http.StatusOK, list)
}

// deletePushSubscription removes one device by id (ownership-checked).
func (s *Server) deletePushSubscription(c *gin.Context) {
	_ = s.store.DeletePushSubByID(c.Request.Context(), currentUser(c).ID, c.Param("id"))
	c.Status(http.StatusNoContent)
}

// deviceLabel distills a user-agent into "Chrome on Android"-style text.
func deviceLabel(ua string) string {
	os := "device"
	for _, cand := range []string{"Android", "iPhone", "iPad", "Windows", "Mac OS", "Linux"} {
		if strings.Contains(ua, cand) {
			os = cand
			break
		}
	}
	browser := "Browser"
	for _, cand := range []string{"Edg", "Firefox", "Chrome", "Safari"} {
		if strings.Contains(ua, cand) {
			browser = map[string]string{"Edg": "Edge"}[cand]
			if browser == "" {
				browser = cand
			}
			break
		}
	}
	return browser + " on " + os
}

func (s *Server) unsubscribePush(c *gin.Context) {
	var body struct {
		Endpoint string `json:"endpoint"`
	}
	_ = c.ShouldBindJSON(&body)
	if body.Endpoint != "" {
		_ = s.store.DeleteUserPushSub(c.Request.Context(), currentUser(c).ID, body.Endpoint)
	}
	c.Status(http.StatusNoContent)
}

// pushStore is what the reminder loop needs from the store — kept narrow so
// the tick is unit-testable without a database.
type pushStore interface {
	DueReminders(ctx context.Context) ([]store.Entry, error)
	PushSubs(ctx context.Context, userID string) ([]store.PushSubscription, error)
	DeletePushSub(ctx context.Context, endpoint string) error
	MarkReminded(ctx context.Context, id string) error
}

// pushSend delivers one notification; returns the HTTP status (0 on error).
type pushSend func(payload []byte, sub store.PushSubscription, pub, priv string) (int, error)

func sendWebpush(payload []byte, sub store.PushSubscription, pub, priv string) (int, error) {
	resp, err := webpush.SendNotification(payload, &webpush.Subscription{
		Endpoint: sub.Endpoint,
		Keys:     webpush.Keys{P256dh: sub.P256dh, Auth: sub.Auth},
	}, &webpush.Options{
		Subscriber:      "cal@localhost",
		VAPIDPublicKey:  pub,
		VAPIDPrivateKey: priv,
		TTL:             300,
	})
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	return resp.StatusCode, nil
}

// pushTick is one scheduler pass: find due reminders, push to each
// subscriber, prune dead endpoints, mark delivered.
func pushTick(ctx context.Context, s pushStore, pub, priv string, send pushSend) {
	due, err := s.DueReminders(ctx)
	if err != nil {
		return
	}
	for _, entry := range due {
		subs, err := s.PushSubs(ctx, entry.OwnerID)
		if err != nil || len(subs) == 0 {
			continue
		}
		payload, _ := json.Marshal(gin.H{
			"title": entry.Title,
			"body":  "Starts at " + deref(entry.StartTime),
			"tag":   entry.ID,
		})
		for _, sub := range subs {
			status, err := send(payload, sub, pub, priv)
			if err != nil {
				continue
			}
			if status == http.StatusGone || status == http.StatusNotFound {
				_ = s.DeletePushSub(ctx, sub.Endpoint)
			}
		}
		_ = s.MarkReminded(ctx, entry.ID)
	}
	// Daily digest: settings.digest_time per user, once per day.
	if st, ok := s.(*store.Store); ok {
		due, err := st.DigestDue(ctx)
		if err != nil {
			return
		}
		for _, d := range due {
			subs, err := st.PushSubs(ctx, d.UserID)
			if err != nil || len(subs) == 0 {
				st.MarkDigestSent(ctx, d.UserID)
				continue
			}
			payload, _ := json.Marshal(gin.H{
				"title": "Good morning — your day",
				"body":  d.Payload + " on the calendar today",
				"tag":   "digest",
			})
			for _, sub := range subs {
				status, err := send(payload, sub, pub, priv)
				if err != nil {
					continue
				}
				if status == http.StatusGone || status == http.StatusNotFound {
					_ = st.DeletePushSub(ctx, sub.Endpoint)
				}
			}
			st.MarkDigestSent(ctx, d.UserID)
		}

		// Person date reminders: "Mum's birthday in 7 days" — one push per
		// occurrence year, deduplicated via person_reminder_log.
		duePeople, err := st.PersonRemindersDue(ctx)
		if err == nil {
			for _, r := range duePeople {
				subs, err := st.PushSubs(ctx, r.UserID)
				if err != nil || len(subs) == 0 {
					st.MarkPersonReminderSent(ctx, r.PersonID, r.DateKey, r.Year)
					continue
				}
				when := "today"
				if r.DaysUntil == 1 {
					when = "tomorrow"
				} else if r.DaysUntil > 1 {
					when = "in " + strconv.Itoa(r.DaysUntil) + " days"
				}
				payload, _ := json.Marshal(gin.H{
					"title": r.Name + "'s " + r.Label,
					"body":  r.Label + " " + when,
					"tag":   "person-" + r.PersonID + "-" + r.DateKey,
				})
				for _, sub := range subs {
					status, err := send(payload, sub, pub, priv)
					if err != nil {
						continue
					}
					if status == http.StatusGone || status == http.StatusNotFound {
						_ = st.DeletePushSub(ctx, sub.Endpoint)
					}
				}
				st.MarkPersonReminderSent(ctx, r.PersonID, r.DateKey, r.Year)
			}
		}
	}
}

// PushLoop fires due reminders every minute. Runs for the life of the process.
func PushLoop(ctx context.Context, s *store.Store, every time.Duration) {
	pub, priv, err := vapidKeys(ctx, s)
	if err != nil {
		log.Printf("push: vapid unavailable: %v", err)
		return
	}
	pushTick(ctx, s, pub, priv, sendWebpush)
	ticker := time.NewTicker(every)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			pushTick(ctx, s, pub, priv, sendWebpush)
		}
	}
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
