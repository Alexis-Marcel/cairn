package server

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Alexis-Marcel/cairn/internal/books"
	"github.com/Alexis-Marcel/cairn/internal/csvfile"
	"github.com/Alexis-Marcel/cairn/internal/fundarr"
	"github.com/Alexis-Marcel/cairn/internal/store"
	"github.com/Alexis-Marcel/cairn/internal/testutil"
	"github.com/Alexis-Marcel/fundarr/pkg/api"
)

var now = time.Date(2025, 12, 31, 18, 0, 0, 0, time.UTC)

type client struct {
	t  *testing.T
	h  http.Handler
	st *store.Store
}

func newClient(t *testing.T) *client {
	st := testutil.Store(t)
	eng := books.New(st, testutil.Prices(st, now))
	eng.Now = func() time.Time { return now }
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	s := &Server{St: st, Books: eng, Sync: fundarr.New(st, log), Log: log, Now: func() time.Time { return now }}
	return &client{t: t, h: s.Handler(nil), st: st}
}

// do sends a request as Cairn's page would, and decodes the answer into out.
func (c *client) do(method, path string, body any, out any) int {
	c.t.Helper()
	var r io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		r = bytes.NewReader(b)
	}
	req := httptest.NewRequest(method, path, r)
	if method != "GET" {
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Sec-Fetch-Site", "same-origin")
	}
	w := httptest.NewRecorder()
	c.h.ServeHTTP(w, req)
	if out != nil {
		if err := json.Unmarshal(w.Body.Bytes(), out); err != nil {
			c.t.Fatalf("%s %s: %d %s", method, path, w.Code, w.Body.String())
		}
	}
	return w.Code
}

func (c *client) add(e map[string]string) opView {
	c.t.Helper()
	var v opView
	if code := c.do("POST", "/api/operations", e, &v); code != 201 {
		c.t.Fatalf("add %v: %d", e, code)
	}
	return v
}

type pf struct {
	Total  map[string]string `json:"total"`
	Assets []struct {
		Asset struct{ Symbol string } `json:"asset"`
		Qty   string                  `json:"qty"`
	} `json:"assets"`
	Sources []struct {
		Name  string `json:"name"`
		Value string `json:"value"`
	} `json:"sources"`
}

// Bought on an exchange, moved to a wallet, half moved back and sold: the
// figures of Fundarr's own example (pkg/portfolio), from manual entries.
func TestManualToPortfolio(t *testing.T) {
	c := newClient(t)
	buy := c.add(map[string]string{"kind": "buy", "date": "2024-01-10", "newAccount": "Binance", "asset": "btc", "qty": "0,1", "total": "4000"})
	bin := buy.Account.ID
	tr := c.add(map[string]string{"kind": "transfer", "date": "2024-02-01", "account": bin, "newTo": "Ledger", "asset": "BTC", "qty": "0.1"})
	if tr.Kind != "transfer" || !tr.Linked || tr.Peer == nil || tr.Peer.Account.Name != "Ledger" {
		t.Fatalf("transfer: %+v", tr)
	}
	led := tr.Peer.Account.ID
	c.add(map[string]string{"kind": "transfer", "date": "2025-03-01", "account": led, "to": bin, "asset": "BTC", "qty": "0.05"})
	c.add(map[string]string{"kind": "sell", "date": "2025-03-02", "account": bin, "asset": "BTC", "qty": "0.05", "total": "3000"})

	var p pf
	if code := c.do("GET", "/api/portfolio", nil, &p); code != 200 {
		t.Fatal(code)
	}
	want := map[string]string{"value": "3000.00", "cost": "2000.00", "realized": "1000.00", "invested": "1000.00", "rewards": "0.00"}
	for k, v := range want {
		if p.Total[k] != v {
			t.Errorf("%s: %s, want %s", k, p.Total[k], v)
		}
	}
	if len(p.Sources) != 1 || p.Sources[0].Name != "Ledger" {
		t.Errorf("sources: %+v", p.Sources)
	}

	var tax struct {
		Total struct {
			Gain, Tax string
			Exempt    bool
		} `json:"total"`
		Disposals []struct{ Gain string } `json:"disposals"`
		Notes     []string                `json:"notes"`
	}
	c.do("GET", "/api/tax/fr?year=2025", nil, &tax)
	if len(tax.Disposals) != 1 || tax.Total.Gain != "1000.00" || tax.Total.Tax != "314.00" || tax.Total.Exempt {
		t.Fatalf("2086: %+v", tax)
	}
	if !strings.HasPrefix(tax.Notes[0], "Les cessions") {
		t.Errorf("notes in French: %q", tax.Notes[0])
	}

	var h struct {
		Points []struct{ Date, Value, Invested string } `json:"points"`
	}
	c.do("GET", "/api/portfolio/history?step=month", nil, &h)
	if len(h.Points) != 24 || h.Points[0].Date != "2024-01-31" || h.Points[0].Value != "4000.00" || h.Points[23].Value != "3000.00" {
		t.Fatalf("history: %d %+v", len(h.Points), h.Points[0])
	}
	if c.do("GET", "/api/portfolio/history?step=hour", nil, &map[string]string{}) != 400 {
		t.Error("bad step")
	}
}

func TestEditAndDelete(t *testing.T) {
	c := newClient(t)
	tr := c.add(map[string]string{"kind": "transfer", "date": "2024-02-01", "newAccount": "A", "newTo": "B", "asset": "ETH", "qty": "2"})
	// Edited from its deposit side, it stays one transfer.
	var v opView
	if code := c.do("PUT", "/api/operations/"+tr.Peer.ID, map[string]string{"kind": "transfer", "date": "2024-02-01",
		"account": tr.Account.ID, "to": tr.Peer.Account.ID, "asset": "ETH", "qty": "1.5"}, &v); code != 200 {
		t.Fatal(code)
	}
	if v.ID != tr.ID || v.Base.Qty.String() != "1.5" || v.Peer == nil || v.Peer.ID != tr.Peer.ID {
		t.Fatalf("edited: %+v", v)
	}
	// Into a plain withdrawal: the deposit goes.
	c.do("PUT", "/api/operations/"+tr.ID, map[string]string{"kind": "withdrawal", "date": "2024-02-01", "account": tr.Account.ID, "asset": "ETH", "qty": "1"}, &v)
	var list struct{ Total int }
	c.do("GET", "/api/operations", nil, &list)
	if list.Total != 1 || v.Kind != "withdrawal" {
		t.Fatalf("%d %s", list.Total, v.Kind)
	}
	if code := c.do("DELETE", "/api/operations/"+tr.ID, map[string]string{}, nil); code != 200 {
		t.Fatal(code)
	}
	c.do("GET", "/api/operations", nil, &list)
	if list.Total != 0 {
		t.Fatal("deleted")
	}
}

func TestRefusals(t *testing.T) {
	c := newClient(t)
	var e struct{ Error string }
	if c.do("POST", "/api/operations", map[string]string{"kind": "buy", "date": "2024-01-10", "newAccount": "X", "asset": "BTC", "qty": "1"}, &e) != 400 || e.Error == "" {
		t.Fatalf("no price: %+v", e)
	}
	var accs []store.Account
	c.do("GET", "/api/accounts", nil, &accs)
	if len(accs) != 0 {
		t.Fatal("a refused entry makes no account")
	}
	if c.do("POST", "/api/operations", map[string]string{"kind": "reward", "date": "2099-01-01", "newAccount": "X", "asset": "BTC", "qty": "1"}, &e) != 400 {
		t.Fatal("future")
	}
	// A write from elsewhere, or not JSON.
	for _, h := range []map[string]string{{"Content-Type": "application/json", "Sec-Fetch-Site": "cross-site"}, {"Content-Type": "application/x-www-form-urlencoded"}} {
		req := httptest.NewRequest("POST", "/api/accounts", strings.NewReader(`{"name":"x"}`))
		for k, v := range h {
			req.Header.Set(k, v)
		}
		w := httptest.NewRecorder()
		c.h.ServeHTTP(w, req)
		if w.Code != 403 && w.Code != 415 {
			t.Errorf("%v: %d", h, w.Code)
		}
	}
	if c.do("GET", "/api/nope", nil, &e) != 404 {
		t.Error("unknown route")
	}
}

func TestFundarrReadOnly(t *testing.T) {
	c := newClient(t)
	ctx := t.Context()
	c.st.SetLink(ctx, "http://f", "t")
	raw := `{"changes":[{"op":"upsert","id":"f1","activity":{"id":"f1","sourceId":"s","time":"2024-01-01T00:00:00Z","type":"reward",
		"base":{"asset":{"kind":"crypto","symbol":"ETH"},"qty":"1"},"source":{"id":"s","name":"Kraken"}}}],"cursor":"e.1"}`
	var page api.Changes
	if err := json.Unmarshal([]byte(raw), &page); err != nil {
		t.Fatal(err)
	}
	if err := c.st.ApplyChanges(ctx, page); err != nil {
		t.Fatal(err)
	}
	var e struct{ Error string }
	if c.do("DELETE", "/api/operations/f1", map[string]string{}, &e) != 409 || !strings.Contains(e.Error, "Fundarr") {
		t.Fatalf("%+v", e)
	}
	var v opView
	c.do("GET", "/api/operations/f1", nil, &v)
	if v.Editable || v.Origin != "fundarr" || v.Value != "2000.00" || v.Account.Name != "Kraken" {
		t.Fatalf("%+v", v)
	}
	var st struct {
		Activities int
		Sync       syncView
	}
	c.do("GET", "/api/status", nil, &st)
	if st.Activities != 1 || !st.Sync.Linked || st.Sync.URL != "http://f" {
		t.Fatalf("%+v", st)
	}
}

func TestImport(t *testing.T) {
	c := newClient(t)
	var p struct {
		Valid, Errors, Duplicates int
		FileAccounts              bool
		Accounts                  []string
	}
	in := map[string]any{"csv": csvfile.Template}
	if c.do("POST", "/api/import/preview", in, &p) != 200 || p.Valid != 7 || !p.FileAccounts || p.Accounts[0] != "Binance" {
		t.Fatalf("preview: %+v", p)
	}
	var r struct{ Added, Skipped, Errors int }
	if c.do("POST", "/api/import", map[string]any{"csv": csvfile.Template, "newAccount": "Kraken"}, &r) != 200 || r.Added != 7 {
		t.Fatalf("import: %+v", r)
	}
	c.do("POST", "/api/import", map[string]any{"csv": csvfile.Template, "fromFile": true}, &r)
	if r.Added != 0 || r.Skipped != 7 {
		t.Fatalf("again: %+v", r)
	}
	c.do("POST", "/api/import/preview", in, &p)
	if p.Duplicates != 7 || p.Valid != 0 {
		t.Fatalf("preview again: %+v", p)
	}
	var e struct{ Error string }
	if c.do("POST", "/api/import/preview", map[string]any{"csv": "a,b\n1,2"}, &e) != 422 {
		t.Fatal("unknown file")
	}
	req := httptest.NewRequest("GET", "/api/import/modele.csv", nil)
	w := httptest.NewRecorder()
	c.h.ServeHTTP(w, req)
	if w.Code != 200 || !strings.HasPrefix(w.Body.String(), "date,type") {
		t.Fatal("template")
	}
}

func TestPrice(t *testing.T) {
	c := newClient(t)
	var p struct{ Price string }
	if c.do("GET", "/api/price?asset=eth&date=2024-05-01", nil, &p) != 200 || p.Price != "2000" {
		t.Fatalf("%+v", p)
	}
	var e struct{ Error string }
	if c.do("GET", "/api/price?asset=NOPE&date=2024-05-01", nil, &e) != 404 {
		t.Fatal("unknown coin")
	}
}

// The statistics answer, and the edit of an operation shows in them at once.
func TestStatsRoutes(t *testing.T) {
	c := newClient(t)
	buy := c.add(map[string]string{"kind": "buy", "date": "2024-01-10", "newAccount": "Binance", "asset": "BTC", "qty": "0.1", "total": "4000"})
	var o struct {
		Total struct{ Value, Invested float64 } `json:"total"`
	}
	if c.do("GET", "/api/overview", nil, &o) != 200 || o.Total.Value != 6000 || o.Total.Invested != 4000 {
		t.Fatalf("%+v", o)
	}
	var s struct {
		Assets []struct{ Gain float64 } `json:"assets"`
	}
	if c.do("GET", "/api/stats", nil, &s) != 200 || len(s.Assets) != 1 || s.Assets[0].Gain != 2000 {
		t.Fatalf("%+v", s)
	}
	c.do("PUT", "/api/operations/"+buy.ID, map[string]string{"kind": "buy", "date": "2024-01-10", "account": buy.Account.ID, "asset": "BTC", "qty": "0.1", "total": "5000"}, nil)
	if c.do("GET", "/api/stats", nil, &s) != 200 || s.Assets[0].Gain != 1000 {
		t.Fatalf("after the edit: %+v", s)
	}
	var a struct {
		Markers []struct{ Price float64 } `json:"markers"`
	}
	if c.do("GET", "/api/assets/btc", nil, &a) != 200 || len(a.Markers) != 1 || a.Markers[0].Price != 50000 {
		t.Fatalf("%+v", a)
	}
	var e struct{ Error string }
	if c.do("GET", "/api/assets/DOGE", nil, &e) != 404 || e.Error == "" {
		t.Fatal("unknown coin")
	}
}
