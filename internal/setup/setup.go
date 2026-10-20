// Package setup holds which Fundarr Cairn reads, and lets the UI set it.
//
// The connection comes from FUNDARR_URL and FUNDARR_TOKEN when they are set;
// otherwise from cairn.json in the data folder, written by the first-run page.
// Before either, the reads answer 503 and the UI asks for an address and a
// token. The token is only ever written, never sent back to the browser.
package setup

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"mime"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Alexis-Marcel/cairn/internal/proxy"
)

// File is the name of the saved connection, in the data folder.
const File = "cairn.json"

// Conn is a Fundarr to read, and the token to read it with.
type Conn struct {
	URL   string `json:"url"`
	Token string `json:"token"`
}

// Setup is mounted on /api/setup, and its API on the rest of /api/.
type Setup struct {
	dir     string
	fromEnv bool
	ttl     time.Duration
	client  *http.Client // for the check before saving

	mu    sync.Mutex             // one change at a time
	now   atomic.Pointer[linked] // nil: not linked
	guard *http.CrossOriginProtection
}

// linked is the Fundarr read now.
type linked struct {
	url   string
	proxy *proxy.Proxy
}

// New reads the connection: env (url or token set) first, then the saved
// file in dir. A broken file is left as is and Cairn starts unlinked.
func New(dir string, env Conn, ttl time.Duration) (*Setup, error) {
	s := &Setup{
		dir:    dir,
		ttl:    ttl,
		client: &http.Client{Timeout: 10 * time.Second},
		guard:  http.NewCrossOriginProtection(),
	}
	if env.URL != "" || env.Token != "" {
		if env.URL == "" {
			env.URL = "http://localhost:9494"
		}
		s.fromEnv = true
		if err := s.use(env); err != nil {
			return nil, fmt.Errorf("FUNDARR_URL: %w", err)
		}
		return s, nil
	}
	if err := writable(dir); err != nil {
		slog.Warn("data folder not writable: the link cannot be saved", "dir", dir, "err", err)
	}
	c, err := Load(dir)
	switch {
	case errors.Is(err, os.ErrNotExist):
	case err != nil:
		slog.Warn("saved connection unreadable, starting unlinked", "file", filepath.Join(dir, File), "err", err)
	default:
		if err := s.use(c); err != nil {
			slog.Warn("saved connection unusable, starting unlinked", "err", err)
		}
	}
	return s, nil
}

// Linked is the Fundarr read now ("" when none), and whether env set it.
func (s *Setup) Linked() (url string, fromEnv bool) {
	if l := s.now.Load(); l != nil {
		url = l.url
	}
	return url, s.fromEnv
}

func (s *Setup) use(c Conn) error {
	p, err := proxy.New(c.URL, c.Token, s.ttl)
	if err != nil {
		return err
	}
	s.now.Store(&linked{url: strings.TrimRight(c.URL, "/"), proxy: p})
	return nil
}

// API forwards to the linked Fundarr, or says there is none.
func (s *Setup) API() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		l := s.now.Load()
		if l == nil {
			writeErr(w, http.StatusServiceUnavailable, "Cairn n'est pas relié à Fundarr")
			return
		}
		l.proxy.ServeHTTP(w, r)
	})
}

// state is what GET /api/setup answers.
type state struct {
	Configured bool   `json:"configured"`
	FromEnv    bool   `json:"fromEnv"`
	URL        string `json:"url"`
}

func (s *Setup) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet, http.MethodHead:
		u, env := s.Linked()
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		json.NewEncoder(w).Encode(state{Configured: u != "", FromEnv: env, URL: u})
	case http.MethodPost, http.MethodDelete:
		if err := s.guard.Check(r); err != nil {
			writeErr(w, http.StatusForbidden, "Requête d'un autre site refusée")
			return
		}
		// A form cannot send JSON: a page elsewhere would need a preflight,
		// which gets no CORS answer.
		if mt, _, _ := mime.ParseMediaType(r.Header.Get("Content-Type")); mt != "application/json" {
			writeErr(w, http.StatusUnsupportedMediaType, "JSON attendu")
			return
		}
		if s.fromEnv {
			writeErr(w, http.StatusConflict, "Réglé par FUNDARR_URL et FUNDARR_TOKEN")
			return
		}
		s.mu.Lock()
		defer s.mu.Unlock()
		if r.Method == http.MethodPost {
			s.link(w, r)
		} else {
			s.delete(w)
		}
	default:
		w.Header().Set("Allow", "GET, HEAD, POST, DELETE")
		writeErr(w, http.StatusMethodNotAllowed, "Méthode refusée")
	}
}

func (s *Setup) link(w http.ResponseWriter, r *http.Request) {
	var c Conn
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8<<10)).Decode(&c); err != nil {
		writeErr(w, http.StatusBadRequest, "Requête illisible")
		return
	}
	c.URL = Normalize(c.URL)
	c.Token = strings.TrimSpace(c.Token)
	if _, err := proxy.New(c.URL, c.Token, 0); err != nil || c.URL == "" {
		writeErr(w, http.StatusBadRequest, "Adresse invalide")
		return
	}
	if c.Token == "" {
		writeErr(w, http.StatusBadRequest, "Jeton manquant")
		return
	}
	if err := Check(r.Context(), s.client, c); err != nil {
		var ce *CheckError
		if errors.As(err, &ce) {
			writeErr(w, ce.Status, ce.Msg)
			return
		}
		writeErr(w, http.StatusBadGateway, "Adresse injoignable")
		return
	}
	if err := Save(s.dir, c); err != nil {
		slog.Error("save connection", "dir", s.dir, "err", err)
		writeErr(w, http.StatusInternalServerError, "Impossible d'enregistrer (dossier CAIRN_DATA)")
		return
	}
	s.use(c)
	slog.Info("linked", "fundarr", c.URL)
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	json.NewEncoder(w).Encode(state{Configured: true, URL: c.URL})
}

func (s *Setup) delete(w http.ResponseWriter) {
	if err := Remove(s.dir); err != nil {
		slog.Error("remove connection", "dir", s.dir, "err", err)
		writeErr(w, http.StatusInternalServerError, "Impossible d'effacer (dossier CAIRN_DATA)")
		return
	}
	s.now.Store(nil)
	slog.Info("unlinked")
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	json.NewEncoder(w).Encode(state{})
}

// Normalize trims an address as typed: spaces, the final slash, and adds
// http:// when no scheme is given ("fundarr:9494").
func Normalize(u string) string {
	u = strings.TrimRight(strings.TrimSpace(u), "/")
	if u != "" && !strings.Contains(u, "://") {
		u = "http://" + u
	}
	return u
}

// CheckError is why a connection was refused, said for the UI.
type CheckError struct {
	Status int
	Msg    string
}

func (e *CheckError) Error() string { return e.Msg }

// Check asks Fundarr for its sources with the token: the cheapest read that
// proves both the address and the token.
func Check(ctx context.Context, client *http.Client, c Conn) error {
	u, err := url.Parse(c.URL)
	if err != nil {
		return &CheckError{http.StatusBadRequest, "Adresse invalide"}
	}
	u.Path = strings.TrimRight(u.Path, "/") + "/api/v1/sources"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return &CheckError{http.StatusBadRequest, "Adresse invalide"}
	}
	req.Header.Set("Authorization", "Bearer "+c.Token)
	req.Header.Set("Accept", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		var ne net.Error
		if errors.As(err, &ne) && ne.Timeout() {
			return &CheckError{http.StatusGatewayTimeout, "Adresse injoignable : pas de réponse"}
		}
		return &CheckError{http.StatusBadGateway, "Adresse injoignable"}
	}
	defer resp.Body.Close()
	switch {
	case resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden:
		return &CheckError{http.StatusUnprocessableEntity, "Jeton refusé par Fundarr"}
	case resp.StatusCode == http.StatusNotFound:
		return &CheckError{http.StatusBadGateway, "Pas de Fundarr à cette adresse"}
	case resp.StatusCode != http.StatusOK:
		return &CheckError{http.StatusBadGateway, fmt.Sprintf("Fundarr a répondu %d", resp.StatusCode)}
	}
	// Some other app may answer 200 to anything: Fundarr answers JSON.
	var v any
	if json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(&v) != nil {
		return &CheckError{http.StatusBadGateway, "Pas de Fundarr à cette adresse"}
	}
	return nil
}

// Load reads the saved connection (os.ErrNotExist when there is none).
func Load(dir string) (Conn, error) {
	var c Conn
	b, err := os.ReadFile(filepath.Join(dir, File))
	if err != nil {
		return c, err
	}
	if err := json.Unmarshal(b, &c); err != nil {
		return c, err
	}
	if c.URL == "" {
		return c, errors.New("no url")
	}
	return c, nil
}

// Save writes the connection, readable by Cairn alone, in one step: a crash
// leaves the old file or the new one, never half of it.
func Save(dir string, c Conn) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	b, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	f, err := os.CreateTemp(dir, ".cairn-*.json")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name()) // gone once renamed
	if err := f.Chmod(0o600); err != nil {
		f.Close()
		return err
	}
	if _, err := f.Write(append(b, '\n')); err != nil {
		f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), filepath.Join(dir, File))
}

// writable tells early what Save would find out late.
func writable(dir string) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	f, err := os.CreateTemp(dir, ".cairn-*")
	if err != nil {
		return err
	}
	f.Close()
	return os.Remove(f.Name())
}

// Remove forgets the saved connection; none is fine.
func Remove(dir string) error {
	err := os.Remove(filepath.Join(dir, File))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

func writeErr(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}
