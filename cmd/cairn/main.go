// Cairn: where my crypto stands, read from Fundarr.
//
// One binary: it serves the UI and forwards the UI's reads to Fundarr's read
// API with the token, which stays on the server. The token comes from the
// environment or from the first-run page, saved in CAIRN_DATA. There is no
// sign-in: put it behind a reverse proxy that does it.
package main

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/Alexis-Marcel/cairn/internal/setup"
	"github.com/Alexis-Marcel/cairn/web"
)

var version = "dev"

func env(k, def string) string {
	if v := strings.TrimSpace(os.Getenv(k)); v != "" {
		return v
	}
	return def
}

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "health":
			os.Exit(health())
		case "version":
			fmt.Println(version)
			return
		}
	}
	log := slog.New(slog.NewTextHandler(os.Stderr, nil))
	slog.SetDefault(log)
	if err := run(log); err != nil {
		log.Error("fatal", "err", err)
		os.Exit(1)
	}
}

func run(log *slog.Logger) error {
	addr := env("CAIRN_ADDR", ":8080")
	ttl, err := time.ParseDuration(env("CAIRN_CACHE", "60s"))
	if err != nil {
		return fmt.Errorf("CAIRN_CACHE: %w", err)
	}
	data := env("CAIRN_DATA", "/data")
	s, err := setup.New(data, setup.Conn{URL: env("FUNDARR_URL", ""), Token: env("FUNDARR_TOKEN", "")}, ttl)
	if err != nil {
		return err
	}
	dist, err := fs.Sub(web.Dist, "dist")
	if err != nil {
		return err
	}

	mux := http.NewServeMux()
	mux.Handle("/api/setup", s)
	mux.Handle("/api/", s.API())
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte("ok"))
	})
	mux.Handle("/", spa(dist))

	srv := &http.Server{Addr: addr, Handler: secure(mux), ReadHeaderTimeout: 10 * time.Second}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go func() {
		<-ctx.Done()
		shut, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		srv.Shutdown(shut)
	}()
	fundarr, fromEnv := s.Linked()
	if fundarr == "" {
		fundarr = "(not linked yet: open Cairn)"
	}
	log.Info("cairn", "version", version, "addr", addr, "data", data, "fundarr", fundarr, "fromEnv", fromEnv)
	if err := srv.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
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

// health is the container healthcheck (the image has no shell or curl).
func health() int {
	addr := env("CAIRN_ADDR", ":8080")
	if strings.HasPrefix(addr, ":") {
		addr = "127.0.0.1" + addr
	}
	c := &http.Client{Timeout: 3 * time.Second}
	resp, err := c.Get("http://" + addr + "/healthz")
	if err != nil || resp.StatusCode != 200 {
		return 1
	}
	return 0
}
