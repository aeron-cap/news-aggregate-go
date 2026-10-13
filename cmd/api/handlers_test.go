package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/aeron-cap/news-aggregator/internal/database"
)

func schemaTestApp(t *testing.T) (*app, *sql.DB) {
	t.Helper()
	schema, err := os.ReadFile(filepath.Join("..", "..", "internal", "database", "schema.sql"))
	if err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(`PRAGMA foreign_keys = ON`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(string(schema)); err != nil {
		t.Fatalf("load schema.sql: %v", err)
	}
	return &app{
		store:  database.NewStore(db),
		logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	}, db
}

func userTestApp(t *testing.T) (*app, *sql.DB) {
	t.Helper()
	a, db := schemaTestApp(t)
	for _, token := range []string{"handler-user-1", "handler-user-2"} {
		if err := a.store.CreateAnonymousSession(context.Background(), hashSession(token), time.Now().UTC().Add(time.Hour)); err != nil {
			t.Fatal(err)
		}
	}
	return a, db
}

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
			r = r.WithContext(context.WithValue(r.Context(), userIDKey{}, int64(1)))
			a.updateInterests(w, r)
			if w.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400; body: %s", w.Code, w.Body.String())
			}
		})
	}
}

func TestChangeInterestsMissingIDRollsBack(t *testing.T) {
	a, db := userTestApp(t)
	if _, err := db.Exec(`
		INSERT INTO interests (id, keyword) VALUES (1, 'science');
		INSERT INTO user_interests (user_id, interest_id, weight, is_active, is_main)
		VALUES (1, 1, 1, 0, 0), (2, 1, 0.5, 1, 1)
	`); err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/change-interests", strings.NewReader(`{"1":{"isActive":true,"isMain":true},"999":{"isActive":false,"isMain":false}}`))
	r = r.WithContext(context.WithValue(r.Context(), userIDKey{}, int64(1)))
	a.updateInterests(w, r)
	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404; body: %s", w.Code, w.Body.String())
	}
	for userID := int64(1); userID <= 2; userID++ {
		var active, main bool
		if err := db.QueryRow(`SELECT is_active, is_main FROM user_interests WHERE user_id = ? AND interest_id = 1`, userID).Scan(&active, &main); err != nil {
			t.Fatal(err)
		}
		if active != (userID == 2) || main != (userID == 2) {
			t.Fatalf("user %d interest changed despite failed request: active=%v main=%v", userID, active, main)
		}
	}
}

func sourceTestApp(t *testing.T) (*app, *sql.DB) {
	t.Helper()
	return userTestApp(t)
}

func sourceRequest(a *app, method, path, body string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	r := httptest.NewRequest(method, path, strings.NewReader(body))
	r.AddCookie(&http.Cookie{Name: CookieName, Value: "handler-user-1"})
	r = r.WithContext(context.WithValue(r.Context(), userIDKey{}, int64(1)))
	routes(a).ServeHTTP(w, r)
	return w
}

func TestFetchSourcesIncludesInactive(t *testing.T) {
	a, db := sourceTestApp(t)
	w := sourceRequest(a, http.MethodGet, "/sources", "")
	if w.Code != http.StatusOK || strings.TrimSpace(w.Body.String()) != "[]" {
		t.Fatalf("empty sources: status=%d body=%s", w.Code, w.Body.String())
	}
	if _, err := db.Exec(`
		INSERT INTO sources (id, name, url, is_seeded) VALUES (1, 'Science', 'https://science.example/rss', 1), (2, 'Sports', 'https://sports.example/rss', 1);
		INSERT INTO user_sources (user_id, source_id, is_active) VALUES (1, 1, 1), (1, 2, 0), (2, 1, 0), (2, 2, 1)
	`); err != nil {
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
	if err := db.QueryRow(`SELECT s.name, s.url, us.is_active FROM sources s JOIN user_sources us ON us.source_id = s.id WHERE us.user_id = 1 AND s.id = 1`).Scan(&name, &url, &active); err != nil {
		t.Fatal(err)
	}
	if name != "Science" || url != "https://science.example/rss" || !active {
		t.Fatalf("unexpected inserted source: %q %q %v", name, url, active)
	}
	var seeded bool
	if err := db.QueryRow(`SELECT is_seeded FROM sources WHERE id = 1`).Scan(&seeded); err != nil || seeded {
		t.Fatalf("user-added source marked seeded: seeded=%v err=%v", seeded, err)
	}
	var otherSubscriptions int
	if err := db.QueryRow(`SELECT COUNT(*) FROM user_sources WHERE user_id = 2`).Scan(&otherSubscriptions); err != nil || otherSubscriptions != 0 {
		t.Fatalf("request changed user 2 subscriptions: count=%d err=%v", otherSubscriptions, err)
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
			if _, err := db.Exec(`
				INSERT INTO sources (id, name, url) VALUES (1, 'Original', 'https://original.example/rss');
				INSERT INTO user_sources (user_id, source_id, is_active) VALUES (1, 1, 1), (2, 1, 1)
			`); err != nil {
				t.Fatal(err)
			}
			body := `{"1":{"name":" Renamed ","url":" https://renamed.example/rss ","is_active":false}`
			wantStatus := http.StatusOK
			if missing {
				body += `,"999":{"name":"Missing","url":"https://missing.example/rss","is_active":true}`
				wantStatus = http.StatusNotFound
			}
			w := sourceRequest(a, http.MethodPost, "/change-sources", body+"}")
			if w.Code != wantStatus {
				t.Fatalf("status=%d, want %d; body=%s", w.Code, wantStatus, w.Body.String())
			}
			var name, url string
			var active bool
			wantURL := "https://renamed.example/rss"
			if missing {
				wantURL = "https://original.example/rss"
			}
			if err := db.QueryRow(`SELECT s.name, s.url, us.is_active FROM sources s JOIN user_sources us ON us.source_id = s.id WHERE us.user_id = 1 AND s.url = ?`, wantURL).Scan(&name, &url, &active); err != nil {
				t.Fatal(err)
			}
			if missing {
				if name != "Original" || url != "https://original.example/rss" || !active {
					t.Fatalf("missing ID changed source: %q %q %v", name, url, active)
				}
			} else if name != "Renamed" || url != "https://renamed.example/rss" || active {
				t.Fatalf("source not updated: %q %q %v", name, url, active)
			}
			var otherActive bool
			if err := db.QueryRow(`SELECT is_active FROM user_sources WHERE user_id = 2 AND source_id = 1`).Scan(&otherActive); err != nil || !otherActive {
				t.Fatalf("request changed user 2 subscription: active=%v err=%v", otherActive, err)
			}
			var originalName, originalURL string
			if err := db.QueryRow(`SELECT name, url FROM sources WHERE id = 1`).Scan(&originalName, &originalURL); err != nil || originalName != "Original" || originalURL != "https://original.example/rss" {
				t.Fatalf("request rewrote shared source: name=%q url=%q err=%v", originalName, originalURL, err)
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

func sessionTestApp(t *testing.T) (*app, *sql.DB) {
	t.Helper()
	return schemaTestApp(t)
}

func TestUpdateTokenHandler(t *testing.T) {
	for _, tc := range []struct {
		name       string
		cookie     string
		expired    bool
		dbFailure  bool
		wantStatus int
	}{
		{name: "renews the same profile", cookie: "original-token", wantStatus: http.StatusOK},
		{name: "rejects missing cookie", wantStatus: http.StatusUnauthorized},
		{name: "rejects unknown token", cookie: "unknown-token", wantStatus: http.StatusUnauthorized},
		{name: "rejects expired token", cookie: "original-token", expired: true, wantStatus: http.StatusUnauthorized},
		{name: "database failure does not issue a cookie", cookie: "original-token", dbFailure: true, wantStatus: http.StatusInternalServerError},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a, db := sessionTestApp(t)
			ctx := context.Background()
			originalHash := hashSession("original-token")
			originalExpiry := time.Now().UTC().Add(time.Hour)
			if tc.expired {
				originalExpiry = time.Now().UTC().Add(-time.Hour)
			}
			if err := a.store.CreateAnonymousSession(ctx, originalHash, originalExpiry); err != nil {
				t.Fatal(err)
			}
			if tc.dbFailure {
				if err := db.Close(); err != nil {
					t.Fatal(err)
				}
			}

			r := httptest.NewRequest(http.MethodPost, "/update-token", nil)
			if tc.cookie != "" {
				r.AddCookie(&http.Cookie{Name: CookieName, Value: tc.cookie})
			}
			w := httptest.NewRecorder()
			routes(a).ServeHTTP(w, r)
			if w.Code != tc.wantStatus {
				t.Fatalf("status = %d, want %d; body: %s", w.Code, tc.wantStatus, w.Body.String())
			}
			cookies := w.Result().Cookies()
			if tc.wantStatus != http.StatusOK {
				if len(cookies) != 0 {
					t.Fatal("failed renewal issued a cookie")
				}
				if !tc.dbFailure {
					var token string
					var expiry time.Time
					if err := db.QueryRow(`SELECT token, expires_at FROM users WHERE id = 1`).Scan(&token, &expiry); err != nil {
						t.Fatal(err)
					}
					if token != originalHash || !expiry.Equal(originalExpiry) {
						t.Fatal("failed renewal changed the session")
					}
				}
				return
			}

			if len(cookies) != 1 {
				t.Fatalf("cookie count = %d, want 1", len(cookies))
			}
			cookie := cookies[0]
			if cookie.Name != CookieName || cookie.Value == "" || cookie.Value == tc.cookie {
				t.Fatal("renewal did not issue a replacement session token")
			}
			if cookie.Path != "/" || !cookie.HttpOnly || !cookie.Secure || cookie.SameSite != http.SameSiteLaxMode {
				t.Fatalf("unexpected cookie attributes: %+v", cookie)
			}
			if cookie.MaxAge != 15*24*60*60 || !cookie.Expires.After(time.Now().UTC().Add(14*24*time.Hour)) {
				t.Fatal("replacement cookie does not have the expected lifetime")
			}
			if _, err := a.store.FetchUserID(ctx, originalHash); !errors.Is(err, sql.ErrNoRows) {
				t.Fatalf("old token lookup = %v, want sql.ErrNoRows", err)
			}
			if id, err := a.store.FetchUserID(ctx, hashSession(cookie.Value)); err != nil || id != 1 {
				t.Fatalf("replacement token lookup = (%d, %v), want (1, nil)", id, err)
			}
			var count int
			if err := db.QueryRow(`SELECT COUNT(*) FROM users`).Scan(&count); err != nil || count != 1 {
				t.Fatalf("user count = (%d, %v), want (1, nil)", count, err)
			}

			// Replaying the old cookie cannot rotate the new token again.
			replay := httptest.NewRecorder()
			routes(a).ServeHTTP(replay, r)
			if replay.Code != http.StatusUnauthorized || len(replay.Result().Cookies()) != 0 {
				t.Fatal("old cookie was accepted after rotation")
			}
		})
	}
}

func TestUpdateTokenRouteRejectsGET(t *testing.T) {
	a, _ := sessionTestApp(t)
	w := httptest.NewRecorder()
	routes(a).ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/update-token", nil))
	if w.Code != http.StatusMethodNotAllowed || len(w.Result().Cookies()) != 0 {
		t.Fatalf("GET status = %d, want 405 without a cookie", w.Code)
	}
}

func TestSetTokenReusesExistingProfile(t *testing.T) {
	a, db := sessionTestApp(t)
	created := httptest.NewRecorder()
	routes(a).ServeHTTP(created, httptest.NewRequest(http.MethodPost, "/set-token", nil))
	if created.Code != http.StatusOK || len(created.Result().Cookies()) != 1 {
		t.Fatalf("create status=%d body=%s", created.Code, created.Body.String())
	}
	cookie := created.Result().Cookies()[0]
	userID, err := a.store.FetchUserID(context.Background(), hashSession(cookie.Value))
	if err != nil {
		t.Fatal(err)
	}
	repeated := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/set-token", nil)
	r.AddCookie(cookie)
	routes(a).ServeHTTP(repeated, r)
	if repeated.Code != http.StatusOK || len(repeated.Result().Cookies()) != 0 {
		t.Fatal("valid session was replaced")
	}
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM users`).Scan(&count); err != nil || count != 1 {
		t.Fatalf("user count=%d err=%v", count, err)
	}
	if id, err := a.store.FetchUserID(context.Background(), hashSession(cookie.Value)); err != nil || id != userID {
		t.Fatalf("profile changed: id=%d err=%v", id, err)
	}
}

func TestPersonalRoutesRequireSession(t *testing.T) {
	a, _ := sessionTestApp(t)
	for _, tc := range []struct{ method, path string }{
		{http.MethodGet, "/today"}, {http.MethodGet, "/interests"}, {http.MethodGet, "/sources"},
		{http.MethodPost, "/articles/1/read"}, {http.MethodPost, "/change-interests"}, {http.MethodPost, "/sources"},
	} {
		w := httptest.NewRecorder()
		routes(a).ServeHTTP(w, httptest.NewRequest(tc.method, tc.path, nil))
		if w.Code != http.StatusUnauthorized {
			t.Fatalf("%s %s status=%d", tc.method, tc.path, w.Code)
		}
	}
}

func TestReadHandlerUsesCookieIdentity(t *testing.T) {
	a, db := userTestApp(t)
	ctx := context.Background()
	for _, userID := range []int64{1, 2} {
		if err := a.store.InsertSources(ctx, userID, []database.SourcesUpdate{{Name: "Shared", URL: "https://example.com/rss", IsActive: true}}); err != nil {
			t.Fatal(err)
		}
	}
	if err := a.store.InsertArticles(ctx, []database.Article{{SourceID: sql.NullInt64{Int64: 1, Valid: true}, Title: "Article", URL: "https://example.com/article", BatchDate: time.Now().UTC()}}); err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest(http.MethodPost, "/articles/1/read?user_id=2", nil)
	r.AddCookie(&http.Cookie{Name: CookieName, Value: "handler-user-1"})
	w := httptest.NewRecorder()
	routes(a).ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("read status=%d body=%s", w.Code, w.Body.String())
	}
	for _, tc := range []struct {
		userID int64
		count  int
	}{{1, 1}, {2, 0}} {
		var count int
		if err := db.QueryRow(`SELECT COUNT(*) FROM user_articles WHERE user_id=? AND read_at IS NOT NULL`, tc.userID).Scan(&count); err != nil || count != tc.count {
			t.Fatalf("user %d reads=%d err=%v", tc.userID, count, err)
		}
	}
	r = httptest.NewRequest(http.MethodGet, "/today", nil)
	r.AddCookie(&http.Cookie{Name: CookieName, Value: "handler-user-2"})
	w = httptest.NewRecorder()
	routes(a).ServeHTTP(w, r)
	var articles []articleResponse
	if err := json.Unmarshal(w.Body.Bytes(), &articles); err != nil || w.Code != http.StatusOK || len(articles) != 1 {
		t.Fatalf("other reader feed status=%d articles=%+v err=%v", w.Code, articles, err)
	}
}

func TestSessionMutationRejectsCrossOriginBrowserRequest(t *testing.T) {
	a, db := sessionTestApp(t)
	r := httptest.NewRequest(http.MethodPost, "/set-token", nil)
	r.Header.Set("Origin", "https://untrusted.example")
	r.Header.Set("Sec-Fetch-Site", "cross-site")
	w := httptest.NewRecorder()
	routes(a).ServeHTTP(w, r)
	if w.Code != http.StatusForbidden || len(w.Result().Cookies()) != 0 {
		t.Fatalf("cross-origin status=%d", w.Code)
	}
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM users`).Scan(&count); err != nil || count != 0 {
		t.Fatalf("cross-origin request created a profile: count=%d err=%v", count, err)
	}
}
