package main

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNoCORSHeaders(t *testing.T) {
	a := &app{logger: slog.New(slog.NewTextHandler(io.Discard, nil))}
	for _, method := range []string{http.MethodGet, http.MethodOptions} {
		w := httptest.NewRecorder()
		r := httptest.NewRequest(method, "/healthz", nil)
		r.Header.Set("Origin", "https://evil.example")
		routes(a).ServeHTTP(w, r)
		for _, header := range []string{"Access-Control-Allow-Origin", "Access-Control-Allow-Methods", "Access-Control-Allow-Headers", "Access-Control-Allow-Credentials"} {
			if got := w.Header().Get(header); got != "" {
				t.Fatalf("%s %s: %s = %q, want no CORS headers", method, r.URL.Path, header, got)
			}
		}
	}
}

func TestCrossOriginWritesAreRejected(t *testing.T) {
	a := &app{logger: slog.New(slog.NewTextHandler(io.Discard, nil))}
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/set-token", nil)
	r.Header.Set("Sec-Fetch-Site", "cross-site")
	routes(a).ServeHTTP(w, r)
	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", w.Code)
	}
}
