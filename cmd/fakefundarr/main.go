// fakefundarr answers like Fundarr's read API with a made-up portfolio, to
// develop Cairn without a Fundarr. Not for anything else.
package main

import (
	"flag"
	"log/slog"
	"net/http"
	"time"

	"github.com/Alexis-Marcel/cairn/internal/fake"
)

func main() {
	addr := flag.String("addr", ":9494", "address to listen on")
	token := flag.String("token", "fdr_dev", "token to require (empty: any)")
	delay := flag.Duration("delay", 0, "added to every answer, e.g. 400ms")
	flag.Parse()

	b := fake.Generate(time.Now().UTC())
	slog.Info("fake fundarr", "addr", *addr, "token", *token, "activities", len(b.Activities), "days", len(b.History))
	if err := http.ListenAndServe(*addr, fake.Handler(b, *token, *delay)); err != nil {
		slog.Error("fatal", "err", err)
	}
}
