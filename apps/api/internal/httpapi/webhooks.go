package httpapi

// Outbound webhooks: registered URLs get a POSTed JSON event on entry
// create/update/delete, signed with HMAC-SHA256 in X-Cal-Signature.
// Fire-and-forget; a failure to deliver is logged, not retried.

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"time"

	"github.com/gin-gonic/gin"
)

// webhookClient refuses private addresses on every connection, redirects
// included; privateWebhookClient serves CAL_ALLOW_PRIVATE_WEBHOOKS.
var (
	webhookClient        = newOutboundClient(5*time.Second, isBlockedIP)
	privateWebhookClient = newOutboundClient(5*time.Second, nil)
)

func (s *Server) listWebhooks(c *gin.Context) {
	hooks, err := s.store.Webhooks(c.Request.Context(), currentUser(c).ID)
	if err != nil {
		c.String(http.StatusInternalServerError, "failed")
		return
	}
	c.JSON(http.StatusOK, hooks)
}

func (s *Server) addWebhook(c *gin.Context) {
	var body struct {
		URL string `json:"url"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.String(http.StatusBadRequest, "url required")
		return
	}
	u, err := url.Parse(body.URL)
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") {
		c.String(http.StatusBadRequest, "http(s) url required")
		return
	}
	// SSRF guard — private/loopback refused unless explicitly opted into for
	// local testing (CAL_ALLOW_PRIVATE_WEBHOOKS=1). Webhooks are user-provided
	// URLs; without this a registered hook becomes an internal probe.
	if os.Getenv("CAL_ALLOW_PRIVATE_WEBHOOKS") == "" {
		if err := checkURL(u); err != nil {
			c.String(http.StatusBadRequest, "webhook url not allowed: "+err.Error())
			return
		}
	}
	secret := make([]byte, 16)
	_, _ = rand.Read(secret)
	hook, err := s.store.CreateWebhook(c.Request.Context(), currentUser(c).ID, u.String(), hex.EncodeToString(secret))
	if err != nil {
		c.String(http.StatusInternalServerError, "failed")
		return
	}
	c.JSON(http.StatusCreated, hook)
}

func (s *Server) deleteWebhook(c *gin.Context) {
	if err := s.store.DeleteWebhook(c.Request.Context(), currentUser(c).ID, c.Param("id")); err != nil {
		c.String(http.StatusInternalServerError, "failed")
		return
	}
	c.Status(http.StatusNoContent)
}

// fireWebhooks posts {event, entry} to every registered URL. Async: callers
// invoke it in a goroutine; delivery failures are logged only.
func (s *Server) fireWebhooks(ctx context.Context, userID, event string, entry any) {
	hooks, err := s.store.Webhooks(ctx, userID)
	if err != nil || len(hooks) == 0 {
		return
	}
	payload, _ := json.Marshal(gin.H{"event": event, "entry": entry, "at": time.Now().UTC()})
	for _, h := range hooks {
		h := h
		go func() {
			if err := deliverWebhook(ctx, h.URL, h.Secret, event, payload); err != nil {
				log.Printf("webhook %s: %v — retrying in 30s", h.URL, err)
				timer := time.NewTimer(30 * time.Second)
				select {
				case <-ctx.Done():
					timer.Stop()
					return
				case <-timer.C:
				}
				if err := deliverWebhook(ctx, h.URL, h.Secret, event, payload); err != nil {
					log.Printf("webhook %s: retry failed: %v", h.URL, err)
				}
			}
		}()
	}
}

// deliverWebhook sends one signed POST; returns error on transport failure or
// a >=400 status (both worth one retry).
func deliverWebhook(ctx context.Context, hookURL, secret, event string, payload []byte) error {
	u, err := url.Parse(hookURL)
	if err != nil {
		return err
	}
	client := webhookClient
	if os.Getenv("CAL_ALLOW_PRIVATE_WEBHOOKS") == "" {
		if err := checkURL(u); err != nil {
			return err
		}
	} else {
		client = privateWebhookClient
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u.String(), bytes.NewReader(payload))
	if err != nil {
		return err
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(payload)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Cal-Event", event)
	req.Header.Set("X-Cal-Signature", hex.EncodeToString(mac.Sum(nil)))
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	_ = resp.Body.Close()
	if resp.StatusCode >= 400 {
		return errors.New("status " + resp.Status)
	}
	return nil
}
