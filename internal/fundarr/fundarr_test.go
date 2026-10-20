package fundarr

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Alexis-Marcel/cairn/internal/store"
	"github.com/Alexis-Marcel/cairn/internal/testutil"
	"github.com/Alexis-Marcel/fundarr/pkg/api"
	"github.com/Alexis-Marcel/fundarr/pkg/model"
)

var ctx = context.Background()

// feed is a fake Fundarr: a list of changes, an epoch, and the token fdr_ok.
type feed struct {
	mu      sync.Mutex
	epoch   string
	changes []api.Change
	calls   []string
}

func act(id, src string, qty string) *api.Activity {
	return &api.Activity{Activity: model.Activity{ID: id, SourceID: src, Time: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
		Type: model.Reward, Base: model.Amt(model.CryptoOrFiat("BTC"), qty)}, Source: api.Source{ID: src, Name: strings.ToUpper(src)}}
}

func (f *feed) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if r.Header.Get("Authorization") != "Bearer fdr_ok" {
		w.WriteHeader(401)
		return
	}
	f.calls = append(f.calls, r.URL.RequestURI())
	switch r.URL.Path {
	case "/api/v1/sources":
		w.Write([]byte(`[]`))
		return
	case "/api/v1/changes":
	default:
		w.WriteHeader(404)
		return
	}
	start := 0
	if after := r.URL.Query().Get("after"); after != "" {
		epoch, n, _ := strings.Cut(after, ".")
		if epoch != f.epoch {
			w.WriteHeader(http.StatusGone)
			return
		}
		for _, c := range n {
			start = start*10 + int(c-'0')
		}
	}
	// Two changes a page.
	end := min(start+2, len(f.changes))
	page := api.Changes{Changes: f.changes[start:end], Cursor: f.epoch + "." + itoa(end), More: end < len(f.changes)}
	json.NewEncoder(w).Encode(page)
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	s := ""
	for ; n > 0; n /= 10 {
		s = string(rune('0'+n%10)) + s
	}
	return s
}

func setup(t *testing.T) (*feed, *Syncer, *store.Store, string) {
	f := &feed{epoch: "a"}
	srv := httptest.NewServer(f)
	t.Cleanup(srv.Close)
	st := testutil.Store(t)
	return f, New(st, nil), st, srv.URL
}

func TestSync(t *testing.T) {
	f, s, st, url := setup(t)
	f.changes = []api.Change{{Op: api.Upsert, ID: "1", Activity: act("1", "bin", "1")}, {Op: api.Upsert, ID: "2", Activity: act("2", "bin", "2")},
		{Op: api.Upsert, ID: "3", Activity: act("3", "okx", "3")}}
	if err := s.Check(ctx, url, "fdr_bad"); err == nil {
		t.Fatal("a refused token fails the check")
	}
	if err := s.Check(ctx, url, "fdr_ok"); err != nil {
		t.Fatal(err)
	}
	st.SetLink(ctx, url, "fdr_ok")
	if err := s.Sync(ctx); err != nil {
		t.Fatal(err)
	}
	if n, _ := st.Count(ctx); n != 3 {
		t.Fatalf("copied %d", n)
	}
	l, _ := st.Link(ctx)
	if l.Cursor != "a.3" || l.LastSync.IsZero() || l.LastError != "" {
		t.Fatalf("%+v", l)
	}

	// Only what changed since comes back: an upsert and a removal.
	f.changes = append(f.changes, api.Change{Op: api.Upsert, ID: "2", Activity: act("2", "bin", "5")}, api.Change{Op: api.Remove, ID: "3"})
	if err := s.Sync(ctx); err != nil {
		t.Fatal(err)
	}
	two, _ := st.Get(ctx, "2")
	if two.Base.Qty.String() != "5" {
		t.Fatalf("upsert: %v", two.Base.Qty)
	}
	if _, err := st.Get(ctx, "3"); err != store.ErrNotFound {
		t.Fatal("removed")
	}
	accs, _ := st.Accounts(ctx)
	if len(accs) != 1 {
		t.Fatalf("the emptied source goes: %+v", accs)
	}
	if last := f.calls[len(f.calls)-1]; !strings.Contains(last, "after=a.") {
		t.Fatalf("asked from the cursor: %s", last)
	}

	// Fundarr starts over (410): the copy is dropped and made again.
	f.epoch = "b"
	f.changes = []api.Change{{Op: api.Upsert, ID: "9", Activity: act("9", "bin", "9")}}
	if err := s.Sync(ctx); err != nil {
		t.Fatal(err)
	}
	all, _ := st.All(ctx)
	if len(all) != 1 || all[0].ID != "9" {
		t.Fatalf("after 410: %+v", all)
	}
	if l, _ := st.Link(ctx); l.Cursor != "b.1" {
		t.Fatalf("cursor %q", l.Cursor)
	}
}

func TestSyncKeepsManual(t *testing.T) {
	f, s, st, url := setup(t)
	f.changes = []api.Change{{Op: api.Upsert, ID: "1", Activity: act("1", "bin", "1")}}
	acc, _ := st.CreateAccount(ctx, "Mine")
	st.Put(ctx, store.Activity{Origin: store.Manual, Activity: model.Activity{ID: "m", SourceID: acc.ID, Time: time.Now(), Type: model.Reward, Base: model.Amt(model.CryptoOrFiat("ETH"), "1")}})
	st.SetLink(ctx, url, "fdr_ok")
	s.Sync(ctx)
	f.epoch = "z"
	s.Sync(ctx)
	if _, err := st.Get(ctx, "m"); err != nil {
		t.Fatal("a resync never touches what was entered by hand")
	}
}

func TestSyncError(t *testing.T) {
	_, s, st, url := setup(t)
	st.SetLink(ctx, url, "fdr_wrong")
	if err := s.Sync(ctx); err == nil {
		t.Fatal("refused token")
	}
	l, _ := st.Link(ctx)
	if l.LastError != "Jeton refusé par Fundarr" {
		t.Fatalf("error kept for the UI: %q", l.LastError)
	}
}

func TestVerify(t *testing.T) {
	body := []byte(`{"event":"changes","cursor":"a.1"}`)
	now := time.Unix(1729425600, 0)
	h := Sign("fns_secret", now, body)
	if err := Verify(h, body, "fns_secret", now.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	for name, c := range map[string]struct {
		h, secret string
		body      []byte
		at        time.Time
	}{
		"other secret": {h, "fns_other", body, now},
		"other body":   {h, "fns_secret", []byte(`{}`), now},
		"too old":      {h, "fns_secret", body, now.Add(10 * time.Minute)},
		"no signature": {"", "fns_secret", body, now},
		"bad hex":      {"t=1729425600,v1=zz", "fns_secret", body, now},
	} {
		if Verify(c.h, c.body, c.secret, c.at) == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}

func TestNotified(t *testing.T) {
	_, s, st, url := setup(t)
	post := func(sig string) int {
		body := `{"event":"changes","cursor":"a.1"}`
		r := httptest.NewRequest("POST", "/api/fundarr/notify", strings.NewReader(body))
		if sig != "" {
			r.Header.Set(Signature, Sign(sig, time.Now(), []byte(body)))
		}
		w := httptest.NewRecorder()
		s.Notified(w, r)
		return w.Code
	}
	if c := post("fns_x"); c != 404 {
		t.Fatalf("off without a link: %d", c)
	}
	st.SetLink(ctx, url, "fdr_ok")
	if c := post("fns_x"); c != 404 {
		t.Fatalf("off without a secret: %d", c)
	}
	st.SetSecret(ctx, "fns_x")
	if c := post("fns_y"); c != 401 {
		t.Fatalf("wrong secret: %d", c)
	}
	if c := post("fns_x"); c != 204 {
		t.Fatalf("signed: %d", c)
	}
	select {
	case <-s.kick:
	default:
		t.Fatal("a notification asks for a sync")
	}
}

func TestNormalize(t *testing.T) {
	for in, want := range map[string]string{
		"fundarr:9494":                  "http://fundarr:9494",
		" https://f.example/api/v1/ ":   "https://f.example",
		"http://127.0.0.1:9494/":        "http://127.0.0.1:9494",
		"https://f.example/base/api/v1": "https://f.example/base",
	} {
		if got := Normalize(in); got != want {
			t.Errorf("%q: %q, want %q", in, got, want)
		}
	}
}
