package main

import (
	"encoding/json"
	"net/http"

	"github.com/aeron-cap/news-aggregator/internal/feed"
)

func health(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("OK"))
}

func (a *app) buildFeed(w http.ResponseWriter, r *http.Request) {
	err := feed.CreateFeed(a.store)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{
			"error":   "Failed to fetch articles",
			"details": err.Error(),
		})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{
		"details": "Articles fetched and stored successfully",
	})
}
