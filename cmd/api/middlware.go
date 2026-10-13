package main

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"time"
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

func (a *app) cors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(w, r)
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

		userID, err := a.store.FetchUserID(r.Context(), hashSession(cookie.Value))
		if errors.Is(err, sql.ErrNoRows) {
			http.Error(w, "Invalid session", http.StatusUnauthorized)
			return
		}
		if err != nil {
			a.logger.ErrorContext(r.Context(), "session lookup failed", "err", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		ctx := context.WithValue(r.Context(), userIDKey{}, userID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func requestUserID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	userID, ok := r.Context().Value(userIDKey{}).(int64)
	if !ok || userID <= 0 {
		http.Error(w, "Session required", http.StatusUnauthorized)
		return 0, false
	}
	return userID, true
}
