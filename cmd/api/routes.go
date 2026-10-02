package main

import "net/http"

func routes(app *app) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", health)
	mux.HandleFunc("GET /build-feed", app.buildFeed)
	mux.HandleFunc("GET /today", app.fetchFeed)
	mux.HandleFunc("POST /articles/{id}/read", app.markAsRead)
	mux.HandleFunc("GET /interests", app.fetchInterests)
	mux.HandleFunc("POST /change-interest", app.changeInterest)

	return app.cors(app.logging(mux))
}