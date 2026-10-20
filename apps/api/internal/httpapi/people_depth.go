package httpapi

// Phase 9 people depth: person detail, relationship links, timeline,
// attachments listing, nameday lookup, holiday browse/import, CardDAV
// import-as-people, and password reset via the configured mail account.

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"cal/apps/api/internal/auth"
	"cal/apps/api/internal/carddav"
	"cal/apps/api/internal/store"

	"github.com/emersion/go-sasl"
	"github.com/gin-gonic/gin"
)

// ---------- Person detail ----------

func (s *Server) getPerson(c *gin.Context) {
	p, err := s.store.GetPerson(c.Request.Context(), currentUser(c).ID, c.Param("id"))
	if errors.Is(err, store.ErrNotFound) {
		c.String(http.StatusNotFound, "not found")
		return
	} else if err != nil {
		c.String(http.StatusInternalServerError, "failed")
		return
	}
	c.JSON(http.StatusOK, p)
}

// ---------- Relationships ----------

var relationKinds = map[string]bool{
	"parent": true, "child": true, "sibling": true, "partner": true,
	"friend": true, "coworker": true, "mentor": true,
}

// allPersonRelations returns every link in the account (family-tree view).
func (s *Server) allPersonRelations(c *gin.Context) {
	rels, err := s.store.AllPersonRelations(c.Request.Context(), currentUser(c).ID)
	if err != nil {
		c.String(http.StatusInternalServerError, "failed")
		return
	}
	c.JSON(http.StatusOK, rels)
}

func (s *Server) personRelations(c *gin.Context) {
	if _, err := s.store.GetPerson(c.Request.Context(), currentUser(c).ID, c.Param("id")); err != nil {
		c.String(http.StatusNotFound, "not found")
		return
	}
	rels, err := s.store.PersonRelations(c.Request.Context(), currentUser(c).ID, c.Param("id"))
	if err != nil {
		c.String(http.StatusInternalServerError, "failed")
		return
	}
	c.JSON(http.StatusOK, rels)
}

func (s *Server) linkPersons(c *gin.Context) {
	var body struct {
		ToID string `json:"toId"`
		Kind string `json:"kind"`
	}
	if !bind(c, &body) || !relationKinds[body.Kind] {
		c.String(http.StatusBadRequest, "invalid link")
		return
	}
	userID := currentUser(c).ID
	fromID := c.Param("id")
	if body.ToID == fromID {
		c.String(http.StatusBadRequest, "cannot link a person to themselves")
		return
	}
	// Both ends must belong to the caller.
	if _, err := s.store.GetPerson(c.Request.Context(), userID, fromID); err != nil {
		c.String(http.StatusNotFound, "not found")
		return
	}
	if _, err := s.store.GetPerson(c.Request.Context(), userID, body.ToID); err != nil {
		c.String(http.StatusBadRequest, "unknown person")
		return
	}
	rel, err := s.store.LinkPersons(c.Request.Context(), userID, fromID, body.ToID, body.Kind)
	if err != nil {
		c.String(http.StatusConflict, "link already exists")
		return
	}
	c.JSON(http.StatusCreated, rel)
}

func (s *Server) unlinkPersons(c *gin.Context) {
	err := s.store.UnlinkPersons(c.Request.Context(), currentUser(c).ID, c.Param("linkId"))
	if errors.Is(err, store.ErrNotFound) {
		c.String(http.StatusNotFound, "not found")
		return
	} else if err != nil {
		c.String(http.StatusInternalServerError, "failed")
		return
	}
	c.Status(http.StatusNoContent)
}

// ---------- Timeline ----------

var timelineTypes = map[string]bool{
	"met": true, "gift": true, "trip": true, "achievement": true,
	"memory": true, "note": true,
}

func (s *Server) personTimeline(c *gin.Context) {
	if _, err := s.store.GetPerson(c.Request.Context(), currentUser(c).ID, c.Param("id")); err != nil {
		c.String(http.StatusNotFound, "not found")
		return
	}
	items, err := s.store.PersonTimeline(c.Request.Context(), currentUser(c).ID, c.Param("id"))
	if err != nil {
		c.String(http.StatusInternalServerError, "failed")
		return
	}
	c.JSON(http.StatusOK, items)
}

type timelineInput struct {
	Type       string `json:"type"`
	Title      string `json:"title"`
	Body       string `json:"body"`
	OccurredOn string `json:"occurredOn"`
}

func validTimelineInput(in timelineInput) bool {
	if !timelineTypes[in.Type] {
		return false
	}
	in.Title = strings.TrimSpace(in.Title)
	if in.Title == "" || len(in.Title) > 200 || len(in.Body) > 20000 {
		return false
	}
	return in.OccurredOn == "" || validDate(in.OccurredOn)
}

func (s *Server) createTimelineItem(c *gin.Context) {
	var body timelineInput
	if !bind(c, &body) {
		c.String(http.StatusBadRequest, "invalid json")
		return
	}
	body.Title = strings.TrimSpace(body.Title)
	if body.Type == "" {
		body.Type = "note"
	}
	if !validTimelineInput(body) {
		c.String(http.StatusBadRequest, "invalid item")
		return
	}
	userID := currentUser(c).ID
	personID := c.Param("id")
	if _, err := s.store.GetPerson(c.Request.Context(), userID, personID); err != nil {
		c.String(http.StatusNotFound, "not found")
		return
	}
	item, err := s.store.CreateTimelineItem(c.Request.Context(), userID, personID, body.Type, body.Title, body.Body, body.OccurredOn)
	if err != nil {
		c.String(http.StatusInternalServerError, "failed")
		return
	}
	c.JSON(http.StatusCreated, item)
}

func (s *Server) updateTimelineItem(c *gin.Context) {
	var body timelineInput
	if !bind(c, &body) {
		c.String(http.StatusBadRequest, "invalid json")
		return
	}
	body.Title = strings.TrimSpace(body.Title)
	if body.Type == "" {
		body.Type = "note"
	}
	if !validTimelineInput(body) {
		c.String(http.StatusBadRequest, "invalid item")
		return
	}
	item, err := s.store.UpdateTimelineItem(c.Request.Context(), currentUser(c).ID, c.Param("itemId"), body.Type, body.Title, body.Body, body.OccurredOn)
	if errors.Is(err, store.ErrNotFound) {
		c.String(http.StatusNotFound, "not found")
		return
	} else if err != nil {
		c.String(http.StatusInternalServerError, "failed")
		return
	}
	c.JSON(http.StatusOK, item)
}

func (s *Server) deleteTimelineItem(c *gin.Context) {
	err := s.store.DeleteTimelineItem(c.Request.Context(), currentUser(c).ID, c.Param("itemId"))
	if errors.Is(err, store.ErrNotFound) {
		c.String(http.StatusNotFound, "not found")
		return
	} else if err != nil {
		c.String(http.StatusInternalServerError, "failed")
		return
	}
	c.Status(http.StatusNoContent)
}

// ---------- Person attachments ----------

func (s *Server) personFiles(c *gin.Context) {
	if _, err := s.store.GetPerson(c.Request.Context(), currentUser(c).ID, c.Param("id")); err != nil {
		c.String(http.StatusNotFound, "not found")
		return
	}
	files, err := s.store.FilesForPerson(c.Request.Context(), currentUser(c).ID, c.Param("id"))
	if err != nil {
		c.String(http.StatusInternalServerError, "failed")
		return
	}
	c.JSON(http.StatusOK, files)
}

// ---------- Namedays ----------

func (s *Server) namedaySearch(c *gin.Context) {
	name := strings.TrimSpace(c.Query("name"))
	if name == "" || len(name) > 60 {
		c.String(http.StatusBadRequest, "name required")
		return
	}
	res, err := s.nameday.SearchByName(c.Request.Context(), name)
	if err != nil {
		c.String(http.StatusBadGateway, "nameday lookup failed")
		return
	}
	// Optional country filter — the editor narrows to the configured country.
	if country := strings.ToLower(strings.TrimSpace(c.Query("country"))); country != "" {
		filtered := res[:0]
		for _, r := range res {
			if r.Country == country {
				filtered = append(filtered, r)
			}
		}
		res = filtered
	}
	c.JSON(http.StatusOK, res)
}

func (s *Server) namedayDate(c *gin.Context) {
	month, err1 := strconv.Atoi(c.Query("month"))
	day, err2 := strconv.Atoi(c.Query("day"))
	if err1 != nil || err2 != nil {
		c.String(http.StatusBadRequest, "month and day required")
		return
	}
	res, err := s.nameday.GetByDate(c.Request.Context(), month, day)
	if err != nil {
		c.String(http.StatusBadGateway, "nameday lookup failed")
		return
	}
	c.JSON(http.StatusOK, res)
}

func (s *Server) namedayCountries(c *gin.Context) {
	c.JSON(http.StatusOK, s.nameday.SupportedCountries())
}

// ---------- Holiday browsing (date.nager.at) ----------

func (s *Server) browseHolidays(c *gin.Context) {
	year, err := strconv.Atoi(c.Query("year"))
	if err != nil || year < 1970 || year > 2100 {
		c.String(http.StatusBadRequest, "invalid year")
		return
	}
	country := strings.TrimSpace(c.Query("country"))
	if len(country) != 2 {
		c.String(http.StatusBadRequest, "country required")
		return
	}
	items, err := s.nager.List(c.Request.Context(), country, year)
	if err != nil {
		c.String(http.StatusBadGateway, "holiday lookup failed")
		return
	}
	c.JSON(http.StatusOK, items)
}

func (s *Server) browseHolidayCountries(c *gin.Context) {
	items, err := s.nager.Countries(c.Request.Context())
	if err != nil {
		c.String(http.StatusBadGateway, "holiday lookup failed")
		return
	}
	c.JSON(http.StatusOK, items)
}

// importHolidays creates yearly all-day events for the requested nager
// holidays — one entry per {date,name}, skipping duplicates already on the
// calendar (title+date match).
func (s *Server) importHolidays(c *gin.Context) {
	var body struct {
		Country string `json:"country"`
		Year    int    `json:"year"`
	}
	if !bind(c, &body) || len(body.Country) != 2 || body.Year < 1970 || body.Year > 2100 {
		c.String(http.StatusBadRequest, "country and year required")
		return
	}
	items, err := s.nager.List(c.Request.Context(), body.Country, body.Year)
	if err != nil {
		c.String(http.StatusBadGateway, "holiday lookup failed")
		return
	}
	userID := currentUser(c).ID
	existing := map[string]bool{}
	if all, err := s.store.ListEntries(c.Request.Context(), userID, "", "", ""); err == nil {
		for _, e := range all {
			existing[e.Date+"|"+e.Title] = true
		}
	}
	imported := 0
	for _, h := range items {
		if h.Date == "" || h.Name == "" || existing[h.Date+"|"+h.Name] {
			continue
		}
		if _, err := s.store.CreateEntry(c.Request.Context(), userID, store.EntryInput{
			Title: h.Name, Type: "event", Date: h.Date, Recur: "yearly",
			Tags: []string{"holiday"},
		}); err == nil {
			imported++
		}
	}
	c.JSON(http.StatusOK, gin.H{"imported": imported, "found": len(items)})
}

// ---------- CardDAV import-as-people ----------

// carddavImportPeople creates person records from a saved addressbook.
// Contacts already present by name are skipped; yearless birthdays become a
// dates[] "birthday" entry anchored to year 1900 (recurrence is month+day).
func (s *Server) carddavImportPeople(c *gin.Context) {
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
	existing := map[string]bool{}
	if people, err := s.store.ListPeople(c.Request.Context(), userID); err == nil {
		for _, p := range people {
			existing[strings.ToLower(p.Name)] = true
		}
	}
	imported := 0
	for _, ct := range contacts {
		name := strings.TrimSpace(ct.Name)
		if name == "" || existing[strings.ToLower(name)] {
			continue
		}
		in := store.PersonInput{Name: name, Relation: "contact"}
		if ct.Birthday != "" {
			in.Dates = []store.PersonDate{{Label: "birthday", Date: "1900-" + ct.Birthday}}
		}
		if _, err := s.store.CreatePerson(c.Request.Context(), userID, in); err == nil {
			existing[strings.ToLower(name)] = true
			imported++
		}
	}
	c.JSON(http.StatusOK, gin.H{"imported": imported, "found": len(contacts)})
}

// ---------- Password reset (via the configured mail account) ----------

// forgotPassword issues a reset link by email when the account exists AND a
// mail account is configured. Always 204 — never leaks which part failed.
func (s *Server) forgotPassword(c *gin.Context) {
	var body struct {
		Email string `json:"email"`
	}
	if !bind(c, &body) || !strings.Contains(body.Email, "@") {
		c.Status(http.StatusNoContent)
		return
	}
	user, _, err := s.store.UserByEmail(c.Request.Context(), body.Email)
	if err != nil {
		c.Status(http.StatusNoContent)
		return
	}
	accounts, err := s.store.ListMailAccounts(c.Request.Context(), user.ID)
	if err != nil || len(accounts) == 0 {
		log.Printf("password reset: no mail account for user %s — link not sent", user.ID)
		c.Status(http.StatusNoContent)
		return
	}
	raw := make([]byte, 32)
	_, _ = rand.Read(raw)
	token := hex.EncodeToString(raw)
	hash := sha256.Sum256([]byte(token))
	if err := s.store.CreatePasswordReset(c.Request.Context(), user.ID, hex.EncodeToString(hash[:]), time.Now().Add(time.Hour)); err != nil {
		c.String(http.StatusInternalServerError, "failed")
		return
	}
	link := strings.TrimRight(env("WEB_ORIGIN", "http://localhost:5173"), "/") + "/reset-password?token=" + token
	if err := s.sendResetMail(c, user.ID, accounts[0].ID, user.Email, link); err != nil {
		log.Printf("password reset: send failed for user %s: %v", user.ID, err)
	}
	c.Status(http.StatusNoContent)
}

// sendResetMail delivers the reset link through the user's own SMTP account.
func (s *Server) sendResetMail(c *gin.Context, userID, accountID, to, link string) error {
	acct, password, err := s.store.MailCredentials(c.Request.Context(), userID, accountID)
	if err != nil {
		return err
	}
	client, err := smtpDial(acct)
	if err != nil {
		return err
	}
	defer client.Close()
	if err := client.Auth(sasl.NewPlainClient("", acct.Username, password)); err != nil {
		return err
	}
	var msg strings.Builder
	fmt.Fprintf(&msg, "From: %s\r\nTo: %s\r\n", sanitizeHeader(acct.Email), sanitizeHeader(to))
	fmt.Fprintf(&msg, "Subject: %s\r\nDate: %s\r\n", "Reset your Cal password", time.Now().Format(time.RFC1123Z))
	fmt.Fprintf(&msg, "Content-Type: text/plain; charset=utf-8\r\n\r\n")
	fmt.Fprintf(&msg, "Someone requested a password reset for this Cal account.\r\n\r\n%s\r\n\r\nThe link works once and expires in one hour. If this wasn't you, ignore this mail.", link)
	return client.SendMail(acct.Email, []string{to}, strings.NewReader(msg.String()))
}

// resetPassword consumes a single-use token and sets the new password.
// Every session is revoked — the user logs back in fresh.
func (s *Server) resetPassword(c *gin.Context) {
	var body struct {
		Token    string `json:"token"`
		Password string `json:"password"`
	}
	if !bind(c, &body) || len(body.Token) != 64 {
		c.String(http.StatusBadRequest, "invalid request")
		return
	}
	if err := auth.ValidatePassword(body.Password); err != nil {
		c.String(http.StatusBadRequest, err.Error())
		return
	}
	hash := sha256.Sum256([]byte(body.Token))
	userID, err := s.store.ConsumePasswordReset(c.Request.Context(), hex.EncodeToString(hash[:]))
	if err != nil {
		c.String(http.StatusBadRequest, "link expired or already used")
		return
	}
	pwHash, err := auth.HashPassword(body.Password)
	if err != nil {
		c.String(http.StatusInternalServerError, "failed")
		return
	}
	if err := s.store.ChangePassword(c.Request.Context(), userID, pwHash); err != nil {
		c.String(http.StatusInternalServerError, "failed")
		return
	}
	_ = s.store.RevokeOtherSessions(c.Request.Context(), userID, "")
	c.Status(http.StatusNoContent)
}
