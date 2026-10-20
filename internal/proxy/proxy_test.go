package proxy

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// upstream is a fake Fundarr that records what it was asked.
type upstream struct {
	srv   *httptest.Server
	calls atomic.Int32
	last  atomic.Pointer[http.Request]
}

func newUpstream(t *testing.T, h http.HandlerFunc) *upstream {
	u := &upstream{}
	u.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u.calls.Add(1)
		u.last.Store(r.Clone(r.Context()))
		h(w, r)
	}))
	t.Cleanup(u.srv.Close)
	return u
}

func ok(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	io.WriteString(w, `{"currency":"EUR"}`)
}

func do(t *testing.T, h http.Handler, method, target string, hdr ...string) (*httptest.ResponseRecorder, map[string]string) {
	t.Helper()
	r := httptest.NewRequest(method, target, nil)
	for i := 0; i+1 < len(hdr); i += 2 {
		r.Header.Set(hdr[i], hdr[i+1])
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	var e map[string]string
	if w.Code != http.StatusOK {
		if err := json.Unmarshal(w.Body.Bytes(), &e); err != nil {
			t.Fatalf("%s %s: %d, body is not a JSON error: %q", method, target, w.Code, w.Body)
		}
	}
	return w, e
}

func TestForwardsWithToken(t *testing.T) {
	up := newUpstream(t, ok)
	p, err := New(up.srv.URL+"/", "fdr_secret", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	w, _ := do(t, p, "GET", "/api/portfolio/history?step=week&from=2025-01-01",
		"Cookie", "sso=1", "Authorization", "Bearer browser")
	if w.Code != 200 || w.Body.String() != `{"currency":"EUR"}` {
		t.Fatalf("got %d %q", w.Code, w.Body)
	}
	if ct := w.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("content type %q", ct)
	}
	r := up.last.Load()
	if r.URL.Path != "/api/v1/portfolio/history" || r.URL.RawQuery != "step=week&from=2025-01-01" {
		t.Errorf("upstream asked %s?%s", r.URL.Path, r.URL.RawQuery)
	}
	if got := r.Header.Get("Authorization"); got != "Bearer fdr_secret" {
		t.Errorf("authorization %q", got)
	}
	if r.Header.Get("Cookie") != "" {
		t.Error("browser cookies forwarded to Fundarr")
	}
}

func TestRefusesWrites(t *testing.T) {
	up := newUpstream(t, ok)
	p, _ := New(up.srv.URL, "t", time.Minute)
	for _, m := range []string{"POST", "PUT", "PATCH", "DELETE"} {
		w, e := do(t, p, m, "/api/sources")
		if w.Code != http.StatusMethodNotAllowed || e["error"] == "" {
			t.Errorf("%s: %d %v", m, w.Code, e)
		}
	}
	if n := up.calls.Load(); n != 0 {
		t.Errorf("%d writes reached Fundarr", n)
	}
}

func TestRefusesOtherPaths(t *testing.T) {
	up := newUpstream(t, ok)
	p, _ := New(up.srv.URL, "t", time.Minute)
	for _, path := range []string{"/api/", "/api/../auth", "/api/a/../../x", "/elsewhere"} {
		if w, _ := do(t, p, "GET", path); w.Code != http.StatusNotFound {
			t.Errorf("%s: %d", path, w.Code)
		}
	}
	if n := up.calls.Load(); n != 0 {
		t.Errorf("%d requests reached Fundarr", n)
	}
}

func TestCache(t *testing.T) {
	up := newUpstream(t, ok)
	p, _ := New(up.srv.URL, "t", time.Minute)
	now := time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)
	p.now = func() time.Time { return now }

	if w, _ := do(t, p, "GET", "/api/portfolio"); w.Header().Get("X-Cache") != "miss" {
		t.Error("first call should miss")
	}
	if w, _ := do(t, p, "GET", "/api/portfolio"); w.Header().Get("X-Cache") != "hit" || w.Body.String() != `{"currency":"EUR"}` {
		t.Errorf("second call: %s %q", w.Header().Get("X-Cache"), w.Body)
	}
	do(t, p, "GET", "/api/portfolio?x=1") // another URL, another entry
	if n := up.calls.Load(); n != 2 {
		t.Errorf("upstream called %d times, want 2", n)
	}

	now = now.Add(61 * time.Second)
	do(t, p, "GET", "/api/portfolio")
	if n := up.calls.Load(); n != 3 {
		t.Errorf("expired entry served: %d calls", n)
	}
}

func TestErrorsNotCached(t *testing.T) {
	var fail atomic.Bool
	fail.Store(true)
	up := newUpstream(t, func(w http.ResponseWriter, r *http.Request) {
		if fail.Load() {
			http.Error(w, "boom", 500)
			return
		}
		ok(w, r)
	})
	p, _ := New(up.srv.URL, "t", time.Minute)
	if w, _ := do(t, p, "GET", "/api/portfolio"); w.Code != http.StatusBadGateway {
		t.Fatalf("got %d", w.Code)
	}
	fail.Store(false)
	if w, _ := do(t, p, "GET", "/api/portfolio"); w.Code != 200 {
		t.Fatalf("error was cached: %d", w.Code)
	}
}

func TestUpstreamErrors(t *testing.T) {
	cases := []struct {
		name   string
		status int
		body   string
		want   int
		msg    string
	}{
		{"token refused", 401, `{"error":"invalid token"}`, 502, "jeton"},
		{"forbidden", 403, ``, 502, "jeton"},
		{"not found", 404, `404 page not found`, 404, "Introuvable"},
		{"bad parameter", 400, `{"error":"year must be a number"}`, 400, "Fundarr : year must be a number"},
		{"crash", 500, `{"error":"database is locked"}`, 502, "Fundarr : database is locked"},
		{"proxy page", 503, `<html>down</html>`, 502, "Fundarr a répondu 503"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			up := newUpstream(t, func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(c.status)
				io.WriteString(w, c.body)
			})
			p, _ := New(up.srv.URL, "t", time.Minute)
			w, e := do(t, p, "GET", "/api/tax/fr?year=x")
			if w.Code != c.want || !strings.Contains(e["error"], c.msg) {
				t.Errorf("got %d %q, want %d containing %q", w.Code, e["error"], c.want, c.msg)
			}
		})
	}
}

func TestUnreachable(t *testing.T) {
	up := newUpstream(t, ok)
	addr := up.srv.URL
	up.srv.Close()
	p, _ := New(addr, "t", time.Minute)
	w, e := do(t, p, "GET", "/api/portfolio")
	if w.Code != http.StatusBadGateway || !strings.Contains(e["error"], "injoignable") {
		t.Errorf("got %d %v", w.Code, e)
	}
}

func TestTimeout(t *testing.T) {
	up := newUpstream(t, func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-time.After(2 * time.Second):
		}
	})
	p, _ := New(up.srv.URL, "t", time.Minute)
	p.client.Timeout = 50 * time.Millisecond
	if w, _ := do(t, p, "GET", "/api/portfolio"); w.Code != http.StatusGatewayTimeout {
		t.Errorf("got %d", w.Code)
	}
}

func TestBadBase(t *testing.T) {
	for _, b := range []string{"", "fundarr:9494", "ftp://x"} {
		if _, err := New(b, "t", 0); err == nil {
			t.Errorf("%q accepted", b)
		}
	}
}
