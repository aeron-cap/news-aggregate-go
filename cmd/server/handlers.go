package main

import (
	"fmt"
	"net/http"

	"github.com/aeron-cap/news-aggregator/internal/feed"
)

func health(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("OK"))
}

func getFeeds(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	articles, err := feed.Fetch()
	if err != nil {
		http.Error(w, "Failed to fetch feeds", http.StatusInternalServerError)
		return
	}

	for _, article := range articles {
		w.Write([]byte(article.Title + "\n" + article.Link + "\n\n" + article.Date + "\n\n" + fmt.Sprintf("%f = %f + %f", article.WeightedScore, article.RelevanceScore, article.RecencyScore) + "\n\n"))
	}
}
