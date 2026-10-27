package httpapi

// Minimal MCP (Model Context Protocol) server over streamable HTTP.
// Implements initialize / ping / tools/list / tools/call with bearer-token
// auth, so any MCP client can drive a Cal instance directly.

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"cal/apps/api/internal/store"

	"github.com/gin-gonic/gin"
)

const mcpProtocol = "2025-03-26"

type rpcRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

func rpcResult(id json.RawMessage, result any) gin.H {
	return gin.H{"jsonrpc": "2.0", "id": id, "result": result}
}

func rpcError(id json.RawMessage, code int, msg string) gin.H {
	return gin.H{"jsonrpc": "2.0", "id": id, "error": gin.H{"code": code, "message": msg}}
}

func toolText(text string, isErr bool) gin.H {
	return gin.H{"content": []gin.H{{"type": "text", "text": text}}, "isError": isErr}
}

var mcpTools = []gin.H{
	{
		"name":        "list_entries",
		"description": "List planner entries (tasks, notes, links, events) in a date range or by search query. Without a range, undated tasks are included (they have no date field).",
		"inputSchema": gin.H{
			"type": "object",
			"properties": gin.H{
				"from":    gin.H{"type": "string", "description": "Start date YYYY-MM-DD (optional)"},
				"to":      gin.H{"type": "string", "description": "End date YYYY-MM-DD (optional)"},
				"q":       gin.H{"type": "string", "description": "Search query (optional)"},
				"undated": gin.H{"type": "boolean", "description": "Also include undated tasks when a range is given (optional)"},
			},
		},
	},
	{
		"name":        "today",
		"description": "Get today's entries: the daily agenda.",
		"inputSchema": gin.H{"type": "object", "properties": gin.H{}},
	},
	{
		"name":        "create_entry",
		"description": "Create a task, note, link or event. Times are HH:MM, date is YYYY-MM-DD. The date is required except for tasks: a task without one is undated and can't have times, recur or remind. Recur: none|daily|weekly|monthly|yearly (tasks only). remind: minutes before start.",
		"inputSchema": gin.H{
			"type": "object",
			"properties": gin.H{
				"title":     gin.H{"type": "string"},
				"type":      gin.H{"type": "string", "enum": []string{"task", "note", "link", "event"}},
				"date":      gin.H{"type": "string", "description": "YYYY-MM-DD"},
				"startTime": gin.H{"type": "string", "description": "HH:MM"},
				"endTime":   gin.H{"type": "string", "description": "HH:MM"},
				"color":     gin.H{"type": "string", "enum": paletteColors},
				"tags":      gin.H{"type": "array", "items": gin.H{"type": "string"}},
				"content":   gin.H{"type": "string"},
				"linkUrl":   gin.H{"type": "string"},
				"recur":     gin.H{"type": "string", "enum": []string{"none", "daily", "weekly", "monthly", "yearly"}},
				"remind":    gin.H{"type": "integer", "description": "minutes before start to remind"},
			},
			"required": []string{"title", "type"},
		},
	},
	{
		"name":        "update_entry",
		"description": "Patch an entry. Only provided fields change. Set completed=true on a recurring task to spawn the next occurrence.",
		"inputSchema": gin.H{
			"type": "object",
			"properties": gin.H{
				"id":        gin.H{"type": "string"},
				"title":     gin.H{"type": "string"},
				"date":      gin.H{"type": "string", "description": "YYYY-MM-DD; \"\" (or null) makes a task undated"},
				"startTime": gin.H{"type": "string"},
				"endTime":   gin.H{"type": "string"},
				"completed": gin.H{"type": "boolean"},
				"color":     gin.H{"type": "string"},
				"tags":      gin.H{"type": "array", "items": gin.H{"type": "string"}},
				"content":   gin.H{"type": "string"},
				"recur":     gin.H{"type": "string"},
				"remind":    gin.H{"type": "integer"},
			},
			"required": []string{"id"},
		},
	},
	{
		"name":        "delete_entry",
		"description": "Delete an entry by id.",
		"inputSchema": gin.H{
			"type":       "object",
			"properties": gin.H{"id": gin.H{"type": "string"}},
			"required":   []string{"id"},
		},
	},
	{
		"name":        "list_feeds",
		"description": "List subscribed external calendar feeds.",
		"inputSchema": gin.H{"type": "object", "properties": gin.H{}},
	},
	{
		"name":        "get_entry",
		"description": "Get a single entry with its full content (e.g. a note's markdown body).",
		"inputSchema": gin.H{
			"type":       "object",
			"properties": gin.H{"id": gin.H{"type": "string"}},
			"required":   []string{"id"},
		},
	},
	{
		"name":        "append_note",
		"description": "Append markdown text to a note's body (or any entry's content). The user never has to open the editor.",
		"inputSchema": gin.H{
			"type": "object",
			"properties": gin.H{
				"id":   gin.H{"type": "string"},
				"text": gin.H{"type": "string", "description": "Markdown to append"},
			},
			"required": []string{"id", "text"},
		},
	},
	{
		"name":        "search_entries",
		"description": "Full-text search across titles and content; matches tags exactly.",
		"inputSchema": gin.H{
			"type":       "object",
			"properties": gin.H{"q": gin.H{"type": "string"}},
			"required":   []string{"q"},
		},
	},
	{
		"name":        "list_accounts",
		"description": "List connected CalDAV calendar accounts.",
		"inputSchema": gin.H{"type": "object", "properties": gin.H{}},
	},
	{
		"name":        "complete_task",
		"description": "Mark a task done; recurring tasks spawn their next occurrence.",
		"inputSchema": gin.H{
			"type":       "object",
			"properties": gin.H{"id": gin.H{"type": "string"}},
			"required":   []string{"id"},
		},
	},
	{
		"name":        "weekly_review",
		"description": "Digest of the past week: tasks done, slipped, notes written, streak, busiest day.",
		"inputSchema": gin.H{"type": "object", "properties": gin.H{}},
	},
	{
		"name":        "list_habits",
		"description": "Habit streaks for recurring tasks tagged #habit.",
		"inputSchema": gin.H{"type": "object", "properties": gin.H{}},
	},
	{
		"name":        "list_files",
		"description": "List uploaded files with their public share tokens.",
		"inputSchema": gin.H{"type": "object", "properties": gin.H{}},
	},
	{
		"name":        "list_boards",
		"description": "List kanban boards.",
		"inputSchema": gin.H{"type": "object", "properties": gin.H{}},
	},
	{
		"name":        "start_timer",
		"description": "Start the focus timer, optionally on a task entry or project board.",
		"inputSchema": gin.H{
			"type": "object",
			"properties": gin.H{
				"entryId":   gin.H{"type": "string"},
				"note":      gin.H{"type": "string"},
				"planned":   gin.H{"type": "integer", "description": "pomodoro minutes"},
				"billable":  gin.H{"type": "boolean"},
				"projectId": gin.H{"type": "string", "description": "board id"},
			},
		},
	},
	{
		"name":        "stop_timer",
		"description": "Stop the running focus timer.",
		"inputSchema": gin.H{"type": "object", "properties": gin.H{}},
	},
	{
		"name":        "time_summary",
		"description": "Minutes tracked today/this week + billable amount + per-task breakdown.",
		"inputSchema": gin.H{"type": "object", "properties": gin.H{}},
	},
	{
		"name":        "github_inbox",
		"description": "Open GitHub issues and PRs involving the user (needs a PAT in settings).",
		"inputSchema": gin.H{"type": "object", "properties": gin.H{}},
	},
	{
		"name":        "board_view",
		"description": "Columns and cards of a board.",
		"inputSchema": gin.H{
			"type":       "object",
			"properties": gin.H{"boardId": gin.H{"type": "string"}},
			"required":   []string{"boardId"},
		},
	},
	{
		"name":        "create_card",
		"description": "Create a task on a board column (title required; date defaults to today).",
		"inputSchema": gin.H{
			"type": "object",
			"properties": gin.H{
				"boardId":  gin.H{"type": "string"},
				"columnId": gin.H{"type": "string"},
				"title":    gin.H{"type": "string"},
				"date":     gin.H{"type": "string"},
				"content":  gin.H{"type": "string"},
			},
			"required": []string{"boardId", "title"},
		},
	},
	{
		"name":        "move_card",
		"description": "Move a card to another column (position optional).",
		"inputSchema": gin.H{
			"type": "object",
			"properties": gin.H{
				"id":       gin.H{"type": "string"},
				"columnId": gin.H{"type": "string"},
				"position": gin.H{"type": "number"},
			},
			"required": []string{"id", "columnId"},
		},
	},
	{
		"name":        "list_people",
		"description": "List people (contacts with birthdays, namedays, notes). Optional q filters by name/nickname/relation/notes/tags.",
		"inputSchema": gin.H{
			"type": "object",
			"properties": gin.H{
				"q": gin.H{"type": "string", "description": "Search query (optional)"},
			},
		},
	},
	{
		"name":        "create_person",
		"description": "Create a person. dates[] are named yearly dates (birthdays, namedays); remindDays on a date pushes a reminder that many days ahead. birthdayRemind does the same for the main birthday.",
		"inputSchema": gin.H{
			"type": "object",
			"properties": gin.H{
				"name":           gin.H{"type": "string"},
				"relation":       gin.H{"type": "string", "description": "family|friend|partner|colleague|acquaintance or free text"},
				"birthday":       gin.H{"type": "string", "description": "YYYY-MM-DD"},
				"birthdayRemind": gin.H{"type": "integer", "description": "days ahead to remind (0 = off)"},
				"nickname":       gin.H{"type": "string"},
				"phone":          gin.H{"type": "string"},
				"email":          gin.H{"type": "string"},
				"notes":          gin.H{"type": "string"},
				"tags":           gin.H{"type": "array", "items": gin.H{"type": "string"}},
				"dates": gin.H{"type": "array", "items": gin.H{
					"type": "object",
					"properties": gin.H{
						"label":      gin.H{"type": "string"},
						"date":       gin.H{"type": "string", "description": "YYYY-MM-DD"},
						"remindDays": gin.H{"type": "integer"},
					},
					"required": []string{"label", "date"},
				}},
			},
			"required": []string{"name"},
		},
	},
	{
		"name":        "update_person",
		"description": "Patch a person. Only provided fields change; arrays (dates, tags, fields, links) replace wholesale when given.",
		"inputSchema": gin.H{
			"type": "object",
			"properties": gin.H{
				"id":             gin.H{"type": "string"},
				"name":           gin.H{"type": "string"},
				"relation":       gin.H{"type": "string"},
				"birthday":       gin.H{"type": "string"},
				"birthdayRemind": gin.H{"type": "integer"},
				"nickname":       gin.H{"type": "string"},
				"phone":          gin.H{"type": "string"},
				"email":          gin.H{"type": "string"},
				"address":        gin.H{"type": "string"},
				"notes":          gin.H{"type": "string"},
				"giftIdeas":      gin.H{"type": "string"},
				"interests":      gin.H{"type": "string"},
				"isFavorite":     gin.H{"type": "boolean"},
				"tags":           gin.H{"type": "array", "items": gin.H{"type": "string"}},
				"dates":          gin.H{"type": "array", "items": gin.H{"type": "object"}},
				"fields":         gin.H{"type": "array", "items": gin.H{"type": "object"}},
				"links":          gin.H{"type": "array", "items": gin.H{"type": "object"}},
			},
			"required": []string{"id"},
		},
	},
	{
		"name":        "person_upcoming",
		"description": "Upcoming birthdays and named dates across all people within `days` (default 60), sorted soonest first.",
		"inputSchema": gin.H{
			"type": "object",
			"properties": gin.H{
				"days": gin.H{"type": "integer", "description": "lookahead window, default 60"},
			},
		},
	},
	{
		"name":        "delete_person",
		"description": "Delete a person and their timeline/links by id.",
		"inputSchema": gin.H{
			"type":       "object",
			"properties": gin.H{"id": gin.H{"type": "string"}},
			"required":   []string{"id"},
		},
	},
	{
		"name":        "list_person_relations",
		"description": "List relationship links between people. With personId, only links touching that person (both directions); without, every link in the account.",
		"inputSchema": gin.H{
			"type": "object",
			"properties": gin.H{
				"personId": gin.H{"type": "string", "description": "optional - scope to one person"},
			},
		},
	},
	{
		"name":        "link_people",
		"description": "Create a directed relationship from one person to another. kind: parent|child|sibling|partner|friend|coworker|mentor.",
		"inputSchema": gin.H{
			"type": "object",
			"properties": gin.H{
				"fromId": gin.H{"type": "string"},
				"toId":   gin.H{"type": "string"},
				"kind":   gin.H{"type": "string", "enum": []string{"parent", "child", "sibling", "partner", "friend", "coworker", "mentor"}},
			},
			"required": []string{"fromId", "toId", "kind"},
		},
	},
	{
		"name":        "unlink_people",
		"description": "Remove a relationship link by its id (from list_person_relations).",
		"inputSchema": gin.H{
			"type":       "object",
			"properties": gin.H{"linkId": gin.H{"type": "string"}},
			"required":   []string{"linkId"},
		},
	},
	{
		"name":        "person_timeline",
		"description": "List a person's timeline items (met, gift, trip, achievement, memory, note), newest first.",
		"inputSchema": gin.H{
			"type":       "object",
			"properties": gin.H{"personId": gin.H{"type": "string"}},
			"required":   []string{"personId"},
		},
	},
	{
		"name":        "add_timeline_item",
		"description": "Add a timeline item to a person. type: met|gift|trip|achievement|memory|note (default note). occurredOn is YYYY-MM-DD (optional).",
		"inputSchema": gin.H{
			"type": "object",
			"properties": gin.H{
				"personId":   gin.H{"type": "string"},
				"type":       gin.H{"type": "string", "enum": []string{"met", "gift", "trip", "achievement", "memory", "note"}},
				"title":      gin.H{"type": "string"},
				"body":       gin.H{"type": "string"},
				"occurredOn": gin.H{"type": "string", "description": "YYYY-MM-DD"},
			},
			"required": []string{"personId", "title"},
		},
	},
	{
		"name":        "update_timeline_item",
		"description": "Replace a timeline item's fields (full update - type, title, body, occurredOn all sent).",
		"inputSchema": gin.H{
			"type": "object",
			"properties": gin.H{
				"itemId":     gin.H{"type": "string"},
				"type":       gin.H{"type": "string", "enum": []string{"met", "gift", "trip", "achievement", "memory", "note"}},
				"title":      gin.H{"type": "string"},
				"body":       gin.H{"type": "string"},
				"occurredOn": gin.H{"type": "string", "description": "YYYY-MM-DD"},
			},
			"required": []string{"itemId", "title"},
		},
	},
	{
		"name":        "delete_timeline_item",
		"description": "Delete a timeline item by id.",
		"inputSchema": gin.H{
			"type":       "object",
			"properties": gin.H{"itemId": gin.H{"type": "string"}},
			"required":   []string{"itemId"},
		},
	},
}

func (s *Server) mcpAuth(c *gin.Context) (store.User, bool) {
	token := strings.TrimPrefix(c.GetHeader("Authorization"), "Bearer ")
	if token == "" || token == c.GetHeader("Authorization") {
		return store.User{}, false
	}
	user, err := s.store.UserByToken(c.Request.Context(), store.TokenAPI, token)
	if err != nil {
		return store.User{}, false
	}
	return user, true
}

func (s *Server) mcp(c *gin.Context) {
	user, ok := s.mcpAuth(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid api token"})
		return
	}
	var req rpcRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.JSONRPC != "2.0" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid JSON-RPC request"})
		return
	}

	switch {
	case req.Method == "initialize":
		c.JSON(http.StatusOK, rpcResult(req.ID, gin.H{
			"protocolVersion": mcpProtocol,
			"capabilities": gin.H{
				"tools":     gin.H{"listChanged": false},
				"resources": gin.H{"listChanged": false},
				"prompts":   gin.H{"listChanged": false},
			},
			"serverInfo": gin.H{"name": "cal", "version": "1.0.0"},
		}))
	case strings.HasPrefix(req.Method, "notifications/"):
		c.Status(http.StatusAccepted)
	case req.Method == "ping":
		c.JSON(http.StatusOK, rpcResult(req.ID, gin.H{}))
	case req.Method == "tools/list":
		c.JSON(http.StatusOK, rpcResult(req.ID, gin.H{"tools": mcpTools}))
	case req.Method == "tools/call":
		s.mcpCall(c, user, req)
	case req.Method == "resources/list":
		// One live resource per logical collection — entries themselves are
		// fetched via tools so the list stays bounded.
		c.JSON(http.StatusOK, rpcResult(req.ID, gin.H{"resources": []gin.H{
			{"uri": "cal://today", "name": "Today", "description": "Today's agenda as JSON", "mimeType": "application/json"},
			{"uri": "cal://week", "name": "This week", "description": "Entries for the current week", "mimeType": "application/json"},
			{"uri": "cal://open-tasks", "name": "Open tasks", "description": "Every incomplete task", "mimeType": "application/json"},
			{"uri": "cal://people", "name": "People", "description": "Everyone in the relationship manager", "mimeType": "application/json"},
		}}))
	case req.Method == "resources/templates/list":
		c.JSON(http.StatusOK, rpcResult(req.ID, gin.H{"resourceTemplates": []gin.H{
			{"uriTemplate": "cal://people/{id}", "name": "Person", "description": "Person profile with relations, timeline and attachments", "mimeType": "application/json"},
		}}))
	case req.Method == "resources/read":
		var p struct {
			URI string `json:"uri"`
		}
		_ = json.Unmarshal(req.Params, &p)
		ctx := c.Request.Context()
		if p.URI == "cal://people" {
			people, err := s.store.ListPeople(ctx, user.ID)
			if err != nil {
				c.JSON(http.StatusOK, rpcError(req.ID, -32603, "query failed"))
				return
			}
			data, _ := json.Marshal(people)
			c.JSON(http.StatusOK, rpcResult(req.ID, gin.H{"contents": []gin.H{
				{"uri": p.URI, "mimeType": "application/json", "text": string(data)},
			}}))
			return
		}
		if id, ok := strings.CutPrefix(p.URI, "cal://people/"); ok && id != "" {
			person, err := s.store.GetPerson(ctx, user.ID, id)
			if err != nil {
				c.JSON(http.StatusOK, rpcError(req.ID, -32602, "unknown person"))
				return
			}
			rels, _ := s.store.PersonRelations(ctx, user.ID, id)
			timeline, _ := s.store.PersonTimeline(ctx, user.ID, id)
			files, _ := s.store.FilesForPerson(ctx, user.ID, id)
			data, _ := json.Marshal(gin.H{
				"person":    person,
				"relations": rels,
				"timeline":  timeline,
				"files":     files,
			})
			c.JSON(http.StatusOK, rpcResult(req.ID, gin.H{"contents": []gin.H{
				{"uri": p.URI, "mimeType": "application/json", "text": string(data)},
			}}))
			return
		}
		var entries []store.Entry
		var err error
		day := localDay(s.store.UserLocation(ctx, user.ID))
		today := day.Format(time.DateOnly)
		switch p.URI {
		case "cal://today":
			entries, err = s.store.ListEntries(ctx, user.ID, today, today, "")
		case "cal://week":
			end := day.AddDate(0, 0, 7).Format(time.DateOnly)
			entries, err = s.store.ListEntries(ctx, user.ID, today, end, "")
		case "cal://open-tasks":
			entries, err = s.store.ListEntries(ctx, user.ID, "", "", "")
			open := entries[:0]
			for _, e := range entries {
				if e.Type == "task" && !e.Completed {
					open = append(open, e)
				}
			}
			entries = open
		default:
			c.JSON(http.StatusOK, rpcError(req.ID, -32602, "unknown resource"))
			return
		}
		if err != nil {
			c.JSON(http.StatusOK, rpcError(req.ID, -32603, "query failed"))
			return
		}
		data, _ := json.Marshal(entries)
		c.JSON(http.StatusOK, rpcResult(req.ID, gin.H{"contents": []gin.H{
			{"uri": p.URI, "mimeType": "application/json", "text": string(data)},
		}}))
	case req.Method == "prompts/list":
		c.JSON(http.StatusOK, rpcResult(req.ID, gin.H{"prompts": []gin.H{
			{"name": "daily-plan", "description": "Turn today's agenda into a realistic plan", "arguments": []gin.H{}},
			{"name": "weekly-review", "description": "Summarize the past week and propose next week's focus", "arguments": []gin.H{}},
		}}))
	case req.Method == "prompts/get":
		var p struct {
			Name string `json:"name"`
		}
		_ = json.Unmarshal(req.Params, &p)
		texts := map[string]string{
			"daily-plan":    "Here is today's agenda (cal://today). Help me plan the day: order the timed blocks realistically, flag what's overpacked, and suggest which tasks to defer. After we agree, update entries or add new ones via the tools.",
			"weekly-review": "Pull cal://week plus all open tasks (cal://open-tasks). Summarize what I completed, what slipped, and propose three focus items for next week. Offer to create the review as a note entry.",
		}
		text, ok := texts[p.Name]
		if !ok {
			c.JSON(http.StatusOK, rpcError(req.ID, -32602, "unknown prompt"))
			return
		}
		c.JSON(http.StatusOK, rpcResult(req.ID, gin.H{
			"description": p.Name,
			"messages":    []gin.H{{"role": "user", "content": gin.H{"type": "text", "text": text}}},
		}))
	default:
		c.JSON(http.StatusOK, rpcError(req.ID, -32601, "method not found"))
	}
}

func (s *Server) mcpCall(c *gin.Context, user store.User, req rpcRequest) {
	var params struct {
		Name      string          `json:"name"`
		Arguments json.RawMessage `json:"arguments"`
	}
	if err := json.Unmarshal(req.Params, &params); err != nil {
		c.JSON(http.StatusOK, rpcError(req.ID, -32602, "invalid params"))
		return
	}
	ctx := c.Request.Context()
	respond := func(result gin.H) { c.JSON(http.StatusOK, rpcResult(req.ID, result)) }
	fail := func(msg string) { respond(toolText(msg, true)) }

	switch params.Name {
	case "list_entries", "today":
		var args struct {
			From    string `json:"from"`
			To      string `json:"to"`
			Q       string `json:"q"`
			Undated bool   `json:"undated"`
		}
		_ = json.Unmarshal(params.Arguments, &args)
		if params.Name == "today" {
			today := todayIn(s.store.UserLocation(ctx, user.ID))
			args.From, args.To, args.Undated = today, today, false
		}
		entries, err := s.store.ListEntriesScoped(ctx, user.ID, args.From, args.To, args.Q, "", args.Undated)
		if err != nil {
			fail("query failed")
			return
		}
		data, _ := json.Marshal(entries)
		respond(toolText(string(data), false))

	case "create_entry":
		var input store.EntryInput
		if err := json.Unmarshal(params.Arguments, &input); err != nil {
			fail("invalid arguments")
			return
		}
		if err := validateEntryInput(&input); err != nil {
			fail(err.Error())
			return
		}
		entry, err := s.store.CreateEntry(ctx, user.ID, input)
		if err != nil {
			fail("create failed: " + err.Error())
			return
		}
		data, _ := json.Marshal(entry)
		respond(toolText(string(data), false))

	case "update_entry":
		// Args carry the patch fields alongside id.
		var raw map[string]json.RawMessage
		if err := json.Unmarshal(params.Arguments, &raw); err != nil {
			fail("invalid arguments")
			return
		}
		var id string
		if v, ok := raw["id"]; ok {
			_ = json.Unmarshal(v, &id)
			delete(raw, "id")
		}
		if id == "" {
			fail("id is required")
			return
		}
		cleaned, _ := json.Marshal(raw)
		var entryPatch store.EntryPatch
		if err := json.Unmarshal(cleaned, &entryPatch); err != nil {
			fail("invalid arguments")
			return
		}
		if _, ok := raw["tags"]; ok {
			entryPatch.HasTags = true
		}
		if d, ok := raw["date"]; ok && (string(d) == "null" || string(d) == `""`) {
			entryPatch.Date, entryPatch.ClearDate = nil, true
		}
		updated, err := s.store.UpdateEntry(ctx, user.ID, id, entryPatch)
		if errors.Is(err, store.ErrNotFound) {
			fail("entry not found")
			return
		} else if errors.Is(err, store.ErrInvalid) {
			fail("invalid update")
			return
		} else if err != nil {
			fail("update failed")
			return
		}
		data, _ := json.Marshal(updated)
		respond(toolText(string(data), false))

	case "delete_entry":
		var args struct {
			ID string `json:"id"`
		}
		_ = json.Unmarshal(params.Arguments, &args)
		if err := s.store.DeleteEntry(ctx, user.ID, args.ID); err != nil {
			fail("entry not found")
			return
		}
		respond(toolText("deleted", false))

	case "list_feeds":
		feeds, err := s.store.ListFeeds(ctx, user.ID)
		if err != nil {
			fail("query failed")
			return
		}
		data, _ := json.Marshal(feeds)
		respond(toolText(string(data), false))

	case "get_entry", "append_note":
		var args struct {
			ID   string `json:"id"`
			Text string `json:"text"`
		}
		_ = json.Unmarshal(params.Arguments, &args)
		if args.ID == "" {
			fail("id is required")
			return
		}
		entry, err := s.store.Entry(ctx, user.ID, args.ID)
		if err != nil {
			fail("entry not found")
			return
		}
		if params.Name == "get_entry" {
			data, _ := json.Marshal(entry)
			respond(toolText(string(data), false))
			return
		}
		if args.Text == "" {
			fail("text is required")
			return
		}
		body := entry.Content
		if body != "" && !strings.HasSuffix(body, "\n") {
			body += "\n"
		}
		_, err = s.store.UpdateEntry(ctx, user.ID, args.ID, store.EntryPatch{Content: &[]string{body + args.Text}[0]})
		if err != nil {
			fail("update failed")
			return
		}
		respond(toolText("appended", false))

	case "search_entries":
		var args struct {
			Q string `json:"q"`
		}
		_ = json.Unmarshal(params.Arguments, &args)
		if strings.TrimSpace(args.Q) == "" {
			fail("q is required")
			return
		}
		entries, err := s.store.ListEntries(ctx, user.ID, "", "", args.Q)
		if err != nil {
			fail("query failed")
			return
		}
		data, _ := json.Marshal(entries)
		respond(toolText(string(data), false))

	case "list_accounts":
		accounts, err := s.store.CaldavAccounts(ctx, user.ID)
		if err != nil {
			fail("query failed")
			return
		}
		data, _ := json.Marshal(accounts)
		respond(toolText(string(data), false))

	case "complete_task":
		var args struct {
			ID string `json:"id"`
		}
		_ = json.Unmarshal(params.Arguments, &args)
		entry, err := s.store.Entry(ctx, user.ID, args.ID)
		if err != nil {
			fail("entry not found")
			return
		}
		if entry.Type != "task" {
			fail("only tasks can be completed")
			return
		}
		done := true
		updated, err := s.store.UpdateEntry(ctx, user.ID, args.ID, store.EntryPatch{Completed: &done})
		if err != nil {
			fail("update failed")
			return
		}
		data, _ := json.Marshal(updated)
		respond(toolText(string(data), false))

	case "weekly_review":
		review, err := s.weekReview(ctx, user.ID)
		if err != nil {
			fail("review failed")
			return
		}
		data, _ := json.Marshal(review)
		respond(toolText(string(data), false))

	case "list_habits":
		habits, err := s.habitStreakList(ctx, user.ID)
		if err != nil {
			fail("query failed")
			return
		}
		data, _ := json.Marshal(habits)
		respond(toolText(string(data), false))

	case "list_files":
		files, err := s.store.ListFiles(ctx, user.ID)
		if err != nil {
			fail("query failed")
			return
		}
		data, _ := json.Marshal(files)
		respond(toolText(string(data), false))

	case "list_boards":
		boards, err := s.store.ListBoards(ctx, user.ID)
		if err != nil {
			fail("query failed")
			return
		}
		data, _ := json.Marshal(boards)
		respond(toolText(string(data), false))

	case "start_timer":
		var args struct {
			EntryID   string `json:"entryId"`
			Note      string `json:"note"`
			Planned   int    `json:"planned"`
			Billable  bool   `json:"billable"`
			ProjectID string `json:"projectId"`
		}
		_ = json.Unmarshal(params.Arguments, &args)
		var ePtr, pPtr *string
		if args.EntryID != "" {
			ePtr = &args.EntryID
		}
		if args.ProjectID != "" {
			pPtr = &args.ProjectID
		}
		var rate *float64
		if st, err := s.store.Settings(ctx, user.ID); err == nil {
			rate = st.DefaultRate
		}
		t, err := s.store.StartTimer(ctx, user.ID, ePtr, args.Note, args.Planned, args.Billable, rate, pPtr, nil)
		if err != nil {
			fail("a timer is already running")
			return
		}
		data, _ := json.Marshal(t)
		respond(toolText(string(data), false))

	case "stop_timer":
		t, err := s.store.StopTimer(ctx, user.ID)
		if err != nil {
			fail("no running timer")
			return
		}
		data, _ := json.Marshal(t)
		respond(toolText(string(data), false))

	case "time_summary":
		sum, err := s.store.TimeSummary(ctx, user.ID)
		if err != nil {
			fail("query failed")
			return
		}
		data, _ := json.Marshal(sum)
		respond(toolText(string(data), false))

	case "github_inbox":
		token, err := s.store.GitHubToken(ctx, user.ID)
		if err != nil || token == "" {
			fail("no github token in settings")
			return
		}
		var res struct {
			Items []ghIssue `json:"items"`
		}
		if err := ghGet(ctx, token, "/search/issues?q="+url.QueryEscape("is:open involves:@me")+"&per_page=50", &res); err != nil {
			fail("github fetch failed")
			return
		}
		data, _ := json.Marshal(res.Items)
		respond(toolText(string(data), false))

	case "board_view":
		var args struct {
			BoardID string `json:"boardId"`
		}
		_ = json.Unmarshal(params.Arguments, &args)
		cols, _ := s.store.BoardColumns(ctx, user.ID, args.BoardID)
		cards, _ := s.store.BoardCards(ctx, user.ID, args.BoardID)
		data, _ := json.Marshal(gin.H{"columns": cols, "cards": cards})
		respond(toolText(string(data), false))

	case "create_card":
		var args struct {
			BoardID  string `json:"boardId"`
			ColumnID string `json:"columnId"`
			Title    string `json:"title"`
			Date     string `json:"date"`
			Content  string `json:"content"`
		}
		if err := json.Unmarshal(params.Arguments, &args); err != nil || args.BoardID == "" || strings.TrimSpace(args.Title) == "" {
			fail("boardId and title are required")
			return
		}
		if !s.store.BoardExists(ctx, user.ID, args.BoardID) {
			fail("unknown board")
			return
		}
		if args.ColumnID != "" && !s.store.ColumnInBoard(ctx, user.ID, args.BoardID, args.ColumnID) {
			fail("column not on this board")
			return
		}
		if args.Date == "" {
			args.Date = todayIn(s.store.UserLocation(ctx, user.ID))
		}
		input := store.EntryInput{Title: args.Title, Type: "task", Date: args.Date, Content: args.Content, BoardID: &args.BoardID}
		if args.ColumnID != "" {
			input.ColumnID = &args.ColumnID
		}
		// Append at the column's end.
		cards, _ := s.store.BoardCards(ctx, user.ID, args.BoardID)
		top := 0.0
		for _, card := range cards {
			if card.ColumnID != nil && args.ColumnID != "" && *card.ColumnID == args.ColumnID && card.Position != nil && *card.Position > top {
				top = *card.Position
			}
		}
		pos := top + 1024
		input.Position = &pos
		entry, err := s.store.CreateEntry(ctx, user.ID, input)
		if err != nil {
			fail("create failed")
			return
		}
		data, _ := json.Marshal(entry)
		respond(toolText(string(data), false))

	case "move_card":
		var args struct {
			ID       string  `json:"id"`
			ColumnID string  `json:"columnId"`
			Position float64 `json:"position"`
		}
		_ = json.Unmarshal(params.Arguments, &args)
		entry, err := s.store.Entry(ctx, user.ID, args.ID)
		if err != nil || entry.BoardID == nil {
			fail("card not found")
			return
		}
		if !s.store.ColumnInBoard(ctx, user.ID, *entry.BoardID, args.ColumnID) {
			fail("column not on this board")
			return
		}
		if err := s.store.MoveCard(ctx, user.ID, args.ID, *entry.BoardID, &args.ColumnID, args.Position); err != nil {
			fail("move failed")
			return
		}
		respond(toolText("moved", false))

	case "list_people":
		var args struct {
			Q string `json:"q"`
		}
		_ = json.Unmarshal(params.Arguments, &args)
		var people []store.Person
		var err error
		if strings.TrimSpace(args.Q) != "" {
			people, err = s.store.SearchPeople(ctx, user.ID, args.Q)
		} else {
			people, err = s.store.ListPeople(ctx, user.ID)
		}
		if err != nil {
			fail("query failed")
			return
		}
		data, _ := json.Marshal(people)
		respond(toolText(string(data), false))

	case "create_person":
		var input store.PersonInput
		if err := json.Unmarshal(params.Arguments, &input); err != nil {
			fail("invalid arguments")
			return
		}
		input.Name = strings.TrimSpace(input.Name)
		if !validPerson(input) {
			fail("invalid person")
			return
		}
		person, err := s.store.CreatePerson(ctx, user.ID, input)
		if err != nil {
			fail("create failed")
			return
		}
		data, _ := json.Marshal(person)
		respond(toolText(string(data), false))

	case "update_person":
		// Patch semantics: provided fields overwrite, the rest keep their
		// stored values (the store update replaces the whole row).
		var raw map[string]json.RawMessage
		if err := json.Unmarshal(params.Arguments, &raw); err != nil {
			fail("invalid arguments")
			return
		}
		var id string
		if v, ok := raw["id"]; ok {
			_ = json.Unmarshal(v, &id)
			delete(raw, "id")
		}
		if id == "" {
			fail("id is required")
			return
		}
		person, err := s.store.GetPerson(ctx, user.ID, id)
		if err != nil {
			fail("person not found")
			return
		}
		in := personToInput(person)
		apply := func(key string, dst any) {
			if v, ok := raw[key]; ok {
				_ = json.Unmarshal(v, dst)
			}
		}
		apply("name", &in.Name)
		apply("relation", &in.Relation)
		apply("birthday", &in.Birthday)
		apply("birthdayRemind", &in.BirthdayRemind)
		apply("nickname", &in.Nickname)
		apply("phone", &in.Phone)
		apply("email", &in.Email)
		apply("address", &in.Address)
		apply("notes", &in.Notes)
		apply("giftIdeas", &in.GiftIdeas)
		apply("interests", &in.Interests)
		apply("isFavorite", &in.IsFavorite)
		apply("tags", &in.Tags)
		apply("dates", &in.Dates)
		apply("fields", &in.Fields)
		apply("links", &in.Links)
		if !validPerson(in) {
			fail("invalid person")
			return
		}
		updated, err := s.store.UpdatePerson(ctx, user.ID, id, in)
		if err != nil {
			fail("update failed")
			return
		}
		data, _ := json.Marshal(updated)
		respond(toolText(string(data), false))

	case "person_upcoming":
		var args struct {
			Days int `json:"days"`
		}
		_ = json.Unmarshal(params.Arguments, &args)
		if args.Days <= 0 || args.Days > 366 {
			args.Days = 60
		}
		people, err := s.store.ListPeople(ctx, user.ID)
		if err != nil {
			fail("query failed")
			return
		}
		respond(toolText(string(mustJSON(upcomingPersonDates(people, args.Days, localDay(s.store.UserLocation(ctx, user.ID))))), false))

	case "delete_person":
		var args struct {
			ID string `json:"id"`
		}
		_ = json.Unmarshal(params.Arguments, &args)
		if err := s.store.DeletePerson(ctx, user.ID, args.ID); err != nil {
			fail("person not found")
			return
		}
		respond(toolText("deleted", false))

	case "list_person_relations":
		var args struct {
			PersonID string `json:"personId"`
		}
		_ = json.Unmarshal(params.Arguments, &args)
		var rels []store.PersonRelation
		var err error
		if args.PersonID == "" {
			rels, err = s.store.AllPersonRelations(ctx, user.ID)
		} else {
			if _, gerr := s.store.GetPerson(ctx, user.ID, args.PersonID); gerr != nil {
				fail("person not found")
				return
			}
			rels, err = s.store.PersonRelations(ctx, user.ID, args.PersonID)
		}
		if err != nil {
			fail("query failed")
			return
		}
		respond(toolText(string(mustJSON(rels)), false))

	case "link_people":
		var args struct {
			FromID string `json:"fromId"`
			ToID   string `json:"toId"`
			Kind   string `json:"kind"`
		}
		_ = json.Unmarshal(params.Arguments, &args)
		if !relationKinds[args.Kind] {
			fail("kind must be parent|child|sibling|partner|friend|coworker|mentor")
			return
		}
		if args.FromID == "" || args.ToID == "" || args.FromID == args.ToID {
			fail("fromId and toId are required and must differ")
			return
		}
		if _, err := s.store.GetPerson(ctx, user.ID, args.FromID); err != nil {
			fail("fromId: person not found")
			return
		}
		if _, err := s.store.GetPerson(ctx, user.ID, args.ToID); err != nil {
			fail("toId: person not found")
			return
		}
		rel, err := s.store.LinkPersons(ctx, user.ID, args.FromID, args.ToID, args.Kind)
		if err != nil {
			fail("link already exists")
			return
		}
		respond(toolText(string(mustJSON(rel)), false))

	case "unlink_people":
		var args struct {
			LinkID string `json:"linkId"`
		}
		_ = json.Unmarshal(params.Arguments, &args)
		if err := s.store.UnlinkPersons(ctx, user.ID, args.LinkID); err != nil {
			fail("link not found")
			return
		}
		respond(toolText("unlinked", false))

	case "person_timeline":
		var args struct {
			PersonID string `json:"personId"`
		}
		_ = json.Unmarshal(params.Arguments, &args)
		if _, err := s.store.GetPerson(ctx, user.ID, args.PersonID); err != nil {
			fail("person not found")
			return
		}
		items, err := s.store.PersonTimeline(ctx, user.ID, args.PersonID)
		if err != nil {
			fail("query failed")
			return
		}
		respond(toolText(string(mustJSON(items)), false))

	case "add_timeline_item":
		var args struct {
			PersonID string `json:"personId"`
			timelineInput
		}
		_ = json.Unmarshal(params.Arguments, &args)
		in := args.timelineInput
		in.Title = strings.TrimSpace(in.Title)
		if in.Type == "" {
			in.Type = "note"
		}
		if args.PersonID == "" || !validTimelineInput(in) {
			fail("invalid item - personId + title required; type: met|gift|trip|achievement|memory|note")
			return
		}
		if _, err := s.store.GetPerson(ctx, user.ID, args.PersonID); err != nil {
			fail("person not found")
			return
		}
		item, err := s.store.CreateTimelineItem(ctx, user.ID, args.PersonID, in.Type, in.Title, in.Body, in.OccurredOn)
		if err != nil {
			fail("create failed")
			return
		}
		respond(toolText(string(mustJSON(item)), false))

	case "update_timeline_item":
		var args struct {
			ItemID string `json:"itemId"`
			timelineInput
		}
		_ = json.Unmarshal(params.Arguments, &args)
		in := args.timelineInput
		in.Title = strings.TrimSpace(in.Title)
		if in.Type == "" {
			in.Type = "note"
		}
		if args.ItemID == "" || !validTimelineInput(in) {
			fail("invalid item - itemId + title required")
			return
		}
		item, err := s.store.UpdateTimelineItem(ctx, user.ID, args.ItemID, in.Type, in.Title, in.Body, in.OccurredOn)
		if errors.Is(err, store.ErrNotFound) {
			fail("item not found")
			return
		} else if err != nil {
			fail("update failed")
			return
		}
		respond(toolText(string(mustJSON(item)), false))

	case "delete_timeline_item":
		var args struct {
			ItemID string `json:"itemId"`
		}
		_ = json.Unmarshal(params.Arguments, &args)
		if err := s.store.DeleteTimelineItem(ctx, user.ID, args.ItemID); err != nil {
			fail("item not found")
			return
		}
		respond(toolText("deleted", false))

	default:
		c.JSON(http.StatusOK, rpcError(req.ID, -32602, "unknown tool"))
	}
}

// personToInput copies a stored person into the editable input shape.
func personToInput(p store.Person) store.PersonInput {
	in := store.PersonInput{
		Name: p.Name, Relation: p.Relation, Notes: p.Notes, Color: p.Color,
		WorkspaceID: p.WorkspaceID, Nickname: p.Nickname, Avatar: p.Avatar,
		Phone: p.Phone, Email: p.Email, Address: p.Address,
		GiftIdeas: p.GiftIdeas, Interests: p.Interests, IsFavorite: p.IsFavorite,
		Fields: p.Fields, Links: p.Links, Tags: p.Tags, Dates: p.Dates,
		BirthdayRemind: p.BirthdayRemind,
	}
	if p.Birthday != nil {
		in.Birthday = *p.Birthday
	}
	return in
}

// upcomingOccurrence is one person date resolved to its next occurrence.
type upcomingOccurrence struct {
	PersonID  string `json:"personId"`
	Name      string `json:"name"`
	Label     string `json:"label"`
	Date      string `json:"date"` // this year's occurrence
	DaysUntil int    `json:"daysUntil"`
	Turns     *int   `json:"turns,omitempty"` // age when the date has a year
}

// upcomingPersonDates expands every person's birthday + named dates to
// their next occurrence (month/day recurrence, Feb 29 → Feb 28 off-leap)
// and returns the ones within `days`, sorted soonest first.
func upcomingPersonDates(people []store.Person, days int, now time.Time) []upcomingOccurrence {
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	var out []upcomingOccurrence
	add := func(p store.Person, label, date string) {
		t, err := time.Parse(time.DateOnly, date)
		if err != nil {
			return
		}
		occ := time.Date(today.Year(), t.Month(), t.Day(), 0, 0, 0, 0, today.Location())
		if occ.Month() != t.Month() { // Feb 29 → Feb 28 off-leap
			occ = time.Date(today.Year(), t.Month(), t.Day()-1, 0, 0, 0, 0, today.Location())
		}
		if occ.Before(today) {
			occ = occ.AddDate(1, 0, 0)
		}
		daysUntil := int(occ.Sub(today).Hours() / 24)
		if daysUntil > days {
			return
		}
		u := upcomingOccurrence{
			PersonID: p.ID, Name: p.Name, Label: label,
			Date: occ.Format(time.DateOnly), DaysUntil: daysUntil,
		}
		if t.Year() > 1900 {
			turns := occ.Year() - t.Year()
			u.Turns = &turns
		}
		out = append(out, u)
	}
	for _, p := range people {
		if p.Birthday != nil {
			add(p, "birthday", *p.Birthday)
		}
		for _, d := range p.Dates {
			add(p, d.Label, d.Date)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].DaysUntil < out[j].DaysUntil })
	return out
}

func mustJSON(v any) []byte {
	data, _ := json.Marshal(v)
	return data
}

// validateEntryInput mirrors the create handler's validation for tool calls.
func validateEntryInput(input *store.EntryInput) error {
	if strings.TrimSpace(input.Title) == "" {
		return errors.New("title is required")
	}
	if input.Type == "" {
		input.Type = "task"
	}
	switch input.Type {
	case "task", "note", "link", "event":
	default:
		return errors.New("type must be task, note, link or event")
	}
	if input.Recur == "" {
		input.Recur = "none"
	}
	if input.Recur != "none" && input.Type != "task" {
		return errors.New("only tasks can recur")
	}
	if input.Date == "" && !input.CanBeUndated() {
		return errors.New("date is required, except for tasks with no times, recur or remind")
	}
	return nil
}
