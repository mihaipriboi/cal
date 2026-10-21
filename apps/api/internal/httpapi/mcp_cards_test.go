package httpapi

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// mcpCall invokes one MCP tool with the API token and returns its text and
// isError flag.
func mcpCall(t *testing.T, srv *httptest.Server, token, tool, args string) (string, bool) {
	t.Helper()
	body := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"` + tool + `","arguments":` + args + `}}`
	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/api/mcp", bytes.NewReader([]byte(body)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	res, err := srv.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	var out struct {
		Result struct {
			Content []struct {
				Text string `json:"text"`
			} `json:"content"`
			IsError bool `json:"isError"`
		} `json:"result"`
	}
	if err := json.NewDecoder(res.Body).Decode(&out); err != nil || len(out.Result.Content) == 0 {
		t.Fatalf("%s: bad response (status %d): %v", tool, res.StatusCode, err)
	}
	return out.Result.Content[0].Text, out.Result.IsError
}

// newBoard creates a kanban board and returns its id and first column id.
func newBoard(t *testing.T, srv *httptest.Server, session, name string) (string, string) {
	t.Helper()
	var board struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal([]byte(doJSON(t, srv, http.MethodPost, "/api/boards", `{"name":"`+name+`","template":"kanban"}`, session)), &board); err != nil || board.ID == "" {
		t.Fatalf("create board %s failed", name)
	}
	var view struct {
		Columns []struct {
			ID string `json:"id"`
		} `json:"columns"`
	}
	if err := json.Unmarshal([]byte(doJSON(t, srv, http.MethodGet, "/api/boards/"+board.ID+"/view", "", session)), &view); err != nil || len(view.Columns) == 0 {
		t.Fatalf("board %s has no columns", name)
	}
	return board.ID, view.Columns[0].ID
}

// MCP card tools must keep a card's column on its own board, like the REST
// handlers do.
func TestMCPCardColumnMustBelongToBoard(t *testing.T) {
	srv, session := newTestServer(t)
	var settings struct {
		APIToken string `json:"apiToken"`
	}
	if err := json.Unmarshal([]byte(doJSON(t, srv, http.MethodGet, "/api/settings", "", session)), &settings); err != nil || settings.APIToken == "" {
		t.Fatal("no API token in settings")
	}
	boardA, colA := newBoard(t, srv, session, "A")
	_, colB := newBoard(t, srv, session, "B")

	if text, isErr := mcpCall(t, srv, settings.APIToken, "create_card", `{"boardId":"`+boardA+`","columnId":"`+colB+`","title":"x"}`); !isErr {
		t.Fatalf("create_card into another board's column succeeded: %s", text)
	}
	text, isErr := mcpCall(t, srv, settings.APIToken, "create_card", `{"boardId":"`+boardA+`","columnId":"`+colA+`","title":"ok"}`)
	if isErr {
		t.Fatalf("create_card: %s", text)
	}
	var card struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal([]byte(text), &card); err != nil || card.ID == "" {
		t.Fatalf("create_card returned %s", text)
	}
	if text, isErr := mcpCall(t, srv, settings.APIToken, "move_card", `{"id":"`+card.ID+`","columnId":"`+colB+`","position":1}`); !isErr {
		t.Fatalf("move_card to another board's column succeeded: %s", text)
	}
	if text, isErr := mcpCall(t, srv, settings.APIToken, "move_card", `{"id":"`+card.ID+`","columnId":"`+colA+`","position":2}`); isErr {
		t.Fatalf("move_card within the board: %s", text)
	}
}
