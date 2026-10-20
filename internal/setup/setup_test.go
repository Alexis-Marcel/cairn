package setup

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// fundarr is a fake Fundarr that knows one token.
func fundarr(t *testing.T, token string) *httptest.Server {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+token {
			w.WriteHeader(http.StatusUnauthorized)
			io.WriteString(w, `{"error":"invalid token"}`)
			return
		}
		if r.URL.Path != "/api/v1/sources" && r.URL.Path != "/api/v1/portfolio" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"ok":true}`)
	}))
	t.Cleanup(srv.Close)
	return srv
}

// call asks h, as the UI does unless hdr says otherwise.
func call(t *testing.T, h http.Handler, method, target, body string, hdr ...string) (*httptest.ResponseRecorder, map[string]any) {
	t.Helper()
	r := httptest.NewRequest(method, target, strings.NewReader(body))
	r.Host = "cairn.test"
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Sec-Fetch-Site", "same-origin")
	r.Header.Set("Origin", "http://cairn.test")
	for i := 0; i+1 < len(hdr); i += 2 {
		if hdr[i+1] == "" {
			r.Header.Del(hdr[i])
		} else {
			r.Header.Set(hdr[i], hdr[i+1])
		}
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	var m map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &m); err != nil {
		t.Fatalf("%s %s: %d, body is not JSON: %q", method, target, w.Code, w.Body)
	}
	return w, m
}

func link(url, token string) string {
	b, _ := json.Marshal(Conn{URL: url, Token: token})
	return string(b)
}

func TestSaveLoad(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "data") // made on the first save
	if _, err := Load(dir); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("empty dir: %v", err)
	}
	want := Conn{URL: "http://fundarr:9494", Token: "fdr_secret"}
	if err := Save(dir, want); err != nil {
		t.Fatal(err)
	}
	st, err := os.Stat(filepath.Join(dir, File))
	if err != nil {
		t.Fatal(err)
	}
	if st.Mode().Perm() != 0o600 {
		t.Errorf("mode %v, want 0600", st.Mode().Perm())
	}
	if got, err := Load(dir); err != nil || got != want {
		t.Errorf("load: %+v %v", got, err)
	}
	// Saving again replaces, and leaves no temporary file behind.
	want.Token = "fdr_other"
	if err := Save(dir, want); err != nil {
		t.Fatal(err)
	}
	if got, _ := Load(dir); got != want {
		t.Errorf("after second save: %+v", got)
	}
	if ents, _ := os.ReadDir(dir); len(ents) != 1 {
		t.Errorf("%d files in data dir, want 1", len(ents))
	}
	if err := Remove(dir); err != nil {
		t.Fatal(err)
	}
	if err := Remove(dir); err != nil {
		t.Errorf("second remove: %v", err)
	}
	if _, err := Load(dir); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("after remove: %v", err)
	}
}

func TestNormalize(t *testing.T) {
	for in, want := range map[string]string{
		" http://fundarr:9494/ ":   "http://fundarr:9494",
		"fundarr:9494":             "http://fundarr:9494",
		"https://f.example.com/x/": "https://f.example.com/x",
		"":                         "",
	} {
		if got := Normalize(in); got != want {
			t.Errorf("%q: %q, want %q", in, got, want)
		}
	}
}

func TestFirstRun(t *testing.T) {
	f := fundarr(t, "fdr_good")
	dir := t.TempDir()
	s, err := New(dir, Conn{}, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	api := s.API()

	if _, m := call(t, s, "GET", "/api/setup", ""); m["configured"] != false || m["fromEnv"] != false {
		t.Errorf("state %v", m)
	}
	if w, _ := call(t, api, "GET", "/api/portfolio", ""); w.Code != http.StatusServiceUnavailable {
		t.Errorf("unlinked read: %d", w.Code)
	}

	// Refused, nothing saved.
	for _, c := range []struct {
		body string
		code int
		msg  string
	}{
		{link(f.URL, "fdr_bad"), 422, "Jeton refusé"},
		{link("http://127.0.0.1:1", "fdr_good"), 502, "injoignable"},
		{link(f.URL+"/elsewhere", "fdr_good"), 502, "Pas de Fundarr"},
		{link("ftp://x", "fdr_good"), 400, "Adresse invalide"},
		{link(f.URL, " "), 400, "Jeton manquant"},
		{`{`, 400, "illisible"},
	} {
		w, m := call(t, s, "POST", "/api/setup", c.body)
		if w.Code != c.code || !strings.Contains(m["error"].(string), c.msg) {
			t.Errorf("%s: %d %v, want %d %q", c.body, w.Code, m, c.code, c.msg)
		}
	}
	if _, err := Load(dir); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("a refused link was saved: %v", err)
	}

	w, m := call(t, s, "POST", "/api/setup", link(f.URL+"/", "fdr_good"))
	if w.Code != 200 || m["configured"] != true || m["url"] != f.URL {
		t.Fatalf("link: %d %v", w.Code, m)
	}
	if strings.Contains(w.Body.String(), "fdr_good") {
		t.Error("token sent back")
	}
	if c, _ := Load(dir); c != (Conn{URL: f.URL, Token: "fdr_good"}) {
		t.Errorf("saved %+v", c)
	}
	if w, _ := call(t, api, "GET", "/api/portfolio", ""); w.Code != 200 {
		t.Errorf("linked read: %d %q", w.Code, w.Body)
	}
	w, m = call(t, s, "GET", "/api/setup", "")
	if m["configured"] != true || m["url"] != f.URL {
		t.Errorf("state %v", m)
	}
	if strings.Contains(w.Body.String(), "fdr_good") {
		t.Error("token in state")
	}

	// A restart reads it back.
	s2, _ := New(dir, Conn{}, time.Minute)
	if u, env := s2.Linked(); u != f.URL || env {
		t.Errorf("after restart: %q %v", u, env)
	}

	if w, m := call(t, s, "DELETE", "/api/setup", ""); w.Code != 200 || m["configured"] != false {
		t.Errorf("unlink: %d %v", w.Code, m)
	}
	if _, err := Load(dir); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("file kept: %v", err)
	}
	if w, _ := call(t, api, "GET", "/api/portfolio", ""); w.Code != http.StatusServiceUnavailable {
		t.Errorf("read after unlink: %d", w.Code)
	}
}

func TestEnvWins(t *testing.T) {
	f := fundarr(t, "fdr_env")
	dir := t.TempDir()
	if err := Save(dir, Conn{URL: "http://saved:9494", Token: "fdr_saved"}); err != nil {
		t.Fatal(err)
	}
	s, err := New(dir, Conn{URL: f.URL, Token: "fdr_env"}, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if _, m := call(t, s, "GET", "/api/setup", ""); m["configured"] != true || m["fromEnv"] != true || m["url"] != f.URL {
		t.Errorf("state %v", m)
	}
	if w, _ := call(t, s.API(), "GET", "/api/portfolio", ""); w.Code != 200 {
		t.Errorf("read: %d", w.Code)
	}
	for _, m := range []string{"POST", "DELETE"} {
		if w, _ := call(t, s, m, "/api/setup", link(f.URL, "fdr_env")); w.Code != http.StatusConflict {
			t.Errorf("%s: %d", m, w.Code)
		}
	}
	if c, _ := Load(dir); c.Token != "fdr_saved" {
		t.Errorf("saved file touched: %+v", c)
	}
	// Only the token: the default address.
	s, _ = New(dir, Conn{Token: "x"}, 0)
	if u, env := s.Linked(); u != "http://localhost:9494" || !env {
		t.Errorf("token only: %q %v", u, env)
	}
	if _, err := New(dir, Conn{URL: "fundarr:9494", Token: "x"}, 0); err == nil {
		t.Error("bad FUNDARR_URL accepted")
	}
}

func TestBrokenFile(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, File), []byte("{nope"), 0o600)
	s, err := New(dir, Conn{}, 0)
	if err != nil {
		t.Fatal(err)
	}
	if u, _ := s.Linked(); u != "" {
		t.Errorf("linked to %q", u)
	}
}

func TestCrossSite(t *testing.T) {
	f := fundarr(t, "fdr_good")
	dir := t.TempDir()
	s, _ := New(dir, Conn{}, 0)
	body := link(f.URL, "fdr_good")
	for _, c := range []struct {
		name string
		hdr  []string
		code int
	}{
		{"other site", []string{"Sec-Fetch-Site", "cross-site", "Origin", "https://evil.example"}, 403},
		{"same site, other origin", []string{"Sec-Fetch-Site", "same-site", "Origin", "http://a.cairn.test"}, 403},
		{"old browser, other origin", []string{"Sec-Fetch-Site", "", "Origin", "https://evil.example"}, 403},
		{"form", []string{"Content-Type", "application/x-www-form-urlencoded"}, 415},
		{"text", []string{"Content-Type", "text/plain"}, 415},
		{"no type", []string{"Content-Type", ""}, 415},
	} {
		for _, m := range []string{"POST", "DELETE"} {
			if w, _ := call(t, s, m, "/api/setup", body, c.hdr...); w.Code != c.code {
				t.Errorf("%s %s: %d, want %d", c.name, m, w.Code, c.code)
			}
		}
	}
	if _, err := Load(dir); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("a cross-site request saved")
	}
	// No browser at all (curl): nothing to forge, allowed.
	if w, _ := call(t, s, "POST", "/api/setup", body, "Sec-Fetch-Site", "", "Origin", ""); w.Code != 200 {
		t.Errorf("curl: %d", w.Code)
	}
	if w, _ := call(t, s, "PUT", "/api/setup", body); w.Code != http.StatusMethodNotAllowed {
		t.Errorf("PUT: %d", w.Code)
	}
}
