package main

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"time"

	"github.com/google/uuid"
)

func (a *app) logging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		defer func() {
			if v := recover(); v != nil {
				a.logger.Error("panic", "err", v, "path", r.URL.Path)
				http.Error(w, "internal error", http.StatusInternalServerError)
			}
		}()

		next.ServeHTTP(w, r)
		a.logger.Info("request", "method", r.Method, "path", r.URL.Path, "duration_ms", time.Since(start).Milliseconds())
	})
}

type userIDKey struct{}

func (a *app) requireSession(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie(CookieName)
		if err != nil {
			http.Error(w, "Session required", http.StatusUnauthorized)
			return
		}

		userID, expiresAt, err := a.store.FetchSession(r.Context(), hashSession(cookie.Value))
		if errors.Is(err, sql.ErrNoRows) {
			http.Error(w, "Invalid session", http.StatusUnauthorized)
			return
		}
		if err != nil {
			a.logger.ErrorContext(r.Context(), "session lookup failed", "err", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		a.extendSession(w, r, cookie.Value, expiresAt)

		ctx := context.WithValue(r.Context(), userIDKey{}, userID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func (a *app) extendSession(w http.ResponseWriter, r *http.Request, token string, expiresAt time.Time) {
	now := time.Now().UTC()
	refreshBefore := now.Add(sessionTTL - sessionRefreshInterval)
	if !expiresAt.Before(refreshBefore) {
		return
	}

	newExpiry := now.Add(sessionTTL)
	extended, err := a.store.ExtendSession(r.Context(), hashSession(token), newExpiry, refreshBefore)
	if err != nil {
		a.logger.WarnContext(r.Context(), "extend session failed", "err", err)
		return
	}
	if extended {
		setSessionCookie(w, token, newExpiry)
	}
}

func setSessionCookie(w http.ResponseWriter, token string, expiresAt time.Time) {
	http.SetCookie(w, &http.Cookie{
		Name:     CookieName,
		Value:    token,
		Path:     "/",
		Expires:  expiresAt,
		MaxAge:   int(time.Until(expiresAt).Seconds()),
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
	})
}

func requestUserID(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	userID, ok := r.Context().Value(userIDKey{}).(uuid.UUID)
	if !ok || userID == uuid.Nil {
		http.Error(w, "Session required", http.StatusUnauthorized)
		return uuid.Nil, false
	}
	return userID, true
}
