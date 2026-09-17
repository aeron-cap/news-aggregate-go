package main

import (
	"net/http"

	"github.com/aeron-cap/news-aggregator/internal/feed"
)

func health(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("OK"))
}

func (a *app) getFeeds(w http.ResponseWriter, r *http.Request) {
	err := feed.CreateFeed(a.store)
	if err != nil {
		http.Error(w, "Failed to fetch feeds", http.StatusInternalServerError)
	}
}
