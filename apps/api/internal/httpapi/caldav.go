package httpapi

// CalDAV account handlers: connect a calendar collection, test credentials,
// delete the account, force a sync. Passwords are AES-encrypted at rest.

import (
	"context"
	"net/http"
	"time"

	"cal/apps/api/internal/caldav"
	"cal/apps/api/internal/carddav"
	"cal/apps/api/internal/store"

	"github.com/gin-gonic/gin"
)

func (s *Server) listCaldav(c *gin.Context) {
	accounts, err := s.store.CaldavAccounts(c.Request.Context(), currentUser(c).ID)
	if err != nil {
		c.String(http.StatusInternalServerError, "failed")
		return
	}
	c.JSON(http.StatusOK, accounts)
}

func (s *Server) testCaldav(c *gin.Context) {
	var body struct {
		URL      string `json:"url"`
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := c.ShouldBindJSON(&body); err != nil || body.URL == "" {
		c.String(http.StatusBadRequest, "url required")
		return
	}
	if err := caldav.New(body.URL, body.Username, body.Password).TestConnection(c.Request.Context()); err != nil {
		c.String(http.StatusBadGateway, "connection failed: "+err.Error())
		return
	}
	c.Status(http.StatusNoContent)
}

func (s *Server) createCaldav(c *gin.Context) {
	var body struct {
		Name     string `json:"name"`
		URL      string `json:"url"`
		Username string `json:"username"`
		Password string `json:"password"`
		Color    string `json:"color"`
	}
	if err := c.ShouldBindJSON(&body); err != nil || body.URL == "" {
		c.String(http.StatusBadRequest, "url required")
		return
	}
	// Verify before persisting — a saved-but-broken account is worse than none.
	if err := caldav.New(body.URL, body.Username, body.Password).TestConnection(c.Request.Context()); err != nil {
		c.String(http.StatusBadGateway, "connection failed: "+err.Error())
		return
	}
	enc, err := s.store.Encrypt(c.Request.Context(), body.Password)
	if err != nil {
		c.String(http.StatusInternalServerError, "failed")
		return
	}
	if body.Name == "" {
		body.Name = body.URL
	}
	if body.Color == "" {
		body.Color = "sky"
	}
	account, err := s.store.CreateCaldavAccount(c.Request.Context(), currentUser(c).ID, body.Name, body.URL, body.Username, enc, body.Color)
	if err != nil {
		c.String(http.StatusInternalServerError, "failed to save account")
		return
	}
	// Kick an initial sync so the calendar fills immediately.
	go func() {
		_ = caldav.NewSyncer(s.store).SyncAccount(context.Background(), currentUser(c).ID, account)
	}()
	c.JSON(http.StatusCreated, account)
}

func (s *Server) deleteCaldav(c *gin.Context) {
	if err := s.store.DeleteCaldavAccount(c.Request.Context(), currentUser(c).ID, c.Param("id")); err != nil {
		c.String(http.StatusInternalServerError, "failed")
		return
	}
	c.Status(http.StatusNoContent)
}

// discoverCaldav lists the calendar collections available under a base URL so
// the user can pick instead of pasting a collection path.
func (s *Server) discoverCaldav(c *gin.Context) {
	var body struct {
		URL      string `json:"url"`
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := c.ShouldBindJSON(&body); err != nil || body.URL == "" {
		c.String(http.StatusBadRequest, "url required")
		return
	}
	cols, err := caldav.Discover(c.Request.Context(), body.URL, body.Username, body.Password)
	if err != nil {
		c.String(http.StatusBadGateway, "discovery failed: "+err.Error())
		return
	}
	c.JSON(http.StatusOK, cols)
}

// connectCarddav stores an addressbook and immediately imports birthdays as
// yearly recurring events tagged "birthday".
func (s *Server) connectCarddav(c *gin.Context) {
	var body struct {
		Name     string `json:"name"`
		URL      string `json:"url"`
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := c.ShouldBindJSON(&body); err != nil || body.URL == "" {
		c.String(http.StatusBadRequest, "url required")
		return
	}
	contacts, err := carddav.New(body.URL, body.Username, body.Password).Birthdays(c.Request.Context())
	if err != nil {
		c.String(http.StatusBadGateway, "sync failed: "+err.Error())
		return
	}
	enc, err := s.store.Encrypt(c.Request.Context(), body.Password)
	if err != nil {
		c.String(http.StatusInternalServerError, "failed")
		return
	}
	if body.Name == "" {
		body.Name = body.URL
	}
	if err := s.store.CreateCarddavAccount(c.Request.Context(), currentUser(c).ID, body.Name, body.URL, body.Username, enc); err != nil {
		c.String(http.StatusInternalServerError, "failed to save account")
		return
	}
	imported := s.importBirthdays(c, currentUser(c).ID, contacts)
	c.JSON(http.StatusCreated, gin.H{"imported": imported, "found": len(contacts)})
}

// listCarddav returns the user's saved addressbooks (no secrets).
func (s *Server) listCarddav(c *gin.Context) {
	accounts, err := s.store.CarddavAccounts(c.Request.Context(), currentUser(c).ID)
	if err != nil {
		c.String(http.StatusInternalServerError, "failed")
		return
	}
	c.JSON(http.StatusOK, accounts)
}

// syncCarddav re-pulls birthdays for a saved account.
func (s *Server) syncCarddav(c *gin.Context) {
	account, userID, err := s.store.CarddavAccountWithSecret(c.Request.Context(), c.Param("id"))
	if err != nil || userID != currentUser(c).ID {
		c.String(http.StatusNotFound, "account not found")
		return
	}
	password, err := s.store.Decrypt(c.Request.Context(), account.PasswordEnc)
	if err != nil {
		c.String(http.StatusInternalServerError, "failed")
		return
	}
	contacts, err := carddav.New(account.URL, account.Username, password).Birthdays(c.Request.Context())
	if err != nil {
		c.String(http.StatusBadGateway, "sync failed: "+err.Error())
		return
	}
	_ = s.store.TouchCarddavSync(c.Request.Context(), account.ID)
	c.JSON(http.StatusOK, gin.H{"imported": s.importBirthdays(c, userID, contacts), "found": len(contacts)})
}

func (s *Server) deleteCarddav(c *gin.Context) {
	if err := s.store.DeleteCarddavAccount(c.Request.Context(), currentUser(c).ID, c.Param("id")); err != nil {
		c.String(http.StatusInternalServerError, "failed")
		return
	}
	c.Status(http.StatusNoContent)
}

// importBirthdays inserts a yearly all-day event per contact with a BDAY,
// skipping ones already present (title match).
func (s *Server) importBirthdays(c *gin.Context, userID string, contacts []carddav.Contact) int {
	existing := map[string]bool{}
	if all, err := s.store.ListEntries(c.Request.Context(), userID, "", "", ""); err == nil {
		for _, e := range all {
			if hasTag(e.Tags, "birthday") {
				existing[e.Title] = true
			}
		}
	}
	year := localDay(s.store.UserLocation(c.Request.Context(), userID)).Format("2006")
	imported := 0
	for _, ct := range contacts {
		title := ct.Name + "'s birthday"
		if existing[title] {
			continue
		}
		// BDAY is MM-DD — anchor it this year; recur yearly handles the rest.
		date := year + "-" + ct.Birthday
		if _, err := time.Parse(time.DateOnly, date); err != nil {
			continue
		}
		if _, err := s.store.CreateEntry(c.Request.Context(), userID, store.EntryInput{
			Title: title, Type: "event", Date: date, Recur: "yearly", Tags: []string{"birthday", "carddav"},
		}); err == nil {
			imported++
		}
	}
	return imported
}

func (s *Server) syncCaldav(c *gin.Context) {
	account, userID, err := s.store.CaldavAccountWithSecret(c.Request.Context(), c.Param("id"))
	if err != nil || userID != currentUser(c).ID {
		c.String(http.StatusNotFound, "account not found")
		return
	}
	if err := caldav.NewSyncer(s.store).SyncAccount(c.Request.Context(), userID, account); err != nil {
		c.String(http.StatusBadGateway, "sync failed: "+err.Error())
		return
	}
	c.Status(http.StatusNoContent)
}
