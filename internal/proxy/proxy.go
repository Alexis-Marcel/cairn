// Package proxy forwards the browser's reads to Fundarr's read API.
//
// The browser asks /api/<path>; the proxy asks Fundarr <base>/api/v1/<path>
// with the token, so the token never leaves the server. Answers that went well
// are kept a short while: the same page asks the same things several times,
// and Fundarr recomputes the portfolio on each call.
package proxy

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"
)

// maxBody bounds what one answer may weigh (a long activity export).
const maxBody = 32 << 20

// validPath is what may follow /api/: letters, digits and a few separators,
// never "..", so a request cannot step out of /api/v1.
var validPath = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._/-]*$`)

// Proxy is an http.Handler mounted on /api/.
type Proxy struct {
	base   *url.URL // Fundarr, e.g. http://fundarr:9494
	token  string
	ttl    time.Duration
	client *http.Client
	now    func() time.Time

	mu    sync.Mutex
	cache map[string]entry
}

// entry is a kept answer.
type entry struct {
	status  int
	header  http.Header
	body    []byte
	expires time.Time
}

// New returns a proxy to Fundarr at base, sending token, keeping answers for
// ttl (0 keeps nothing).
func New(base, token string, ttl time.Duration) (*Proxy, error) {
	u, err := url.Parse(strings.TrimRight(base, "/"))
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return nil, fmt.Errorf("%q is not an http(s) address", base)
	}
	return &Proxy{
		base:   u,
		token:  token,
		ttl:    ttl,
		client: &http.Client{Timeout: 60 * time.Second},
		now:    time.Now,
		cache:  map[string]entry{},
	}, nil
}

func (p *Proxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		writeErr(w, http.StatusMethodNotAllowed, "Lecture seule")
		return
	}
	path := strings.TrimPrefix(r.URL.Path, "/api/")
	if path == r.URL.Path || !validPath.MatchString(path) || strings.Contains(path, "..") {
		writeErr(w, http.StatusNotFound, "Introuvable")
		return
	}
	key := path + "?" + r.URL.RawQuery

	if e, ok := p.get(key); ok {
		w.Header().Set("X-Cache", "hit")
		e.write(w, r)
		return
	}

	e, err := p.fetch(r, path)
	if err != nil {
		slog.Warn("fundarr", "path", path, "err", err)
		var ue *upstreamError
		if errors.As(err, &ue) {
			writeErr(w, ue.status, ue.msg)
			return
		}
		var ne net.Error
		if errors.As(err, &ne) && ne.Timeout() {
			writeErr(w, http.StatusGatewayTimeout, "Fundarr ne répond pas")
			return
		}
		writeErr(w, http.StatusBadGateway, "Fundarr est injoignable")
		return
	}
	if e.status == http.StatusOK {
		p.put(key, e)
	}
	w.Header().Set("X-Cache", "miss")
	e.write(w, r)
}

// upstreamError is Fundarr's answer turned into one the UI can show.
type upstreamError struct {
	status int
	msg    string
}

func (e *upstreamError) Error() string { return fmt.Sprintf("%d %s", e.status, e.msg) }

// fetch asks Fundarr. Only the query and Accept are forwarded: the browser's
// cookies and headers (the SSO's among them) stay here.
func (p *Proxy) fetch(r *http.Request, path string) (entry, error) {
	u := *p.base
	u.Path = p.base.Path + "/api/v1/" + path
	u.RawQuery = r.URL.RawQuery
	req, err := http.NewRequestWithContext(r.Context(), http.MethodGet, u.String(), nil)
	if err != nil {
		return entry{}, err
	}
	if p.token != "" {
		req.Header.Set("Authorization", "Bearer "+p.token)
	}
	if a := r.Header.Get("Accept"); a != "" {
		req.Header.Set("Accept", a)
	}
	resp, err := p.client.Do(req)
	if err != nil {
		return entry{}, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBody+1))
	if err != nil {
		return entry{}, err
	}
	if len(body) > maxBody {
		return entry{}, &upstreamError{http.StatusBadGateway, "Réponse de Fundarr trop lourde"}
	}

	switch {
	case resp.StatusCode == http.StatusOK:
	case resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden:
		return entry{}, &upstreamError{http.StatusBadGateway, "Fundarr refuse le jeton"}
	case resp.StatusCode == http.StatusNotFound:
		return entry{}, &upstreamError{http.StatusNotFound, "Introuvable dans Fundarr"}
	case resp.StatusCode >= 400 && resp.StatusCode < 500:
		// A bad parameter: Fundarr's own message says which.
		return entry{}, &upstreamError{resp.StatusCode, upstreamMsg(body, resp.StatusCode)}
	default:
		return entry{}, &upstreamError{http.StatusBadGateway, upstreamMsg(body, resp.StatusCode)}
	}

	h := http.Header{}
	for _, k := range []string{"Content-Type", "Content-Disposition"} {
		if v := resp.Header.Get(k); v != "" {
			h.Set(k, v)
		}
	}
	return entry{status: resp.StatusCode, header: h, body: body}, nil
}

// upstreamMsg is the "error" of Fundarr's JSON error, or its status.
func upstreamMsg(body []byte, status int) string {
	var e struct {
		Error string `json:"error"`
	}
	if json.Unmarshal(body, &e) == nil && e.Error != "" {
		return "Fundarr : " + e.Error
	}
	return fmt.Sprintf("Fundarr a répondu %d", status)
}

func (p *Proxy) get(key string) (entry, bool) {
	if p.ttl <= 0 {
		return entry{}, false
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	e, ok := p.cache[key]
	if !ok || !p.now().Before(e.expires) {
		return entry{}, false
	}
	return e, true
}

func (p *Proxy) put(key string, e entry) {
	if p.ttl <= 0 {
		return
	}
	now := p.now()
	e.expires = now.Add(p.ttl)
	p.mu.Lock()
	defer p.mu.Unlock()
	// Drop what expired, so the map stays the size of what is in use.
	for k, old := range p.cache {
		if !now.Before(old.expires) {
			delete(p.cache, k)
		}
	}
	p.cache[key] = e
}

func (e entry) write(w http.ResponseWriter, r *http.Request) {
	for k, v := range e.header {
		w.Header()[k] = v
	}
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(e.status)
	if r.Method != http.MethodHead {
		w.Write(e.body)
	}
}

func writeErr(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}
