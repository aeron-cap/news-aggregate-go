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
		a.logger.ErrorContext(ctx, "build feed failed", "err", err)
		respondWithError(w, http.StatusInternalServerError, "Failed to build feed", err.Error())
		return
	}

	if !refreshed {
		a.logger.InfoContext(ctx, "feed is already up to date")
		respondWithJSON(w, http.StatusOK, map[string]string{
			"details": "Feed is already up to date",
		})
		return
	}

	a.cache.clear()

	respondWithJSON(w, http.StatusOK, map[string]string{
		"details": "Feed refreshed successfully",
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
		a.logger.ErrorContext(r.Context(), "fetch interests failed", "err", err)
		respondWithError(w, http.StatusInternalServerError, "Failed to fetch interests", err.Error())
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

	respondWithJSON(w, http.StatusOK, payload)
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
		respondWithJSON(w, http.StatusOK, json.RawMessage(data))
		return
	}

	articles, err := a.store.GetUnreadArticles(r.Context())
	if err != nil {
		a.logger.ErrorContext(r.Context(), "fetch unread articles failed", "err", err)
		respondWithError(w, http.StatusInternalServerError, "Failed to fetch unread articles", err.Error())
		return
	}

	if len(articles) == 0 {
		respondWithJSON(w, http.StatusOK, []articleResponse{})
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
		a.logger.ErrorContext(r.Context(), "marshal articles failed", "err", err)
		respondWithError(w, http.StatusInternalServerError, "Failed to marshal articles", err.Error())
		return
	}

	a.cache.set(b, 12*time.Hour)

	respondWithJSON(w, http.StatusOK, payload)
}

func (a *app) markAsRead(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)

	if id == 0 {
		a.logger.WarnContext(r.Context(), "invalid article id", "id_raw", r.PathValue("id"))
		respondWithError(w, http.StatusBadRequest, "Invalid article ID", "ID must be a positive integer")
		return
	}

	err := a.store.MarkArticleAsRead(r.Context(), id)
	if err != nil {
		a.logger.ErrorContext(r.Context(), "mark article as read failed", "id", id, "err", err)
		respondWithError(w, http.StatusInternalServerError, "Failed to mark article as read", err.Error())
		return
	}

	a.cache.clear()

	respondWithJSON(w, http.StatusOK, map[string]string{
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
		a.logger.WarnContext(r.Context(), "invalid interest payload", "err", err)
		respondWithError(w, http.StatusBadRequest, "Invalid request payload", err.Error())
		return
	}

	id := payload.ID
	isActive := payload.IsActive

	if id <= 0 {
		a.logger.WarnContext(r.Context(), "invalid interest id", "id", id)
		respondWithError(w, http.StatusBadRequest, "Invalid interest ID", "ID must be a positive integer")
		return
	}

	err = a.store.UpdateInterestActivation(r.Context(), id, isActive)
	if err != nil {
		a.logger.ErrorContext(r.Context(), "update interest activation failed", "id", id, "is_active", isActive, "err", err)
		respondWithError(w, http.StatusInternalServerError, "Failed to update interest activation", err.Error())
		return
	}

	err = scripts.RunResnik(r.Context(), "internal/scripts/resnik.py")
	if err != nil {
		a.logger.ErrorContext(r.Context(), "run resnik script failed", "id", id, "err", err)
		respondWithError(w, http.StatusInternalServerError, "Failed to run resnik script", err.Error())
		return
	}

	a.cache.clear()

	respondWithJSON(w, http.StatusOK, map[string]string{
		"details": "Interest activation updated and resnik script executed successfully",
	})
}
