package main

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/aeron-cap/news-aggregator/internal/feed"
	"github.com/aeron-cap/news-aggregator/internal/scripts"
)

func health(w http.ResponseWriter, r *http.Request) {
	if isShuttingDown.Load() {
		http.Error(w, "Server is shutting down", http.StatusServiceUnavailable)
		return
	}

	w.WriteHeader(http.StatusOK)
	w.Write([]byte("OK"))
}

func (a *app) buildFeed(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	refreshed, err := feed.CreateFeed(ctx, a.store)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{
			"error":   "Failed to fetch articles",
			"details": err.Error(),
		})
		return
	}

	if !refreshed {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{
			"details": "Feed still fresh",
		})
		return
	}

	a.cache.clear()

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{
		"details": "Articles fetched and stored successfully",
	})
}

type interestResponse struct {
	ID       int64   `json:"id"`
	Keyword  string  `json:"keyword"`
	Weight   float64 `json:"weight"`
	IsMain   bool    `json:"is_main"`
	IsActive bool    `json:"is_active"`
}

func (a *app) fetchInterests(w http.ResponseWriter, r *http.Request) {
	interests, err := a.store.GetInterests(r.Context())
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{
			"error":   "Failed to fetch interests",
			"details": err.Error(),
		})
		return
	}

	payload := make([]interestResponse, 0, len(interests))
	for _, interest := range interests {
		payload = append(payload, interestResponse{
			ID:       interest.ID,
			Keyword:  interest.Keyword,
			Weight:   interest.Weight,
			IsMain:   interest.IsMain,
			IsActive: interest.IsActive,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(payload)
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
	SourceName    *string    `json:"source_name"`
}

func (a *app) fetchFeed(w http.ResponseWriter, r *http.Request) {
	data, hit := a.cache.get()
	if hit {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write(data)
		return
	}

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

	if len(articles) == 0 {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode([]articleResponse{})
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
			SourceName:    feed.NullString(article.SourceName),
		})
	}

	b, err := json.Marshal(payload)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{
			"error":   "Failed to marshal articles",
			"details": err.Error(),
		})
		return
	}

	a.cache.set(b, 12*time.Hour)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(payload)
}

func (a *app) markAsRead(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)

	if id == 0 {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{
			"error": "Invalid article ID",
		})
		return
	}

	err := a.store.MarkArticleAsRead(r.Context(), id)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{
			"error":   "Failed to mark article as read",
			"details": err.Error(),
		})
		return
	}

	a.cache.clear()

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{
		"details": "Article marked as read successfully",
	})
}

type interestPayload struct {
	ID       int64 `json:"id"`
	IsActive bool  `json:"is_active"`
}

func (a *app) changeInterest(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 1048576)

	var payload interestPayload
	err := json.NewDecoder(r.Body).Decode(&payload)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{
			"error":   "Invalid request payload",
			"details": err.Error(),
		})
		return
	}

	id := payload.ID
	isActive := payload.IsActive
	
	if id <= 0 {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{
			"error": "Invalid interest ID",
		})
		return
	}

	err = a.store.UpdateInterestActivation(r.Context(), id, isActive)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{
			"error":   "Failed to update interest activation",
			"details": err.Error(),
		})
		return
	}

	err = scripts.RunResnik(r.Context(), "internal/scripts/resnik.py")
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{
			"error":   "Failed to run resnik script",
			"details": err.Error(),
		})
		return
	}
	
	a.cache.clear()

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{
		"details": "Interest updated successfully",
	})
}
