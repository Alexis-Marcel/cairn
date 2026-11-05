package server

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const mcpToken = "test-token"

type bearerTransport struct{ token string }

func (b bearerTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	r = r.Clone(r.Context())
	r.Header.Set("Authorization", "Bearer "+b.token)
	return http.DefaultTransport.RoundTrip(r)
}

// mcpSession serves a Cairn with a few operations and connects to its /mcp.
func mcpSession(t *testing.T) (*client, *mcp.ClientSession) {
	t.Helper()
	s := newServer(t)
	s.MCPToken = mcpToken
	h := s.Handler(nil)
	c := &client{t: t, h: h, st: s.St}
	c.add(map[string]string{"kind": "buy", "date": "2024-01-10", "newAccount": "Binance", "asset": "btc", "qty": "0,1", "total": "4000"})
	ts := httptest.NewServer(h)
	t.Cleanup(ts.Close)
	session, err := mcp.NewClient(&mcp.Implementation{Name: "test"}, nil).Connect(context.Background(),
		&mcp.StreamableClientTransport{Endpoint: ts.URL + "/mcp", HTTPClient: &http.Client{Transport: bearerTransport{mcpToken}}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { session.Close() })
	return c, session
}

func call(t *testing.T, s *mcp.ClientSession, name string, args map[string]any) *mcp.CallToolResult {
	t.Helper()
	res, err := s.CallTool(context.Background(), &mcp.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	return res
}

func text(t *testing.T, res *mcp.CallToolResult) string {
	t.Helper()
	if len(res.Content) != 1 {
		t.Fatalf("content: %+v", res.Content)
	}
	return res.Content[0].(*mcp.TextContent).Text
}

func TestMCPToolsAreAllReads(t *testing.T) {
	_, s := mcpSession(t)
	list, err := s.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, tool := range list.Tools {
		names = append(names, tool.Name)
		if tool.Annotations == nil || !tool.Annotations.ReadOnlyHint {
			t.Errorf("%s is not marked read-only", tool.Name)
		}
	}
	slices.Sort(names)
	want := []string{"get_asset", "get_history", "get_overview", "get_plan", "get_portfolio", "get_stats", "get_tax_fr",
		"list_accounts", "list_operations"}
	if !slices.Equal(names, want) {
		t.Fatalf("tools: %v", names)
	}
}

func TestMCPAnswersWhatTheAPIAnswers(t *testing.T) {
	c, s := mcpSession(t)
	for _, tc := range []struct {
		tool string
		args map[string]any
		path string
	}{
		{"get_overview", map[string]any{"scope": "crypto"}, "/api/overview?scope=crypto"},
		{"get_portfolio", nil, "/api/portfolio"},
		{"get_history", map[string]any{"step": "month", "from": "2024-01-01"}, "/api/portfolio/history?step=month&from=2024-01-01"},
		{"get_stats", nil, "/api/stats"},
		{"get_asset", map[string]any{"symbol": "BTC"}, "/api/assets/BTC"},
		{"get_tax_fr", map[string]any{"year": 2024, "rewards_as_income": true}, "/api/tax/fr?year=2024&rewards=income"},
		{"list_operations", map[string]any{"asset": "BTC", "limit": 1}, "/api/operations?asset=BTC&limit=1"},
		{"list_accounts", nil, "/api/accounts"},
		{"get_plan", nil, "/api/plan"},
	} {
		res := call(t, s, tc.tool, tc.args)
		if res.IsError {
			t.Errorf("%s: %s", tc.tool, text(t, res))
			continue
		}
		w := httptest.NewRecorder()
		c.h.ServeHTTP(w, httptest.NewRequest("GET", tc.path, nil))
		if w.Code != 200 {
			t.Fatalf("%s: %d", tc.path, w.Code)
		}
		var got, want any
		if err := json.Unmarshal([]byte(text(t, res)), &got); err != nil {
			t.Fatalf("%s: %v", tc.tool, err)
		}
		json.Unmarshal(w.Body.Bytes(), &want)
		g, _ := json.Marshal(got)
		x, _ := json.Marshal(want)
		if string(g) != string(x) {
			t.Errorf("%s:\n got %s\nwant %s", tc.tool, g, x)
		}
	}
}

func TestMCPSaysTheAPIsRefusal(t *testing.T) {
	_, s := mcpSession(t)
	res := call(t, s, "get_overview", map[string]any{"scope": "nope"})
	if !res.IsError {
		t.Fatalf("no error: %s", text(t, res))
	}
	res = call(t, s, "get_asset", map[string]any{"symbol": "../operations"})
	if !res.IsError {
		t.Fatalf("left the asset's route: %s", text(t, res))
	}
}

func mcpPost(h http.Handler, authorization string) int {
	r := httptest.NewRequest("POST", "/mcp", strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"tools/list"}`))
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Accept", "application/json, text/event-stream")
	if authorization != "" {
		r.Header.Set("Authorization", authorization)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	io.Copy(io.Discard, w.Body)
	return w.Code
}

func TestMCPNeedsTheToken(t *testing.T) {
	s := newServer(t)
	s.MCPToken = mcpToken
	h := s.Handler(nil)
	for _, a := range []string{"", "Bearer wrong", mcpToken, "Basic " + mcpToken} {
		if code := mcpPost(h, a); code != http.StatusUnauthorized {
			t.Errorf("%q: %d", a, code)
		}
	}
}

// Without a token /mcp does not exist: not even the page, which answers any
// other path.
func TestMCPIsOffWithoutAToken(t *testing.T) {
	h := newServer(t).Handler(fstest.MapFS{"index.html": {Data: []byte("page")}})
	for _, a := range []string{"", "Bearer ", "Bearer anything"} {
		if code := mcpPost(h, a); code != http.StatusNotFound {
			t.Errorf("%q: %d", a, code)
		}
	}
}
