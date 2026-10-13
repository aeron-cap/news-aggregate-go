package main

import "net/http"

func routes(app *app) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", health)

	mux.HandleFunc("POST /set-token", app.setToken)
	mux.HandleFunc("POST /update-token", app.updateToken)

	mux.Handle("GET /build-feed", app.requireSession(http.HandlerFunc(app.buildFeed)))
	mux.Handle("GET /today", app.requireSession(http.HandlerFunc(app.fetchFeed)))
	mux.Handle("POST /articles/{id}/read", app.requireSession(http.HandlerFunc(app.markAsRead)))

	mux.Handle("GET /interests", app.requireSession(http.HandlerFunc(app.fetchInterests)))
	mux.Handle("POST /change-interests", app.requireSession(http.HandlerFunc(app.updateInterests)))

	mux.Handle("GET /sources", app.requireSession(http.HandlerFunc(app.fetchSources)))
	mux.Handle("POST /sources", app.requireSession(http.HandlerFunc(app.insertSources)))
	mux.Handle("POST /change-sources", app.requireSession(http.HandlerFunc(app.updateSources)))

	return app.cors(app.logging(http.NewCrossOriginProtection().Handler(mux)))
}
