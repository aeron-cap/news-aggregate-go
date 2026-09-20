package main

import (
	"encoding/json"
	"net/http"
	"time"

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

type articleResponse struct {
	ID            int64      `json:"id"`
	Title         string     `json:"title"`
	Summary       *string    `json:"summary"`
	Author        *string    `json:"author"`
	URL           string     `json:"url"`
	SourceDate    *time.Time `json:"source_date"`
	WeightedScore float64    `json:"weighted_score"`
	BatchDate     time.Time  `json:"batch_date"`
}

func (a *app) fetchFeed(w http.ResponseWriter, r *http.Request) {
	articles, err := a.store.GetUnreadArticles(r.Context()) 
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{
			"error":   "Failed to fetch articles",
			"details": err.Error(),
		})
		return
	}

	payload := make([]articleResponse, 0, len(articles))
	for _, article := range articles {
		payload = append(payload, articleResponse{
			ID:            article.ID,
			Title:         article.Title,
			Summary:       feed.NullString(article.Summary),
			Author:        feed.NullString(article.Author),
			URL:           article.URL,
			SourceDate:    feed.NullTime(article.SourceDate),
			WeightedScore: article.WeightedScore,
			BatchDate:     article.BatchDate,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(payload)
} 
