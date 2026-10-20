package httpapi

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestShopping(t *testing.T) {
	srv, session := newTestServer(t)

	// List CRUD.
	created := doJSON(t, srv, http.MethodPost, "/api/shopping/lists", `{"name":"Groceries","icon":"cart"}`, session)
	var list struct {
		ID    string `json:"id"`
		Open  int    `json:"open"`
		Items int    `json:"items"`
	}
	if err := json.Unmarshal([]byte(created), &list); err != nil || list.ID == "" {
		t.Fatalf("create list: %s", created)
	}

	// Section + items.
	sec := doJSON(t, srv, http.MethodPost, "/api/shopping/lists/"+list.ID+"/sections", `{"name":"Dairy"}`, session)
	var section struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal([]byte(sec), &section); err != nil || section.ID == "" {
		t.Fatalf("create section: %s", sec)
	}
	doJSON(t, srv, http.MethodPost, "/api/shopping/lists/"+list.ID+"/items",
		`{"name":"Milk","quantity":"2","sectionId":"`+section.ID+`"}`, session)
	item := doJSON(t, srv, http.MethodPost, "/api/shopping/lists/"+list.ID+"/items",
		`{"name":"Bread","note":"sourdough"}`, session)
	var it struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal([]byte(item), &it); err != nil || it.ID == "" {
		t.Fatalf("create item: %s", item)
	}

	// Detail fetch returns both.
	detail := doJSON(t, srv, http.MethodGet, "/api/shopping/lists/"+list.ID, "", session)
	if !strings.Contains(detail, "Milk") || !strings.Contains(detail, "Dairy") {
		t.Fatalf("detail: %s", detail)
	}

	// Check + uncertain flags round-trip.
	got := doJSON(t, srv, http.MethodPatch, "/api/shopping/items/"+it.ID,
		`{"checked":true,"uncertain":false}`, session)
	if !strings.Contains(got, `"checked":true`) {
		t.Fatalf("patch item: %s", got)
	}

	// Suggestions see the new items.
	sug := doJSON(t, srv, http.MethodGet, "/api/shopping/suggest?q=mil", "", session)
	if !strings.Contains(sug, "Milk") {
		t.Fatalf("suggest: %s", sug)
	}

	// Bulk section check marks Milk too.
	if code, _ := doJSONStatus(t, srv, http.MethodPost, "/api/shopping/sections/"+section.ID+"/check",
		`{"checked":true}`, session); code != http.StatusNoContent {
		t.Fatalf("section check: %d", code)
	}

	// Snapshot now — the export must carry lists, sections and items.
	exp := doJSON(t, srv, http.MethodGet, "/api/export", "", session)
	if !strings.Contains(exp, "shoppingLists") || !strings.Contains(exp, "Groceries") {
		t.Fatalf("export missing shopping: %d bytes", len(exp))
	}

	// Dry-run against the still-present data reports everything existing.
	preview := doJSON(t, srv, http.MethodPost, "/api/restore?dry=1", exp, session)
	if !strings.Contains(preview, "shoppingLists") || !strings.Contains(preview, `"existing":1`) {
		t.Fatalf("preview: %s", preview)
	}

	// Clear purchased removes both checked rows.
	cleared := doJSON(t, srv, http.MethodPost, "/api/shopping/lists/"+list.ID+"/clear", `{}`, session)
	if !strings.Contains(cleared, `"removed":2`) {
		t.Fatalf("clear: %s", cleared)
	}

	// Ownership: a second user must not see the first user's list.
	openRegistration(t, srv, session)
	code, regBody := doJSONStatus(t, srv, http.MethodPost, "/api/auth/register",
		`{"email":"other@cal.local","password":"test-pass-123"}`, "")
	if code != http.StatusOK {
		t.Fatalf("second register: %d %s", code, regBody)
	}
	var reg2 struct {
		Session string `json:"session"`
	}
	_ = json.Unmarshal([]byte(regBody), &reg2)
	if code, _ := doJSONStatus(t, srv, http.MethodGet, "/api/shopping/lists/"+list.ID, "", reg2.Session); code != http.StatusNotFound {
		t.Fatalf("foreign list must 404, got %d", code)
	}

	// Same-account round-trip: delete the list (children cascade), restore
	// the export — list, section and both items come back.
	if code, _ := doJSONStatus(t, srv, http.MethodDelete, "/api/shopping/lists/"+list.ID, "", session); code != http.StatusNoContent {
		t.Fatalf("delete list: %d", code)
	}
	restored := doJSON(t, srv, http.MethodPost, "/api/restore", exp, session)
	if !strings.Contains(restored, `"shoppingLists":1`) ||
		!strings.Contains(restored, `"shoppingSections":1`) ||
		!strings.Contains(restored, `"shoppingItems":2`) {
		t.Fatalf("restore: %s", restored)
	}
	back := doJSON(t, srv, http.MethodGet, "/api/shopping/lists/"+list.ID, "", session)
	if !strings.Contains(back, "Milk") || !strings.Contains(back, "Dairy") {
		t.Fatalf("restored detail: %s", back)
	}
}
