package fundarr

import (
	"context"
	"io"
	"net/http"
	"strings"
	"time"
)

// Nearby are the addresses a Fundarr installed next to Cairn usually has:
// its container name on a shared Docker network, then the same machine.
var Nearby = []string{"http://fundarr:9494", "http://localhost:9494", "http://host.docker.internal:9494"}

// Find returns the first of the addresses that answers as Fundarr, or "".
// It asks for the API description, which needs no token. The addresses are
// fixed, never taken from a request.
func Find(ctx context.Context, addrs []string) string {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	found := make([]chan bool, len(addrs))
	for i, a := range addrs {
		found[i] = make(chan bool, 1)
		go func() { found[i] <- isFundarr(ctx, a) }()
	}
	for i, a := range addrs {
		if <-found[i] {
			return a
		}
	}
	return ""
}

func isFundarr(ctx context.Context, base string) bool {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/api/v1/openapi.yaml", nil)
	if err != nil {
		return false
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	head, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
	return resp.StatusCode == http.StatusOK && strings.Contains(string(head), "Fundarr")
}
