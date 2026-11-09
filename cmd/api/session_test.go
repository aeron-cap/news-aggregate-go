package main

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// createSessionExpiringIn creates an anonymous session for token that expires after d (negative = already expired).
func createSessionExpiringIn(t *testing.T, a *app, token string, d time.Duration) time.Time {
	t.Helper()
	expiresAt := time.Now().UTC().Truncate(time.Microsecond).Add(d)
	if err := a.store.CreateAnonymousSession(context.Background(), hashSession(token), expiresAt); err != nil {
		t.Fatal(err)
	}
	return expiresAt
}

func sessionExpiry(t *testing.T, db *sql.DB, token string) time.Time {
	t.Helper()
	var expiresAt time.Time
	if err := db.QueryRow(`SELECT expires_at FROM users WHERE token = $1`, hashSession(token)).Scan(&expiresAt); err != nil {
		t.Fatal(err)
	}
	return expiresAt
}

func sessionRequest(method, path, token string) *http.Request {
	r := httptest.NewRequest(method, path, nil)
	r.AddCookie(&http.Cookie{Name: CookieName, Value: token})
	return r
}

func TestActiveSessionIsExtended(t *testing.T) {
	for _, tc := range []struct{ method, path string }{
		{http.MethodGet, "/interests"},
		{http.MethodPost, "/set-token"},
	} {
		t.Run(tc.method+" "+tc.path, func(t *testing.T) {
			a, db := sessionTestApp(t)
			createSessionExpiringIn(t, a, "active-token", 13*24*time.Hour)

			w := httptest.NewRecorder()
			routes(a).ServeHTTP(w, sessionRequest(tc.method, tc.path, "active-token"))
			if w.Code != http.StatusOK {
				t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
			}

			minExpiry := time.Now().UTC().Add(sessionTTL - time.Minute)
			if got := sessionExpiry(t, db, "active-token"); got.Before(minExpiry) {
				t.Fatalf("expires_at = %v, want about 15 days from now", got)
			}

			cookies := w.Result().Cookies()
			if len(cookies) != 1 {
				t.Fatalf("cookie count = %d, want 1", len(cookies))
			}
			cookie := cookies[0]
			if cookie.Name != CookieName || cookie.Value != "active-token" {
				t.Fatalf("cookie = %s=%s, want the same session token re-issued", cookie.Name, cookie.Value)
			}
			if cookie.Path != "/" || !cookie.HttpOnly || !cookie.Secure || cookie.SameSite != http.SameSiteLaxMode {
				t.Fatalf("unexpected cookie attributes: %+v", cookie)
			}
			if cookie.MaxAge < int((sessionTTL-time.Minute).Seconds()) || cookie.Expires.Before(minExpiry.Truncate(time.Second)) {
				t.Fatalf("cookie lifetime not extended: MaxAge=%d Expires=%v", cookie.MaxAge, cookie.Expires)
			}
		})
	}
}

func TestRecentlyExtendedSessionIsNotRewritten(t *testing.T) {
	a, db := sessionTestApp(t)
	original := createSessionExpiringIn(t, a, "fresh-token", sessionTTL-time.Hour)

	w := httptest.NewRecorder()
	routes(a).ServeHTTP(w, sessionRequest(http.MethodGet, "/interests", "fresh-token"))
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	if len(w.Result().Cookies()) != 0 {
		t.Fatal("cookie re-issued for a session extended less than a day ago")
	}
	if got := sessionExpiry(t, db, "fresh-token"); !got.Equal(original) {
		t.Fatalf("expires_at changed from %v to %v", original, got)
	}
}

func TestUntouchedSessionStillExpires(t *testing.T) {
	a, db := sessionTestApp(t)
	original := createSessionExpiringIn(t, a, "stale-token", -time.Minute)

	w := httptest.NewRecorder()
	routes(a).ServeHTTP(w, sessionRequest(http.MethodGet, "/interests", "stale-token"))
	if w.Code != http.StatusUnauthorized || len(w.Result().Cookies()) != 0 {
		t.Fatalf("status=%d cookies=%d, want 401 without a cookie", w.Code, len(w.Result().Cookies()))
	}
	if got := sessionExpiry(t, db, "stale-token"); !got.Equal(original) {
		t.Fatalf("expired session was extended to %v", got)
	}

	a.deleteExpiredSessions(context.Background())
	if _, err := a.store.FetchUserID(context.Background(), hashSession("stale-token")); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("lookup after cleanup = %v, want sql.ErrNoRows", err)
	}
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM users`).Scan(&count); err != nil || count != 0 {
		t.Fatalf("user count = (%d, %v), want (0, nil)", count, err)
	}
}

func TestUpdateTokenRouteIsRemoved(t *testing.T) {
	a, _ := sessionTestApp(t)
	w := httptest.NewRecorder()
	routes(a).ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/update-token", nil))
	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", w.Code)
	}
}
