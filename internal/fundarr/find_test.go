package fundarr

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestFind(t *testing.T) {
	fundarr := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/openapi.yaml" {
			w.Write([]byte("openapi: 3.1.0\ninfo:\n  title: Fundarr read API\n"))
			return
		}
		http.NotFound(w, r)
	}))
	defer fundarr.Close()
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("hello"))
	}))
	defer other.Close()

	if got := Find(context.Background(), []string{"http://127.0.0.1:1", other.URL, fundarr.URL}); got != fundarr.URL {
		t.Fatalf("got %q, want %q", got, fundarr.URL)
	}
	if got := Find(context.Background(), []string{other.URL}); got != "" {
		t.Fatalf("got %q, want nothing", got)
	}
}
