// Cairn: where my crypto stands, and the French 2086 form.
//
// One binary: it serves the UI and its API, keeps the activities in SQLite
// (CAIRN_DATA/cairn.db) and works the portfolio out itself. They are entered
// by hand, imported from a CSV, or copied from a Fundarr (optional). There
// is no sign-in: put it behind a reverse proxy that does it.
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

	"github.com/Alexis-Marcel/cairn/internal/books"
	"github.com/Alexis-Marcel/cairn/internal/fundarr"
	"github.com/Alexis-Marcel/cairn/internal/server"
	"github.com/Alexis-Marcel/cairn/internal/store"
	"github.com/Alexis-Marcel/cairn/web"
	"github.com/Alexis-Marcel/fundarr/pkg/price"
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
	st, err := store.Open(data)
	if err != nil {
		return fmt.Errorf("database in %s: %w", data, err)
	}
	defer st.Close()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if moved, err := st.MigrateLegacy(ctx, data); err != nil {
		log.Warn("cairn.json not migrated", "err", err)
	} else if moved {
		log.Info("link to Fundarr moved from cairn.json into the database")
	}
	fromEnv := false
	if u, t := env("FUNDARR_URL", ""), env("FUNDARR_TOKEN", ""); u != "" || t != "" {
		if u == "" {
			u = "http://localhost:9494"
		}
		u = fundarr.Normalize(u)
		if !fundarr.ValidURL(u) {
			return fmt.Errorf("FUNDARR_URL: %q is not an http(s) address", u)
		}
		if err := st.SetLink(ctx, u, t); err != nil {
			return err
		}
		fromEnv = true
	}
	if s := env("FUNDARR_NOTIFY_SECRET", ""); s != "" {
		if err := st.SetSecret(ctx, s); err != nil {
			return err
		}
	}

	prices := price.New(st, &http.Client{Timeout: 30 * time.Second})
	engine := books.New(st, prices)
	engine.TTL = ttl
	syncer := fundarr.New(st, log)
	syncer.After = func() { engine.Warm(ctx) }

	dist, err := fs.Sub(web.Dist, "dist")
	if err != nil {
		return err
	}
	srv := &server.Server{St: st, Books: engine, Sync: syncer, FromEnv: fromEnv, Log: log}
	hs := &http.Server{Addr: addr, Handler: srv.Handler(dist), ReadHeaderTimeout: 10 * time.Second}
	go func() {
		<-ctx.Done()
		shut, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		hs.Shutdown(shut)
	}()
	go syncer.Run(ctx)
	go engine.Warm(ctx)

	l, _ := st.Link(ctx)
	linked := l.URL
	if linked == "" {
		linked = "(none)"
	}
	log.Info("cairn", "version", version, "addr", addr, "data", data, "fundarr", linked, "fromEnv", fromEnv)
	if err := hs.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
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
