// Package server is Cairn's HTTP API, under /api, and the UI around it.
//
// Reads are GET. Every other method must be JSON sent by Cairn's own page
// (cross-origin protection, and a content type a form cannot send), except
// POST /api/fundarr/notify, which Fundarr calls and signs.
package server

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"log/slog"
	"mime"
	"net/http"
	"strings"
	"time"

	"github.com/Alexis-Marcel/cairn/internal/books"
	"github.com/Alexis-Marcel/cairn/internal/fundarr"
	"github.com/Alexis-Marcel/cairn/internal/store"
)

// Server holds what the handlers need.
type Server struct {
	St    *store.Store
	Books *books.Engine
	Sync  *fundarr.Syncer
	// FromEnv: FUNDARR_URL and FUNDARR_TOKEN fix the link; the UI cannot
	// change it.
	FromEnv bool
	Log     *slog.Logger
	// Now is the clock (tests set it).
	Now func() time.Time
}

func (s *Server) now() time.Time {
	if s.Now != nil {
		return s.Now().UTC()
	}
	return time.Now().UTC()
}

// Handler serves the API, and dist (the built UI) for any other path.
func (s *Server) Handler(dist fs.FS) http.Handler {
	if s.Log == nil {
		s.Log = slog.Default()
	}
	mux := http.NewServeMux()
	api := http.NewServeMux()

	api.HandleFunc("GET /api/status", s.status)
	api.HandleFunc("GET /api/portfolio", s.portfolio)
	api.HandleFunc("GET /api/portfolio/history", s.history)
	api.HandleFunc("GET /api/tax/fr", s.taxFR)
	api.HandleFunc("GET /api/activities", s.activities)

	api.HandleFunc("GET /api/operations", s.listOps)
	api.HandleFunc("POST /api/operations", s.createOp)
	api.HandleFunc("GET /api/operations/{id}", s.getOp)
	api.HandleFunc("PUT /api/operations/{id}", s.updateOp)
	api.HandleFunc("DELETE /api/operations/{id}", s.deleteOp)

	api.HandleFunc("GET /api/accounts", s.listAccounts)
	api.HandleFunc("POST /api/accounts", s.createAccount)
	api.HandleFunc("PATCH /api/accounts/{id}", s.renameAccount)
	api.HandleFunc("DELETE /api/accounts/{id}", s.deleteAccount)

	api.HandleFunc("GET /api/assets", s.assets)
	api.HandleFunc("GET /api/price", s.price)

	api.HandleFunc("POST /api/import/preview", s.importPreview)
	api.HandleFunc("POST /api/import", s.importFile)
	api.HandleFunc("GET /api/import/modele.csv", s.template)

	api.HandleFunc("GET /api/sync", s.syncState)
	api.HandleFunc("POST /api/sync", s.link)
	api.HandleFunc("DELETE /api/sync", s.unlink)
	api.HandleFunc("POST /api/sync/now", s.syncNow)
	api.HandleFunc("GET /api/sync/find", s.findFundarr)
	api.HandleFunc("PUT /api/sync/secret", s.setSecret)

	api.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) {
		writeErr(w, http.StatusNotFound, "Introuvable")
	})

	// Fundarr's notifications come from Fundarr, not from the page: the
	// signature is their proof.
	mux.HandleFunc("POST /api/fundarr/notify", s.Sync.Notified)
	mux.Handle("/api/", guard(api))
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		if err := s.St.Ping(r.Context()); err != nil {
			http.Error(w, "database", http.StatusServiceUnavailable)
			return
		}
		w.Write([]byte("ok"))
	})
	if dist != nil {
		mux.Handle("/", spa(dist))
	}
	return secure(mux)
}

// guard refuses writes that do not come from Cairn's own page.
func guard(h http.Handler) http.Handler {
	cop := http.NewCrossOriginProtection()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		if r.Method == http.MethodGet || r.Method == http.MethodHead {
			h.ServeHTTP(w, r)
			return
		}
		if err := cop.Check(r); err != nil {
			writeErr(w, http.StatusForbidden, "Requête d'un autre site refusée")
			return
		}
		// A form cannot send JSON: a page elsewhere would need a preflight,
		// which gets no CORS answer.
		if mt, _, _ := mime.ParseMediaType(r.Header.Get("Content-Type")); mt != "application/json" {
			writeErr(w, http.StatusUnsupportedMediaType, "JSON attendu")
			return
		}
		h.ServeHTTP(w, r)
	})
}

// spa serves the built files, and index.html for any other path (the UI
// routes on its own). Hashed assets are kept for good, the page never.
func spa(dist fs.FS) http.Handler {
	files := http.FileServerFS(dist)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		name := strings.TrimPrefix(r.URL.Path, "/")
		if name != "" {
			if st, err := fs.Stat(dist, name); err == nil && !st.IsDir() {
				if strings.HasPrefix(name, "assets/") {
					w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
				}
				files.ServeHTTP(w, r)
				return
			}
		}
		w.Header().Set("Cache-Control", "no-cache")
		http.ServeFileFS(w, r, dist, "index.html")
	})
}

// secure adds the headers every answer should carry.
func secure(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; img-src 'self' data:; style-src 'self' 'unsafe-inline'; frame-ancestors 'none'")
		h.ServeHTTP(w, r)
	})
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(v)
}

func writeRaw(w http.ResponseWriter, body []byte) {
	w.Header().Set("Content-Type", "application/json")
	w.Write(body)
}

func writeErr(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, code, map[string]string{"error": msg})
}

// fail answers an error: a refusal with its status, anything else 500.
func (s *Server) fail(w http.ResponseWriter, r *http.Request, err error) {
	var be *books.Error
	switch {
	case errors.As(err, &be):
		writeErr(w, be.Status, be.Msg)
	case errors.Is(err, store.ErrNotFound):
		writeErr(w, http.StatusNotFound, "Introuvable")
	case errors.Is(err, context.Canceled):
		// The page went away.
	default:
		s.Log.Error("api", "path", r.URL.Path, "err", err)
		writeErr(w, http.StatusInternalServerError, "Erreur de Cairn")
	}
}

// decode reads a JSON body of at most max bytes.
func decode(w http.ResponseWriter, r *http.Request, max int64, v any) bool {
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, max)).Decode(v); err != nil {
		var mbe *http.MaxBytesError
		if errors.As(err, &mbe) {
			writeErr(w, http.StatusRequestEntityTooLarge, "Trop lourd")
		} else {
			writeErr(w, http.StatusBadRequest, "Requête illisible")
		}
		return false
	}
	return true
}

// ---- reads computed from the activities

func (s *Server) portfolio(w http.ResponseWriter, r *http.Request) {
	b, err := s.Books.Portfolio(r.Context())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeRaw(w, b)
}

func (s *Server) history(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	b, err := s.Books.History(r.Context(), q.Get("from"), q.Get("to"), q.Get("step"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeRaw(w, b)
}

func (s *Server) taxFR(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	b, err := s.Books.TaxFR(r.Context(), q.Get("year"), q.Get("rewards"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeRaw(w, b)
}

// syncView is the link to Fundarr, as the UI sees it (never the token).
type syncView struct {
	Linked    bool       `json:"linked"`
	FromEnv   bool       `json:"fromEnv"`
	URL       string     `json:"url,omitempty"`
	LastSync  *time.Time `json:"lastSync,omitempty"`
	LastError string     `json:"lastError,omitempty"`
	Notify    bool       `json:"notify"`
	Running   bool       `json:"running"`
}

func (s *Server) syncView(ctx context.Context) (syncView, error) {
	l, err := s.St.Link(ctx)
	if err != nil {
		return syncView{}, err
	}
	v := syncView{Linked: l.URL != "", FromEnv: s.FromEnv, URL: l.URL, LastError: l.LastError,
		Notify: l.Secret != "", Running: s.Sync.Running()}
	if !l.LastSync.IsZero() {
		t := l.LastSync
		v.LastSync = &t
	}
	return v, nil
}

func (s *Server) status(w http.ResponseWriter, r *http.Request) {
	n, err := s.St.Count(r.Context())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	sv, err := s.syncView(r.Context())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, 200, map[string]any{"activities": n, "sync": sv})
}
