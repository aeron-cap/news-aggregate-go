package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/aeron-cap/news-aggregator/internal/database"
	"github.com/aeron-cap/news-aggregator/internal/testutil"
)

func closedStoreApp(t *testing.T) *app {
	t.Helper()
	db, err := sql.Open("pgx", "postgresql://news_aggregator@db.internal.example:5432/news_aggregator")
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	return &app{store: database.NewStore(db), logger: slog.New(slog.NewTextHandler(io.Discard, nil))}
}

func TestServerErrorsHideInternalDetails(t *testing.T) {
	a := closedStoreApp(t)
	id := testutil.ID(1).String()
	for _, tc := range []struct {
		name, method, body string
		handler            http.HandlerFunc
	}{
		{"set token", http.MethodPost, "", a.setToken},
		{"build feed", http.MethodGet, "", a.buildFeed},
		{"fetch feed", http.MethodGet, "", a.fetchFeed},
		{"mark as read", http.MethodPost, "", a.markAsRead},
		{"fetch interests", http.MethodGet, "", a.fetchInterests},
		{"update interests", http.MethodPost, `{"` + id + `":{"isActive":true,"isMain":false}}`, a.updateInterests},
		{"fetch sources", http.MethodGet, "", a.fetchSources},
		{"insert sources", http.MethodPost, `[{"name":"Example","url":"https://example.com/rss","is_active":true}]`, a.insertSources},
		{"update sources", http.MethodPost, `{"` + id + `":{"name":"Example","url":"https://example.com/rss","is_active":true}}`, a.updateSources},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			r := httptest.NewRequest(tc.method, "/", strings.NewReader(tc.body))
			r.SetPathValue("id", id)
			r = r.WithContext(context.WithValue(r.Context(), userIDKey{}, testutil.ID(1)))
			tc.handler(w, r)
			if w.Code != http.StatusInternalServerError {
				t.Fatalf("status = %d, want 500; body: %s", w.Code, w.Body.String())
			}
			var body map[string]string
			if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
				t.Fatalf("decode body: %v; body: %s", err, w.Body.String())
			}
			if body["details"] != tryAgainLater {
				t.Fatalf("details = %q, want %q", body["details"], tryAgainLater)
			}
			for _, leak := range []string{"sql:", "closed", "db.internal.example"} {
				if strings.Contains(w.Body.String(), leak) {
					t.Fatalf("response leaks %q: %s", leak, w.Body.String())
				}
			}
		})
	}
}

func TestDuplicateSourceURLIsBadRequest(t *testing.T) {
	a, _ := sourceTestApp(t)
	w := sourceRequest(a, http.MethodPost, "/sources", `[{"name":"One","url":"https://example.com/rss","is_active":true},{"name":"Two","url":"https://example.com/rss","is_active":true}]`)
	if w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), database.ErrDuplicateSourceURL.Error()) {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
}
