// Package fundarr keeps a copy of a Fundarr's history in Cairn, through
// its changes feed (GET /api/v1/changes): every 15 minutes, when Fundarr
// notifies a change, and on demand. Fundarr is optional: Cairn works the
// same without it, from what is entered or imported by hand.
package fundarr

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Alexis-Marcel/cairn/internal/store"
	"github.com/Alexis-Marcel/fundarr/pkg/api"
)

// Every is how often the copy is brought up to date without being told.
const Every = 15 * time.Minute

// Syncer copies a Fundarr into the store.
type Syncer struct {
	st     *store.Store
	client *http.Client
	log    *slog.Logger
	// After is called after a sync that changed something (to warm caches).
	After func()

	kick    chan struct{}
	mu      sync.Mutex // one sync at a time
	running atomic.Bool
	now     func() time.Time
}

// New returns a syncer over st.
func New(st *store.Store, log *slog.Logger) *Syncer {
	if log == nil {
		log = slog.Default()
	}
	return &Syncer{st: st, client: &http.Client{Timeout: 60 * time.Second}, log: log,
		kick: make(chan struct{}, 1), now: func() time.Time { return time.Now().UTC() }}
}

// Running reports whether a sync is under way.
func (s *Syncer) Running() bool { return s.running.Load() }

// Kick asks for a sync soon. Never blocks.
func (s *Syncer) Kick() {
	select {
	case s.kick <- struct{}{}:
	default:
	}
}

// Run syncs at start, every Every and when kicked, until ctx is done.
func (s *Syncer) Run(ctx context.Context) {
	t := time.NewTicker(Every)
	defer t.Stop()
	s.Kick()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		case <-s.kick:
		}
		if err := s.Sync(ctx); err != nil && ctx.Err() == nil {
			s.log.Warn("fundarr sync", "err", err)
		}
	}
}

// Sync brings the copy up to date now, if Cairn is linked to a Fundarr.
func (s *Syncer) Sync(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	l, err := s.st.Link(ctx)
	if err != nil || l.URL == "" {
		return err
	}
	s.running.Store(true)
	defer s.running.Store(false)
	before := s.st.Version()
	err = s.pull(ctx, l)
	if ctx.Err() != nil {
		return err
	}
	if serr := s.st.SyncDone(ctx, s.now(), err); serr != nil && err == nil {
		err = serr
	}
	if err == nil && s.st.Version() != before && s.After != nil {
		go s.After()
	}
	return err
}

// errGone: Fundarr no longer knows the cursor.
var errGone = errors.New("gone")

func (s *Syncer) pull(ctx context.Context, l store.Link) error {
	cursor := l.Cursor
	restarted := false
	for pages := 0; pages < 10000; pages++ {
		page, err := s.page(ctx, l, cursor)
		if errors.Is(err, errGone) && !restarted {
			// The cursor is too old, or from another Fundarr: start over.
			s.log.Info("fundarr: cursor refused, copying again from the start")
			if err := s.st.DropFundarr(ctx); err != nil {
				return err
			}
			cursor, restarted = "", true
			continue
		}
		if err != nil {
			return err
		}
		if err := s.st.ApplyChanges(ctx, page); err != nil {
			return err
		}
		cursor = page.Cursor
		if !page.More {
			return nil
		}
	}
	return errors.New("Fundarr : trop de pages")
}

// page asks for the changes after cursor.
func (s *Syncer) page(ctx context.Context, l store.Link, cursor string) (api.Changes, error) {
	var page api.Changes
	u, err := url.Parse(l.URL)
	if err != nil {
		return page, err
	}
	u.Path = strings.TrimRight(u.Path, "/") + "/api/v1/changes"
	q := url.Values{"limit": {"1000"}}
	if cursor != "" {
		q.Set("after", cursor)
	}
	u.RawQuery = q.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return page, err
	}
	req.Header.Set("Authorization", "Bearer "+l.Token)
	req.Header.Set("Accept", "application/json")
	resp, err := s.client.Do(req)
	if err != nil {
		return page, unreachable(err)
	}
	defer resp.Body.Close()
	switch {
	case resp.StatusCode == http.StatusGone, resp.StatusCode == http.StatusBadRequest && cursor != "":
		io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<16))
		return page, errGone
	case resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden:
		return page, errors.New("Jeton refusé par Fundarr")
	case resp.StatusCode == http.StatusNotFound:
		return page, errors.New("Ce Fundarr n'a pas de flux de changements : mets-le à jour")
	case resp.StatusCode != http.StatusOK:
		return page, fmt.Errorf("Fundarr a répondu %d", resp.StatusCode)
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 256<<20)).Decode(&page); err != nil {
		return page, fmt.Errorf("Réponse de Fundarr illisible : %w", err)
	}
	return page, nil
}

func unreachable(err error) error {
	var ne net.Error
	if errors.As(err, &ne) && ne.Timeout() {
		return errors.New("Fundarr ne répond pas")
	}
	return errors.New("Fundarr est injoignable")
}

// ---- notifications

// Signature is the header Fundarr signs its notifications with.
const Signature = "Fundarr-Signature"

// Tolerance: how old a notification may be.
const Tolerance = 5 * time.Minute

// Verify checks a Fundarr-Signature header ("t=<unix>,v1=<hex>") against
// body and secret, and that it is recent.
func Verify(header string, body []byte, secret string, now time.Time) error {
	var ts, sig string
	for _, part := range strings.Split(header, ",") {
		k, v, _ := strings.Cut(strings.TrimSpace(part), "=")
		switch k {
		case "t":
			ts = v
		case "v1":
			if sig == "" {
				sig = v
			}
		}
	}
	t, err := strconv.ParseInt(ts, 10, 64)
	if err != nil || sig == "" {
		return errors.New("signature missing")
	}
	if d := now.Sub(time.Unix(t, 0)); d > Tolerance || d < -Tolerance {
		return errors.New("signature too old")
	}
	got, err := hex.DecodeString(sig)
	if err != nil {
		return errors.New("signature unreadable")
	}
	m := hmac.New(sha256.New, []byte(secret))
	m.Write([]byte(ts + "."))
	m.Write(body)
	if !hmac.Equal(got, m.Sum(nil)) {
		return errors.New("signature wrong")
	}
	return nil
}

// Sign makes the header Fundarr would send (tests, docs).
func Sign(secret string, t time.Time, body []byte) string {
	ts := strconv.FormatInt(t.Unix(), 10)
	m := hmac.New(sha256.New, []byte(secret))
	m.Write([]byte(ts + "."))
	m.Write(body)
	return "t=" + ts + ",v1=" + hex.EncodeToString(m.Sum(nil))
}

// Notified handles POST /api/fundarr/notify: a signed call from Fundarr
// saying the history changed. Without a secret it is turned off (404).
func (s *Syncer) Notified(w http.ResponseWriter, r *http.Request) {
	l, err := s.st.Link(r.Context())
	if err != nil {
		http.Error(w, "store", http.StatusInternalServerError)
		return
	}
	if l.URL == "" || l.Secret == "" {
		writeErr(w, http.StatusNotFound, "notifications off")
		return
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 64<<10))
	if err != nil {
		writeErr(w, http.StatusRequestEntityTooLarge, "body too large")
		return
	}
	if err := Verify(r.Header.Get(Signature), body, l.Secret, time.Now()); err != nil {
		s.log.Warn("fundarr notify refused", "err", err, "remote", r.RemoteAddr)
		writeErr(w, http.StatusUnauthorized, err.Error())
		return
	}
	var n struct {
		Event string `json:"event"`
	}
	if json.Unmarshal(body, &n) != nil {
		writeErr(w, http.StatusBadRequest, "JSON expected")
		return
	}
	if n.Event == "changes" {
		s.Kick()
	}
	w.WriteHeader(http.StatusNoContent)
}

func writeErr(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

// ---- linking

// Normalize trims an address as typed: spaces, the final slash, the API
// path Fundarr's Apps page shows (/api/v1), and adds http:// when no scheme
// is given ("fundarr:9494").
func Normalize(u string) string {
	u = strings.TrimRight(strings.TrimSpace(u), "/")
	u = strings.TrimRight(strings.TrimSuffix(u, "/api/v1"), "/")
	if u != "" && !strings.Contains(u, "://") {
		u = "http://" + u
	}
	return u
}

// ValidURL reports whether u is an http(s) address.
func ValidURL(u string) bool {
	p, err := url.Parse(u)
	return err == nil && (p.Scheme == "http" || p.Scheme == "https") && p.Host != ""
}

// CheckError is why a link was refused, said for the UI.
type CheckError struct {
	Status int
	Msg    string
}

func (e *CheckError) Error() string { return e.Msg }

// Check asks Fundarr for its sources with the token, the cheapest read that
// proves both the address and the token, then for an empty page of changes:
// Cairn needs the feed.
func (s *Syncer) Check(ctx context.Context, base, token string) error {
	for _, path := range []string{"/api/v1/sources", "/api/v1/changes?limit=1"} {
		if err := s.check(ctx, base, token, path); err != nil {
			return err
		}
	}
	return nil
}

func (s *Syncer) check(ctx context.Context, base, token, path string) error {
	u, err := url.Parse(base)
	if err != nil {
		return &CheckError{http.StatusBadRequest, "Adresse invalide"}
	}
	p, q, _ := strings.Cut(path, "?")
	u.Path = strings.TrimRight(u.Path, "/") + p
	u.RawQuery = q
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return &CheckError{http.StatusBadRequest, "Adresse invalide"}
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")
	c := &http.Client{Timeout: 10 * time.Second}
	resp, err := c.Do(req)
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
	case resp.StatusCode == http.StatusNotFound && p == "/api/v1/changes":
		return &CheckError{http.StatusBadGateway, "Ce Fundarr est trop ancien : mets-le à jour"}
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
