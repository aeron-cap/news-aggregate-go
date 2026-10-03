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
	"time"

	"github.com/aeron-cap/news-aggregator/internal/database"
)

func TestChangeInterestsRejectsInvalidPayload(t *testing.T) {
	for _, body := range []string{
		`{}`,
		`null`,
		`[]`,
		`{"id":1,"is_active":true}`,
		`{"abc":{"isActive":true,"isMain":false}}`,
		`{"0":{"isActive":true,"isMain":false}}`,
		`{"-1":{"isActive":true,"isMain":false}}`,
		`{"1":{"isActive":true}}`,
		`{"1":{"isMain":true}}`,
		`{"1":null}`,
		`{"1":{"isActive":null,"isMain":false}}`,
		`{"1":{"isActive":"true","isMain":false}}`,
		`{"1":{"isActive":true,"isMain":false,"weight":1}}`,
		`{"1":{"isActive":true,"isMain":false}} {}`,
		`{"1":{"isActive":true,"isMain":false}} garbage`,
		`{"1":{"isActive":true,"isMain":false},"2":{"isActive":false}}`,
	} {
		t.Run(body, func(t *testing.T) {
			a := &app{logger: slog.New(slog.NewTextHandler(io.Discard, nil))}
			w := httptest.NewRecorder()
			r := httptest.NewRequest(http.MethodPost, "/change-interests", strings.NewReader(body))
			a.updateInterests(w, r)
			if w.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400; body: %s", w.Code, w.Body.String())
			}
		})
	}
}

func TestChangeInterestsMissingIDRollsBack(t *testing.T) {
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(`CREATE TABLE interests (id INTEGER PRIMARY KEY, keyword TEXT, weight REAL, is_main BOOLEAN, anchor TEXT, is_active BOOLEAN);
		INSERT INTO interests VALUES (1, 'science', 1, 0, NULL, 0)`); err != nil {
		t.Fatal(err)
	}
	a := &app{
		store:  database.NewStore(db),
		cache:  &feedCache{},
		logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/change-interests", strings.NewReader(`{"1":{"isActive":true,"isMain":true},"999":{"isActive":false,"isMain":false}}`))
	a.updateInterests(w, r)
	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404; body: %s", w.Code, w.Body.String())
	}
	interests, err := a.store.GetInterests(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if interests[0].IsActive || interests[0].IsMain {
		t.Fatal("interest flags changed despite missing ID in batch")
	}
}

func sourceTestApp(t *testing.T) (*app, *sql.DB) {
	t.Helper()
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(`CREATE TABLE sources (id INTEGER PRIMARY KEY, name TEXT NOT NULL, url TEXT NOT NULL UNIQUE, is_active BOOLEAN NOT NULL DEFAULT 1)`); err != nil {
		t.Fatal(err)
	}
	return &app{store: database.NewStore(db), cache: &feedCache{}, logger: slog.New(slog.NewTextHandler(io.Discard, nil))}, db
}

func sourceRequest(a *app, method, path, body string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	routes(a).ServeHTTP(w, httptest.NewRequest(method, path, strings.NewReader(body)))
	return w
}

func TestFetchSourcesIncludesInactive(t *testing.T) {
	a, db := sourceTestApp(t)
	w := sourceRequest(a, http.MethodGet, "/sources", "")
	if w.Code != http.StatusOK || strings.TrimSpace(w.Body.String()) != "[]" {
		t.Fatalf("empty sources: status=%d body=%s", w.Code, w.Body.String())
	}
	if _, err := db.Exec(`INSERT INTO sources VALUES (1, 'Science', 'https://science.example/rss', 1), (2, 'Sports', 'https://sports.example/rss', 0)`); err != nil {
		t.Fatal(err)
	}
	w = sourceRequest(a, http.MethodGet, "/sources", "")
	var sources []sourcesResponse
	if err := json.Unmarshal(w.Body.Bytes(), &sources); err != nil {
		t.Fatal(err)
	}
	if w.Code != http.StatusOK || len(sources) != 2 || sources[0].ID != 1 || !sources[0].IsActive || sources[1].ID != 2 || sources[1].IsActive {
		t.Fatalf("status=%d sources=%+v", w.Code, sources)
	}
}

func TestInsertSourcesInsertsExactlyRequestedRows(t *testing.T) {
	a, db := sourceTestApp(t)
	w := sourceRequest(a, http.MethodPost, "/sources", `[{"name":" Science ","url":" https://science.example/rss ","is_active":true},{"name":"Sports","url":"http://sports.example/rss","is_active":false}]`)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM sources`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Fatalf("inserted %d sources, want 2", count)
	}
	var name, url string
	var active bool
	if err := db.QueryRow(`SELECT name, url, is_active FROM sources WHERE id = 1`).Scan(&name, &url, &active); err != nil {
		t.Fatal(err)
	}
	if name != "Science" || url != "https://science.example/rss" || !active {
		t.Fatalf("unexpected inserted source: %q %q %v", name, url, active)
	}
}

func TestSourcesRejectInvalidPayload(t *testing.T) {
	invalidSources := []string{
		`null`,
		`{}`,
		`{"name":"","url":"https://example.com/rss","is_active":true}`,
		`{"name":"  ","url":"https://example.com/rss","is_active":true}`,
		`{"name":"Example","url":"  ","is_active":true}`,
		`{"name":"Example","url":"/rss","is_active":true}`,
		`{"name":"Example","url":"ftp://example.com/rss","is_active":true}`,
		`{"name":"Example","url":"https://","is_active":true}`,
		`{"name":"Example","url":"https://example.com/rss"}`,
		`{"name":"Example","url":"https://example.com/rss","is_active":null}`,
		`{"name":"Example","url":"https://example.com/rss","is_active":"true"}`,
		`{"name":"Example","url":"https://example.com/rss","is_active":true,"extra":1}`,
	}
	for _, path := range []string{"/sources", "/change-sources"} {
		bodies := []string{"", `null`, `[]`, `{}`, `garbage`}
		valid := `{"name":"Example","url":"https://example.com/rss","is_active":true}`
		wrap := func(source string) string {
			if path == "/sources" {
				return "[" + source + "]"
			}
			return `{"1":` + source + "}"
		}
		for _, source := range invalidSources {
			bodies = append(bodies, wrap(source))
		}
		if path == "/sources" {
			bodies = append(bodies, "["+valid+`,{"name":"Invalid"}]`)
		} else {
			bodies = append(bodies, `{"1":`+valid+`,"2":{"name":"Invalid"}}`)
		}
		bodies = append(bodies, wrap(valid)+" {}", wrap(valid)+" garbage", wrap(valid)+strings.Repeat(" ", 1048576))
		if path == "/change-sources" {
			for _, id := range []string{"0", "-1", "abc"} {
				bodies = append(bodies, `{"`+id+`":`+valid+`}`)
			}
		}
		for _, body := range bodies {
			t.Run(path+"/"+body[:min(len(body), 100)], func(t *testing.T) {
				a, db := sourceTestApp(t)
				w := sourceRequest(a, http.MethodPost, path, body)
				if w.Code != http.StatusBadRequest {
					t.Fatalf("status=%d, want 400; body=%s", w.Code, w.Body.String())
				}
				var count int
				if err := db.QueryRow(`SELECT COUNT(*) FROM sources`).Scan(&count); err != nil || count != 0 {
					t.Fatalf("invalid request changed sources: count=%d err=%v", count, err)
				}
			})
		}
	}
}

func TestUpdateSourcesAndMissingID(t *testing.T) {
	for _, missing := range []bool{false, true} {
		t.Run(map[bool]string{false: "success", true: "missing ID rolls back"}[missing], func(t *testing.T) {
			a, db := sourceTestApp(t)
			if _, err := db.Exec(`INSERT INTO sources VALUES (1, 'Original', 'https://original.example/rss', 1)`); err != nil {
				t.Fatal(err)
			}
			body := `{"1":{"name":" Renamed ","url":" https://renamed.example/rss ","is_active":false}`
			a.cache.set([]byte(`[]`), time.Hour)
			wantStatus := http.StatusOK
			if missing {
				body += `,"999":{"name":"Missing","url":"https://missing.example/rss","is_active":true}`
				wantStatus = http.StatusNotFound
			}
			w := sourceRequest(a, http.MethodPost, "/change-sources", body+"}")
			if w.Code != wantStatus {
				t.Fatalf("status=%d, want %d; body=%s", w.Code, wantStatus, w.Body.String())
			}
			if _, hit := a.cache.get(); hit != missing {
				t.Fatalf("cache hit=%v, want %v", hit, missing)
			}
			var name, url string
			var active bool
			if err := db.QueryRow(`SELECT name, url, is_active FROM sources WHERE id = 1`).Scan(&name, &url, &active); err != nil {
				t.Fatal(err)
			}
			if missing {
				if name != "Original" || url != "https://original.example/rss" || !active {
					t.Fatalf("missing ID changed source: %q %q %v", name, url, active)
				}
			} else if name != "Renamed" || url != "https://renamed.example/rss" || active {
				t.Fatalf("source not updated: %q %q %v", name, url, active)
			}
		})
	}
}

func TestSourcesDatabaseErrors(t *testing.T) {
	for _, tc := range []struct{ method, path, body string }{
		{http.MethodGet, "/sources", ""},
		{http.MethodPost, "/sources", `[{"name":"Example","url":"https://example.com/rss","is_active":true}]`},
		{http.MethodPost, "/change-sources", `{"1":{"name":"Example","url":"https://example.com/rss","is_active":true}}`},
	} {
		t.Run(tc.path+tc.method, func(t *testing.T) {
			a, db := sourceTestApp(t)
			db.Close()
			w := sourceRequest(a, tc.method, tc.path, tc.body)
			if w.Code != http.StatusInternalServerError {
				t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
			}
		})
	}
}
