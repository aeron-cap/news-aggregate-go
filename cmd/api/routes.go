package main

import "net/http"

func routes(app *app) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", health)

	mux.HandleFunc("GET /build-feed", app.buildFeed)
	mux.HandleFunc("GET /today", app.fetchFeed)
	mux.HandleFunc("POST /articles/{id}/read", app.markAsRead)

	mux.HandleFunc("GET /interests", app.fetchInterests)
	mux.HandleFunc("POST /change-interests", app.updateInterests)

	mux.HandleFunc("GET /sources", app.fetchSources)
	mux.HandleFunc("POST /sources", app.insertSources)
	mux.HandleFunc("POST /change-sources", app.updateSources)

	return app.cors(app.logging(mux))
}
