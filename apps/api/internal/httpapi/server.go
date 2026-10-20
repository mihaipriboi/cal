package httpapi

import (
	"archive/zip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"cal/apps/api/internal/auth"
	"cal/apps/api/internal/calendar"
	"cal/apps/api/internal/holiday"
	"cal/apps/api/internal/nameday"
	"cal/apps/api/internal/store"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
)

type Server struct {
	store   *store.Store
	holiday *calendar.HolidayCache
	nameday *nameday.Service
	nager   *holiday.Client
	secure  bool
	dataDir string
}

type authRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

func New(st *store.Store, holidays *calendar.HolidayCache) *gin.Engine {
	secure := os.Getenv("SESSION_SECURE") == "true"
	ndLoader, _ := nameday.NewLoader() // embedded; an error means a corrupt build
	server := &Server{
		store:   st,
		holiday: holidays,
		nameday: nameday.NewService(nameday.NewAbalinClient(env("NAMEDAY_API_URL", "")), ndLoader),
		nager:   holiday.NewClient(env("NAGER_API_URL", "")),
		secure:  secure,
		dataDir: env("DATA_DIR", "./data"),
	}

	router := gin.New()
	// ClientIP() (rate limits) only honors X-Forwarded-For from listed
	// proxies; unset trusts none, so the socket peer is the client.
	proxies, err := trustedProxies(os.Getenv("TRUSTED_PROXIES"))
	if err == nil {
		err = router.SetTrustedProxies(proxies)
	}
	if err != nil {
		log.Fatalf("TRUSTED_PROXIES: %v", err)
	}
	router.Use(gin.Recovery())
	router.Use(securityHeaders())
	router.Use(cors.New(cors.Config{
		AllowMethods: []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowHeaders: []string{"Content-Type", "Authorization"},
		AllowOriginFunc: func(origin string) bool {
			// Web origin stays explicit; Capacitor/Ionic webview schemes are
			// app-only — browsers can't send them as Origin.
			if origin == env("WEB_ORIGIN", "http://localhost:5173") {
				return true
			}
			if strings.HasPrefix(origin, "capacitor://") || strings.HasPrefix(origin, "ionic://") {
				return true
			}
			for _, p := range []string{"http://localhost", "https://localhost", "http://127.0.0.1", "https://127.0.0.1", "http://[::1]", "https://[::1]"} {
				if strings.HasPrefix(origin, p) {
					return true
				}
			}
			return false
		},
		AllowCredentials: true,
		MaxAge:           12 * time.Hour,
	}))

	api := router.Group("/api")
	api.GET("/health", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"ok": true}) })
	api.GET("/auth/config", server.authConfig)
	authLimited := api.Group("/auth", newRateLimiter(8, time.Minute))
	authLimited.POST("/register", server.register)
	authLimited.POST("/login", server.login)
	authLimited.POST("/forgot", server.forgotPassword)
	authLimited.POST("/reset", server.resetPassword)
	api.POST("/auth/logout", server.logout)
	api.GET("/holidays", server.holidays)
	api.GET("/holidays/countries", server.holidayCountries)

	authed := api.Group("")
	authed.Use(server.requireUser)
	authed.GET("/me", server.me)
	authed.GET("/entries", server.entries)
	authed.POST("/entries/:id/refresh-link", server.refreshLink)
	authed.POST("/entries", server.createEntry)
	authed.PATCH("/entries/:id", server.updateEntry)
	authed.DELETE("/entries/:id", server.deleteEntry)
	authed.GET("/entries/:id/revisions", server.entryRevisions)
	authed.POST("/entries/:id/restore/:rev", server.restoreRevision)
	authed.GET("/settings", server.settings)
	authed.PUT("/settings", server.updateSettings)
	authed.GET("/export", server.export)
	authed.GET("/feeds", server.listFeeds)
	authed.POST("/feeds", newRateLimiter(10, time.Minute), server.createFeed)
	authed.DELETE("/feeds/:id", server.deleteFeed)
	authed.POST("/feeds/:id/refresh", server.refreshFeed)
	authed.GET("/feed-events", server.feedEvents)
	authed.POST("/import", server.importICS)
	authed.POST("/settings/widget-token", server.rotateWidgetToken)
	authed.POST("/settings/api-token", server.rotateApiToken)
	authed.POST("/restore", server.restoreJSON)
	authed.GET("/sessions", server.listSessions)
	authed.DELETE("/sessions/:id", server.revokeSession)
	authed.POST("/password", server.changePassword)
	authed.GET("/webhooks", server.listWebhooks)
	authed.POST("/webhooks", server.addWebhook)
	authed.DELETE("/webhooks/:id", server.deleteWebhook)
	authed.GET("/review/week", server.weeklyReview)
	authed.GET("/activity", server.activityMap)
	authed.GET("/habits", server.habitStreaks)
	authed.GET("/unfurl", server.unfurl)
	authed.POST("/files", newRateLimiter(20, time.Minute), server.uploadFile)
	authed.GET("/files", server.listFiles)
	authed.GET("/files/:name", server.serveFile)
	authed.DELETE("/files/:id", server.deleteFile)
	authed.POST("/files/:id/share", server.shareFile)
	authed.GET("/storage", server.storageUsage)
	authed.GET("/push/vapid", server.pushVapid)
	authed.POST("/push/subscribe", server.subscribePush)
	authed.POST("/push/unsubscribe", server.unsubscribePush)
	authed.GET("/push/subscriptions", server.pushSubscriptions)
	authed.DELETE("/push/subscriptions/:id", server.deletePushSubscription)
	authed.GET("/boards", server.listBoards)
	authed.POST("/boards", server.createBoard)
	authed.DELETE("/boards/:id", server.deleteBoard)
	authed.GET("/boards/:id/view", server.boardView)
	authed.POST("/boards/:id/columns", server.createColumn)
	authed.PATCH("/columns/:id", server.renameColumn)
	authed.DELETE("/columns/:id", server.deleteColumn)
	authed.POST("/cards/:id/move", server.moveCard)
	authed.POST("/timer/start", server.startTimer)
	authed.POST("/timer/stop", server.stopTimer)
	authed.GET("/timer/current", server.currentTimer)
	authed.GET("/time/summary", server.timeSummary)
	authed.GET("/time/log", server.timeLog)
	authed.POST("/time/log", server.createTimeEntry)
	authed.PATCH("/time/log/:id", server.updateTimeEntry)
	authed.DELETE("/time/log/:id", server.deleteTimeEntry)
	authed.GET("/time/export", server.timeExport)
	authed.GET("/entries/:id/activity", server.entryActivity)
	authed.GET("/trash", server.listTrash)
	authed.POST("/trash/:id/restore", server.restoreEntry)
	authed.DELETE("/trash/:id", server.purgeEntry)
	authed.PATCH("/boards/:id", server.updateBoard)
	authed.POST("/boards/:id/share", server.shareBoard)
	authed.GET("/tags", server.tagCounts)
	authed.GET("/agenda", server.agendaMarkdown)
	authed.GET("/search", server.globalSearch)
	authed.GET("/youtube/search", server.youtubeSearch)
	authed.GET("/dashboard", server.dashboard)
	authed.GET("/workspaces", server.listWorkspaces)
	authed.POST("/workspaces", server.createWorkspace)
	authed.PATCH("/workspaces/:id", server.updateWorkspace)
	authed.DELETE("/workspaces/:id", server.deleteWorkspace)
	authed.PATCH("/files/:id", server.updateFile)
	authed.GET("/filters", server.listFilters)
	authed.POST("/filters", server.createFilter)
	authed.DELETE("/filters/:id", server.deleteFilter)
	authed.GET("/mail/accounts", server.listMailAccounts)
	authed.POST("/mail/accounts", server.createMailAccount)
	authed.DELETE("/mail/accounts/:id", server.deleteMailAccount)
	authed.POST("/mail/accounts/:id/test", server.testMailAccount)
	authed.GET("/mail/:id/mailboxes", server.mailMailboxes)
	authed.GET("/mail/:id/messages", server.mailMessages)
	authed.GET("/mail/:id/message/:uid", server.mailMessage)
	authed.POST("/mail/:id/message/:uid/flag", server.mailFlag)
	authed.DELETE("/mail/:id/message/:uid", server.mailDelete)
	authed.POST("/mail/:id/send", server.mailSend)
	authed.GET("/github/inbox", server.githubInbox)
	authed.GET("/github/activity", server.githubActivity)
	authed.POST("/github/import", server.githubImport)
	authed.GET("/caldav", server.listCaldav)
	authed.POST("/caldav", server.createCaldav)
	authed.POST("/caldav/test", server.testCaldav)
	authed.POST("/caldav/discover", server.discoverCaldav)
	authed.GET("/carddav", server.listCarddav)
	authed.POST("/carddav", server.connectCarddav)
	authed.POST("/carddav/:id/sync", server.syncCarddav)
	authed.DELETE("/carddav/:id", server.deleteCarddav)
	authed.GET("/google/connect", server.googleConnect)
	authed.GET("/google/status", server.googleStatus)
	authed.POST("/google/sync", server.googleSyncNow)
	authed.DELETE("/google", server.googleDisconnect)
	authed.DELETE("/caldav/:id", server.deleteCaldav)
	authed.POST("/caldav/:id/sync", server.syncCaldav)
	authed.GET("/people", server.listPeople)
	authed.POST("/people", server.createPerson)
	authed.POST("/people/share", server.sharePeople)
	authed.GET("/people/relations", server.allPersonRelations)
	authed.DELETE("/people/relations/:linkId", server.unlinkPersons)
	authed.GET("/people/:id", server.getPerson)
	authed.PATCH("/people/:id", server.updatePerson)
	authed.DELETE("/people/:id", server.deletePerson)
	authed.GET("/people/:id/relations", server.personRelations)
	authed.POST("/people/:id/relations", server.linkPersons)
	authed.GET("/people/:id/timeline", server.personTimeline)
	authed.POST("/people/:id/timeline", server.createTimelineItem)
	authed.PATCH("/people/:id/timeline/:itemId", server.updateTimelineItem)
	authed.DELETE("/people/:id/timeline/:itemId", server.deleteTimelineItem)
	authed.GET("/people/:id/files", server.personFiles)
	authed.GET("/namedays/search", server.namedaySearch)
	authed.GET("/namedays/date", server.namedayDate)
	authed.GET("/namedays/countries", server.namedayCountries)
	authed.GET("/holidays/browse", server.browseHolidays)
	authed.GET("/holidays/browse/countries", server.browseHolidayCountries)
	authed.POST("/holidays/import", server.importHolidays)
	authed.POST("/carddav/:id/import-people", server.carddavImportPeople)
	authed.GET("/shopping/lists", server.shoppingLists)
	authed.POST("/shopping/lists", server.shoppingCreateList)
	authed.GET("/shopping/lists/:id", server.shoppingListDetail)
	authed.PATCH("/shopping/lists/:id", server.shoppingUpdateList)
	authed.DELETE("/shopping/lists/:id", server.shoppingDeleteList)
	authed.POST("/shopping/lists/:id/sections", server.shoppingCreateSection)
	authed.PATCH("/shopping/sections/:id", server.shoppingUpdateSection)
	authed.DELETE("/shopping/sections/:id", server.shoppingDeleteSection)
	authed.POST("/shopping/sections/:id/check", server.shoppingCheckSection)
	authed.POST("/shopping/lists/:id/clear", server.shoppingClearPurchased)
	authed.POST("/shopping/lists/:id/items", server.shoppingCreateItem)
	authed.PATCH("/shopping/items/:id", server.shoppingUpdateItem)
	authed.DELETE("/shopping/items/:id", server.shoppingDeleteItem)
	authed.GET("/shopping/suggest", server.shoppingSuggest)

	admin := authed.Group("/admin", server.requireAdmin)
	admin.GET("/users", server.adminUsers)
	admin.PATCH("/users/:id", server.adminUpdateUser)
	admin.DELETE("/users/:id", server.adminDeleteUser)
	admin.GET("/config", server.adminConfig)
	admin.PUT("/config", server.adminUpdateConfig)

	router.GET("/api/widget/today", server.widgetToday)
	router.GET("/api/shared/files/:token", server.serveSharedFile)
	router.GET("/api/shared/boards/:token", server.serveSharedBoard)
	router.GET("/api/shared/people/:token", server.serveSharedPeople)
	router.GET("/api/shared/people/:token/calendar.ics", server.serveSharedPeopleICS)
	router.POST("/api/shared/boards/:token/cards/:id/move", server.sharedBoardMove)
	router.GET("/api/feed.ics", server.exportICS)
	router.POST("/api/mcp", newRateLimiter(60, time.Minute), server.mcp)
	router.POST("/api/intake", newRateLimiter(10, time.Minute), server.intake)
	router.GET("/api/google/callback", server.googleCallback)

	return router
}

func (s *Server) register(c *gin.Context) {
	var input authRequest
	if !bind(c, &input) {
		c.String(http.StatusBadRequest, "invalid email or password")
		return
	}
	if err := auth.ValidatePassword(input.Password); err != nil {
		c.String(http.StatusBadRequest, err.Error())
		return
	}
	if !validAuth(input) {
		c.String(http.StatusBadRequest, "invalid email or password")
		return
	}
	if s.registrationClosed(c) {
		c.String(http.StatusForbidden, "registration is closed")
		return
	}
	hash, err := auth.HashPassword(input.Password)
	if err != nil {
		c.String(http.StatusInternalServerError, "failed to create user")
		return
	}
	user, err := s.store.CreateUser(c.Request.Context(), input.Email, hash)
	if errors.Is(err, store.ErrRegistrationClosed) {
		c.String(http.StatusForbidden, "registration is closed")
		return
	}
	if err != nil {
		c.String(http.StatusConflict, "account already exists")
		return
	}
	session, ok := s.setSession(c, user.ID)
	if !ok {
		return
	}
	c.JSON(http.StatusOK, gin.H{"user": user, "session": session})
}

func (s *Server) login(c *gin.Context) {
	var input authRequest
	if !bind(c, &input) || !validAuth(input) {
		c.String(http.StatusUnauthorized, "invalid credentials")
		return
	}
	user, hash, err := s.store.UserByEmail(c.Request.Context(), input.Email)
	if err != nil || !auth.VerifyPassword(input.Password, hash) {
		c.String(http.StatusUnauthorized, "invalid credentials")
		return
	}
	session, ok := s.setSession(c, user.ID)
	if !ok {
		return
	}
	c.JSON(http.StatusOK, gin.H{"user": user, "session": session})
}

func (s *Server) logout(c *gin.Context) {
	if cookie, err := c.Cookie("cal_session"); err == nil {
		_ = s.store.DeleteSession(c.Request.Context(), cookie)
	}
	s.writeSessionCookie(c, "", -1)
	c.Status(http.StatusNoContent)
}

func (s *Server) me(c *gin.Context) {
	c.JSON(http.StatusOK, currentUser(c))
}

func (s *Server) entries(c *gin.Context) {
	user := currentUser(c)
	entries, err := s.store.ListEntriesScoped(
		c.Request.Context(),
		user.ID,
		c.Query("from"),
		c.Query("to"),
		c.Query("q"),
		c.Query("workspace"),
	)
	if err != nil {
		c.String(http.StatusInternalServerError, "failed to list entries")
		return
	}
	c.JSON(http.StatusOK, entries)
}

func (s *Server) createEntry(c *gin.Context) {
	var input store.EntryInput
	if !bind(c, &input) || !validEntry(input) {
		c.String(http.StatusBadRequest, "invalid entry")
		return
	}
	// accountId must reference one of the caller's CalDAV accounts.
	if input.AccountID != nil && *input.AccountID != "" {
		_, owner, err := s.store.CaldavAccountWithSecret(c.Request.Context(), *input.AccountID)
		if err != nil || owner != currentUser(c).ID {
			c.String(http.StatusBadRequest, "unknown calendar account")
			return
		}
	} else {
		input.AccountID = nil
	}
	// boardId/columnId must belong to the caller; a card appended without a
	// position lands at the end of its column.
	if input.BoardID != nil && *input.BoardID != "" {
		userID := currentUser(c).ID
		if !s.store.BoardExists(c.Request.Context(), userID, *input.BoardID) {
			c.String(http.StatusBadRequest, "unknown board")
			return
		}
		if input.ColumnID != nil && *input.ColumnID != "" && !s.store.ColumnInBoard(c.Request.Context(), userID, *input.BoardID, *input.ColumnID) {
			c.String(http.StatusBadRequest, "column not on this board")
			return
		}
		if input.Position == nil {
			cards, _ := s.store.BoardCards(c.Request.Context(), userID, *input.BoardID)
			top := 0.0
			for _, card := range cards {
				if card.ColumnID != nil && input.ColumnID != nil && *card.ColumnID == *input.ColumnID && card.Position != nil && *card.Position > top {
					top = *card.Position
				}
			}
			pos := top + 1024
			input.Position = &pos
		}
	} else {
		input.BoardID, input.ColumnID = nil, nil
	}
	// workspaceId must belong to the caller; blocked_by likewise.
	if input.WorkspaceID != nil && *input.WorkspaceID != "" {
		if !s.store.WorkspaceOwned(c.Request.Context(), currentUser(c).ID, *input.WorkspaceID) {
			c.String(http.StatusBadRequest, "unknown workspace")
			return
		}
	} else {
		input.WorkspaceID = nil
	}
	if input.BlockedBy != nil && *input.BlockedBy != "" {
		if _, err := s.store.Entry(c.Request.Context(), currentUser(c).ID, *input.BlockedBy); err != nil {
			c.String(http.StatusBadRequest, "unknown blocker")
			return
		}
	} else {
		input.BlockedBy = nil
	}
	entry, err := s.store.CreateEntry(c.Request.Context(), currentUser(c).ID, input)
	if err != nil {
		log.Printf("create entry: %v", err)
		c.String(http.StatusInternalServerError, "failed to create entry")
		return
	}
	go s.fireWebhooks(context.Background(), currentUser(c).ID, "entry.created", entry)
	if input.BoardID != nil {
		s.store.LogActivity(c.Request.Context(), currentUser(c).ID, entry.ID, "created", "card created on board")
	}
	if entry.LinkURL != "" {
		go s.enrichLink(currentUser(c).ID, entry.ID, entry.LinkURL)
	}
	c.JSON(http.StatusCreated, entry)
}

func (s *Server) updateEntry(c *gin.Context) {
	var raw map[string]json.RawMessage
	if !bind(c, &raw) {
		c.String(http.StatusBadRequest, "invalid entry")
		return
	}
	patch, ok := parsePatch(raw)
	if !ok {
		c.String(http.StatusBadRequest, "invalid entry")
		return
	}
	entry, err := s.store.UpdateEntry(c.Request.Context(), currentUser(c).ID, c.Param("id"), patch)
	if errors.Is(err, store.ErrNotFound) {
		c.String(http.StatusNotFound, "entry not found")
		return
	}
	if errors.Is(err, store.ErrBlocked) {
		c.String(http.StatusConflict, "entry is blocked")
		return
	}
	if errors.Is(err, store.ErrInvalid) {
		c.String(http.StatusBadRequest, "invalid entry")
		return
	}
	if err != nil {
		c.String(http.StatusInternalServerError, "failed to update entry")
		return
	}
	go s.fireWebhooks(context.Background(), currentUser(c).ID, "entry.updated", entry)
	// Cheap card audit trail — only on board cards, where it matters.
	if entry.BoardID != nil {
		if patch.Completed != nil && *patch.Completed && entry.LinkURL != "" && strings.Contains(entry.LinkURL, "github.com") {
			go func() {
				if st, err := s.store.Settings(context.Background(), currentUser(c).ID); err == nil && st.GithubToken != "" {
					ghCloseIssue(context.Background(), st.GithubToken, entry.LinkURL)
				}
			}()
		}
		if patch.Completed != nil {
			action := "completed"
			if !*patch.Completed {
				action = "reopened"
			}
			s.store.LogActivity(c.Request.Context(), currentUser(c).ID, entry.ID, action, "")
		} else if patch.Title != nil {
			s.store.LogActivity(c.Request.Context(), currentUser(c).ID, entry.ID, "renamed", *patch.Title)
		} else {
			s.store.LogActivity(c.Request.Context(), currentUser(c).ID, entry.ID, "edited", "")
		}
	}
	if patch.LinkURL != nil && entry.LinkURL != "" {
		go s.enrichLink(currentUser(c).ID, entry.ID, entry.LinkURL)
	}
	c.JSON(http.StatusOK, entry)
}

func (s *Server) deleteEntry(c *gin.Context) {
	userID, id := currentUser(c).ID, c.Param("id")
	// A synced delete writes a tombstone so the next sync can DELETE remotely.
	if accountID, href, ok, err := s.store.EntryExternalRef(c.Request.Context(), userID, id); err == nil && ok {
		_ = s.store.Tombstone(c.Request.Context(), accountID, href)
	}
	err := s.store.DeleteEntry(c.Request.Context(), userID, id)
	if err == nil {
		go s.fireWebhooks(context.Background(), userID, "entry.deleted", gin.H{"id": id})
	}
	if errors.Is(err, store.ErrNotFound) {
		c.String(http.StatusNotFound, "entry not found")
		return
	}
	if err != nil {
		c.String(http.StatusInternalServerError, "failed to delete entry")
		return
	}
	c.Status(http.StatusNoContent)
}

func (s *Server) entryRevisions(c *gin.Context) {
	revs, err := s.store.EntryRevisions(c.Request.Context(), currentUser(c).ID, c.Param("id"))
	if err != nil {
		c.String(http.StatusInternalServerError, "failed")
		return
	}
	c.JSON(http.StatusOK, revs)
}

func (s *Server) restoreRevision(c *gin.Context) {
	err := s.store.RestoreRevision(c.Request.Context(), currentUser(c).ID, c.Param("id"), c.Param("rev"))
	if errors.Is(err, store.ErrNotFound) {
		c.String(http.StatusNotFound, "revision not found")
		return
	}
	if err != nil {
		c.String(http.StatusInternalServerError, "failed")
		return
	}
	c.Status(http.StatusNoContent)
}

func (s *Server) settings(c *gin.Context) {
	settings, err := s.store.Settings(c.Request.Context(), currentUser(c).ID)
	if err != nil {
		c.String(http.StatusInternalServerError, "failed to load settings")
		return
	}
	c.JSON(http.StatusOK, settings)
}

func (s *Server) updateSettings(c *gin.Context) {
	var input store.Settings
	if !bind(c, &input) || !validSettings(input) {
		c.String(http.StatusBadRequest, "invalid settings")
		return
	}
	settings, err := s.store.UpdateSettings(c.Request.Context(), currentUser(c).ID, input)
	if err != nil {
		c.String(http.StatusInternalServerError, "failed to update settings")
		return
	}
	c.JSON(http.StatusOK, settings)
}

func (s *Server) export(c *gin.Context) {
	user := currentUser(c)
	entries, err := s.store.ListEntries(c.Request.Context(), user.ID, "", "", "")
	if err != nil {
		c.String(http.StatusInternalServerError, "failed to export entries")
		return
	}
	settings, err := s.store.Settings(c.Request.Context(), user.ID)
	if err != nil {
		c.String(http.StatusInternalServerError, "failed to export settings")
		return
	}
	people, _ := s.store.ListPeople(c.Request.Context(), user.ID)
	links, _ := s.store.AllPersonRelations(c.Request.Context(), user.ID)
	timeline, _ := s.store.AllTimeline(c.Request.Context(), user.ID)
	files, _ := s.store.ListFiles(c.Request.Context(), user.ID)
	shopLists, _ := s.store.ListShoppingLists(c.Request.Context(), user.ID)
	shopSections, _ := s.store.AllShoppingSections(c.Request.Context(), user.ID)
	shopItems, _ := s.store.AllShoppingItems(c.Request.Context(), user.ID)
	payload := gin.H{
		"exportedAt":       time.Now().UTC().Format(time.RFC3339),
		"user":             user,
		"settings":         settings,
		"entries":          entries,
		"people":           people,
		"personLinks":      links,
		"personTimeline":   timeline,
		"files":            files,
		"shoppingLists":    shopLists,
		"shoppingSections": shopSections,
		"shoppingItems":    shopItems,
	}
	if c.Query("format") != "zip" {
		c.Header("Content-Disposition", `attachment; filename="cal-export.json"`)
		c.JSON(http.StatusOK, payload)
		return
	}
	// Zip archive: cal-export.json manifest + every upload binary under
	// files/<stored-name>. A row whose binary is missing on disk stays in the
	// manifest (restore counts it as skipped) but gets no archive member.
	c.Header("Content-Disposition", `attachment; filename="cal-export.zip"`)
	c.Header("Content-Type", "application/zip")
	zw := zip.NewWriter(c.Writer)
	mj, err := json.MarshalIndent(payload, "", "  ")
	if err == nil {
		if w, cerr := zw.Create("cal-export.json"); cerr == nil {
			_, _ = w.Write(mj)
		}
	}
	dir := filepath.Join(s.dataDir, "uploads", user.ID)
	for _, f := range files {
		if !reSafeName.MatchString(f.Name) {
			continue
		}
		src, err := os.Open(filepath.Join(dir, f.Name))
		if err != nil {
			continue
		}
		if w, cerr := zw.Create("files/" + f.Name); cerr == nil {
			_, _ = io.Copy(w, src)
		}
		_ = src.Close()
	}
	_ = zw.Close()
}

func (s *Server) holidays(c *gin.Context) {
	year, err := strconv.Atoi(c.Query("year"))
	if err != nil || year < 1970 || year > 2100 {
		c.String(http.StatusBadRequest, "invalid year")
		return
	}
	country := strings.ToUpper(strings.TrimSpace(c.Query("country")))
	if len(country) != 2 || !calendar.Supported(country) {
		c.String(http.StatusBadRequest, "unsupported country")
		return
	}
	c.JSON(http.StatusOK, s.holiday.For(country, year))
}

func (s *Server) holidayCountries(c *gin.Context) {
	c.JSON(http.StatusOK, calendar.Countries())
}

func (s *Server) requireUser(c *gin.Context) {
	// Cookie first (web), then Bearer header (native apps — WebView cookies
	// are cross-origin SameSite=Lax and never arrive), then ?session= for
	// <img src> / file URLs that can't carry headers.
	sessionID, err := c.Cookie("cal_session")
	if err != nil || sessionID == "" {
		sessionID = bearerOrQuery(c)
	}
	if sessionID == "" {
		c.String(http.StatusUnauthorized, "authentication required")
		c.Abort()
		return
	}
	user, err := s.store.UserBySession(c.Request.Context(), sessionID)
	if err != nil {
		c.String(http.StatusUnauthorized, "authentication required")
		c.Abort()
		return
	}
	c.Set("user", user)
	c.Set("sessionID", sessionID)
	s.store.TouchSession(c.Request.Context(), sessionID)
	c.Next()
}

func (s *Server) setSession(c *gin.Context, userID string) (string, bool) {
	sessionID, err := s.store.CreateSession(c.Request.Context(), userID, c.Request.UserAgent())
	if err != nil {
		c.String(http.StatusInternalServerError, "failed to create session")
		c.Abort()
		return "", false
	}
	s.writeSessionCookie(c, sessionID, int((30 * 24 * time.Hour).Seconds()))
	return sessionID, true
}

func (s *Server) writeSessionCookie(c *gin.Context, value string, maxAge int) {
	http.SetCookie(c.Writer, &http.Cookie{
		Name:     "cal_session",
		Value:    value,
		Path:     "/",
		MaxAge:   maxAge,
		HttpOnly: true,
		Secure:   s.secure,
		SameSite: http.SameSiteLaxMode,
	})
}

func bearerOrQuery(c *gin.Context) string {
	h := c.GetHeader("Authorization")
	if strings.HasPrefix(h, "Bearer ") {
		return strings.TrimSpace(strings.TrimPrefix(h, "Bearer "))
	}
	return strings.TrimSpace(c.Query("session"))
}

func currentUser(c *gin.Context) store.User {
	user, _ := c.Get("user")
	return user.(store.User)
}

func bind(c *gin.Context, target any) bool {
	decoder := json.NewDecoder(c.Request.Body)
	decoder.DisallowUnknownFields()
	return decoder.Decode(target) == nil
}

func parsePatch(raw map[string]json.RawMessage) (store.EntryPatch, bool) {
	var patch store.EntryPatch
	for key, value := range raw {
		switch key {
		case "title":
			var v string
			if json.Unmarshal(value, &v) != nil || strings.TrimSpace(v) == "" {
				return patch, false
			}
			patch.Title = &v
		case "content":
			var v string
			if json.Unmarshal(value, &v) != nil {
				return patch, false
			}
			patch.Content = &v
		case "type":
			var v string
			if json.Unmarshal(value, &v) != nil || !validType(v) {
				return patch, false
			}
			patch.Type = &v
		case "linkUrl":
			var v string
			if json.Unmarshal(value, &v) != nil {
				return patch, false
			}
			patch.LinkURL = &v
		case "date":
			var v string
			if json.Unmarshal(value, &v) != nil || !validDate(v) {
				return patch, false
			}
			patch.Date = &v
		case "startTime":
			var v string
			if json.Unmarshal(value, &v) != nil || !validTime(v) {
				return patch, false
			}
			patch.StartTime = &v
		case "endTime":
			var v string
			if json.Unmarshal(value, &v) != nil || !validTime(v) {
				return patch, false
			}
			patch.EndTime = &v
		case "recur":
			var v string
			if json.Unmarshal(value, &v) != nil || !validRecur(v) {
				return patch, false
			}
			patch.Recur = &v
		case "completed":
			var v bool
			if json.Unmarshal(value, &v) != nil {
				return patch, false
			}
			patch.Completed = &v
		case "pinned":
			var v bool
			if json.Unmarshal(value, &v) != nil {
				return patch, false
			}
			patch.Pinned = &v
		case "watched":
			var v bool
			if json.Unmarshal(value, &v) != nil {
				return patch, false
			}
			patch.Watched = &v
		case "boardId", "columnId":
			var v *string
			if json.Unmarshal(value, &v) != nil {
				return patch, false
			}
			if v != nil && *v == "" {
				v = nil // empty string clears the assignment
			}
			if key == "boardId" {
				patch.BoardID = v
				if v == nil {
					patch.ClearBoard = true // explicit clear: leave the board
				}
			} else {
				patch.ColumnID = v
			}
		case "color":
			var v string
			if json.Unmarshal(value, &v) != nil {
				return patch, false
			}
			patch.Color = &v
		case "tags":
			var v []string
			if json.Unmarshal(value, &v) != nil {
				return patch, false
			}
			patch.Tags = v
			patch.HasTags = true
		case "workspaceId", "blockedBy":
			var v *string
			if json.Unmarshal(value, &v) != nil {
				return patch, false
			}
			// null and "" both mean "clear"; uuid sets.
			v2 := ""
			if v != nil {
				v2 = *v
			}
			if key == "workspaceId" {
				patch.WorkspaceID = &v2
			} else {
				patch.BlockedBy = &v2
			}
		default:
			return patch, false
		}
	}
	return patch, true
}

func validAuth(input authRequest) bool {
	return strings.Contains(input.Email, "@") && len(input.Email) <= 254 && len(input.Password) >= 8 && len(input.Password) <= 128
}

func validEntry(input store.EntryInput) bool {
	if strings.TrimSpace(input.Title) == "" || !validType(input.Type) || !validDate(input.Date) {
		return false
	}
	if !validTime(input.StartTime) || !validTime(input.EndTime) {
		return false
	}
	if input.StartTime == "" && input.EndTime != "" {
		return false
	}
	if input.StartTime != "" && input.EndTime != "" && input.EndTime <= input.StartTime {
		return false
	}
	if input.Recur != "" && (!validRecur(input.Recur) || input.Type != "task") {
		return false
	}
	return true
}

func validType(typ string) bool {
	return typ == "task" || typ == "note" || typ == "link" || typ == "event"
}

func validDate(date string) bool {
	_, err := time.Parse(time.DateOnly, date)
	return err == nil
}

// validTime accepts "" (unset) or an HH:MM 24h clock time.
func validTime(v string) bool {
	if v == "" {
		return true
	}
	_, err := time.Parse("15:04", v)
	return err == nil
}

func validRecur(v string) bool {
	switch v {
	case "none", "daily", "weekly", "monthly", "yearly":
		return true
	}
	return false
}

func validSettings(settings store.Settings) bool {
	theme := settings.Theme
	validTheme := theme == "light" || theme == "dark" || theme == "system"
	if settings.Timezone != "" {
		if _, err := time.LoadLocation(settings.Timezone); err != nil {
			return false
		}
	}
	if settings.DigestTime != "" && !validTime(settings.DigestTime) {
		return false
	}
	if settings.InvidiousURL != "" {
		u, err := url.Parse(settings.InvidiousURL)
		if err != nil || len(settings.InvidiousURL) > 500 ||
			(u.Scheme != "https" && u.Scheme != "http") || u.Host == "" {
			return false
		}
	}
	validView := settings.DefaultView == "" || settings.DefaultView == "month" ||
		settings.DefaultView == "week" || settings.DefaultView == "day"
	return len(settings.Country) == 2 && validTheme && validView &&
		(settings.WeekStart == "monday" || settings.WeekStart == "sunday")
}

func currentSessionID(c *gin.Context) string {
	if v, ok := c.Get("sessionID"); ok {
		if id, ok := v.(string); ok {
			return id
		}
	}
	return ""
}

func (s *Server) listSessions(c *gin.Context) {
	sessions, err := s.store.ListSessions(c.Request.Context(), currentUser(c).ID, currentSessionID(c))
	if err != nil {
		c.String(http.StatusInternalServerError, "failed")
		return
	}
	c.JSON(http.StatusOK, sessions)
}

func (s *Server) revokeSession(c *gin.Context) {
	if err := s.store.RevokeSession(c.Request.Context(), currentUser(c).ID, c.Param("id")); err != nil {
		c.String(http.StatusInternalServerError, "failed")
		return
	}
	c.Status(http.StatusNoContent)
}

func (s *Server) changePassword(c *gin.Context) {
	var body struct {
		Current  string `json:"current"`
		Password string `json:"password"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.String(http.StatusBadRequest, "invalid request")
		return
	}
	if err := auth.ValidatePassword(body.Password); err != nil {
		c.String(http.StatusBadRequest, err.Error())
		return
	}
	user, hash, err := s.store.UserByEmail(c.Request.Context(), currentUser(c).Email)
	if err != nil || !auth.VerifyPassword(body.Current, hash) {
		c.String(http.StatusForbidden, "current password is wrong")
		return
	}
	newHash, err := auth.HashPassword(body.Password)
	if err != nil {
		c.String(http.StatusInternalServerError, "failed")
		return
	}
	if err := s.store.ChangePassword(c.Request.Context(), user.ID, newHash); err != nil {
		c.String(http.StatusInternalServerError, "failed")
		return
	}
	// Sign out every other session — a password change invalidates them.
	_ = s.store.RevokeOtherSessions(c.Request.Context(), user.ID, currentSessionID(c))
	c.Status(http.StatusNoContent)
}

func securityHeaders() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("X-Content-Type-Options", "nosniff")
		c.Header("X-Frame-Options", "DENY")
		c.Header("Referrer-Policy", "no-referrer")
		c.Header("Content-Security-Policy", "default-src 'self'")
		c.Next()
	}
}

// trustedProxies parses a comma-separated list of proxy IPs/CIDRs. Empty
// input returns nil (trust no proxy).
func trustedProxies(raw string) ([]string, error) {
	var out []string
	for _, p := range strings.Split(raw, ",") {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if net.ParseIP(p) == nil {
			if _, _, err := net.ParseCIDR(p); err != nil {
				return nil, fmt.Errorf("invalid IP or CIDR %q", p)
			}
		}
		out = append(out, p)
	}
	return out, nil
}

func env(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
