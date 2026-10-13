package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/aeron-cap/news-aggregator/internal/database"
	"github.com/aeron-cap/news-aggregator/internal/feed"
	"github.com/aeron-cap/news-aggregator/internal/scripts"
)

const CookieName = "news_agg_session"

func health(w http.ResponseWriter, r *http.Request) {

	if isShuttingDown.Load() {
		http.Error(w, "Server is shutting down", http.StatusServiceUnavailable)
		return
	}

	w.WriteHeader(http.StatusOK)
	w.Write([]byte("OK"))
}

func (a *app) setToken(w http.ResponseWriter, r *http.Request) {
	if cookie, err := r.Cookie(CookieName); err == nil {
		if _, err := a.store.FetchUserID(r.Context(), hashSession(cookie.Value)); err == nil {
			w.Header().Set("Cache-Control", "no-store")
			respondWithJSON(w, http.StatusOK, map[string]string{"details": "Session already exists"})
			return
		} else if !errors.Is(err, sql.ErrNoRows) {
			a.logger.ErrorContext(r.Context(), "session lookup failed", "err", err)
			respondWithError(w, http.StatusInternalServerError, "Failed to load session", "Please try again later")
			return
		}
	}
	token, tokenHash := newSessionToken()
	expiresAt := time.Now().UTC().Add(15 * 24 * time.Hour)

	err := a.store.CreateAnonymousSession(r.Context(), tokenHash, expiresAt)
	if err != nil {
		a.logger.ErrorContext(r.Context(), "saving cookie failed", "err", err)
		respondWithError(w, http.StatusInternalServerError, "Failed to set cookie", err.Error())
		return
	}

	cookie := http.Cookie{
		Name:     CookieName,
		Value:    token,
		Path:     "/",
		Expires:  expiresAt,
		MaxAge:   int(expiresAt.Sub(time.Now().UTC()).Seconds()),
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
	}

	http.SetCookie(w, &cookie)
	w.Header().Set("Cache-Control", "no-store")
	a.logger.InfoContext(r.Context(), "cookie set successfully", "cookie_name", CookieName, "expires_at", expiresAt)
	respondWithJSON(w, http.StatusOK, map[string]string{
		"details": "Cookie set successfully",
	})

	return
}

func (a *app) updateToken(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie(CookieName)
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, "Session required", "A valid session cookie is required")
		return
	}

	token, tokenHash := newSessionToken()
	expiresAt := time.Now().UTC().Add(15 * 24 * time.Hour)
	err = a.store.UpdateToken(r.Context(), hashSession(cookie.Value), tokenHash, expiresAt)
	if errors.Is(err, sql.ErrNoRows) {
		respondWithError(w, http.StatusUnauthorized, "Invalid or expired session", "Create a new session to continue")
		return
	}
	if err != nil {
		a.logger.ErrorContext(r.Context(), "update session token failed", "err", err)
		respondWithError(w, http.StatusInternalServerError, "Failed to update session token", "Please try again later")
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:     CookieName,
		Value:    token,
		Path:     "/",
		Expires:  expiresAt,
		MaxAge:   int((15 * 24 * time.Hour) / time.Second),
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
	})
	w.Header().Set("Cache-Control", "no-store")
	respondWithJSON(w, http.StatusOK, map[string]string{
		"details": "Session token updated successfully",
	})
}

func (a *app) buildFeed(w http.ResponseWriter, r *http.Request) {
	userID, ok := requestUserID(w, r)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	refreshed, err := feed.CreateFeed(ctx, a.store, userID)
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

	respondWithJSON(w, http.StatusOK, map[string]string{
		"details": "Feed refreshed successfully",
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
	SourceName    *string    `json:"source_name"`
}

func (a *app) fetchFeed(w http.ResponseWriter, r *http.Request) {
	userID, ok := requestUserID(w, r)
	if !ok {
		return
	}
	articles, err := feed.GetFeed(r.Context(), a.store, userID)
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

	respondWithJSON(w, http.StatusOK, payload)
}

func (a *app) markAsRead(w http.ResponseWriter, r *http.Request) {
	userID, ok := requestUserID(w, r)
	if !ok {
		return
	}
	id, parseErr := strconv.ParseInt(r.PathValue("id"), 10, 64)

	if parseErr != nil || id <= 0 {
		a.logger.WarnContext(r.Context(), "invalid article id", "id_raw", r.PathValue("id"))
		respondWithError(w, http.StatusBadRequest, "Invalid article ID", "ID must be a positive integer")
		return
	}

	err := a.store.MarkArticleAsRead(r.Context(), userID, id)
	if errors.Is(err, sql.ErrNoRows) {
		respondWithError(w, http.StatusNotFound, "Article not found", "The article is not in your active subscriptions")
		return
	}
	if err != nil {
		a.logger.ErrorContext(r.Context(), "mark article as read failed", "id", id, "err", err)
		respondWithError(w, http.StatusInternalServerError, "Failed to mark article as read", err.Error())
		return
	}

	respondWithJSON(w, http.StatusOK, map[string]string{
		"details": "Article marked as read successfully",
	})
}

type interestResponse struct {
	ID       int64   `json:"id"`
	Keyword  string  `json:"keyword"`
	Weight   float64 `json:"weight"`
	IsMain   bool    `json:"is_main"`
	IsActive bool    `json:"is_active"`
}

type interestPayload struct {
	IsActive *bool `json:"isActive"`
	IsMain   *bool `json:"isMain"`
}

func (a *app) fetchInterests(w http.ResponseWriter, r *http.Request) {
	userID, ok := requestUserID(w, r)
	if !ok {
		return
	}
	interests, err := a.store.GetInterests(r.Context(), userID)
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

func (a *app) updateInterests(w http.ResponseWriter, r *http.Request) {
	userID, ok := requestUserID(w, r)
	if !ok {
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 1048576)

	var payload map[int64]interestPayload
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	err := decoder.Decode(&payload)
	if err != nil {
		a.logger.WarnContext(r.Context(), "invalid interest payload", "err", err)
		respondWithError(w, http.StatusBadRequest, "Invalid request payload", err.Error())
		return
	}

	if err := decoder.Decode(new(any)); err != io.EOF {
		respondWithError(w, http.StatusBadRequest, "Invalid request payload", "Body must contain exactly one JSON object")
		return
	}

	if len(payload) == 0 {
		respondWithError(w, http.StatusBadRequest, "Invalid request payload", "At least one interest is required")
		return
	}

	updates := make(map[int64]database.InterestUpdate, len(payload))
	for id, interest := range payload {
		if id <= 0 {
			a.logger.WarnContext(r.Context(), "invalid interest id", "id", id)
			respondWithError(w, http.StatusBadRequest, "Invalid interest ID", "IDs must be positive integers")
			return
		}

		if interest.IsActive == nil || interest.IsMain == nil {
			respondWithError(w, http.StatusBadRequest, "Invalid request payload", "Each interest must include boolean isActive and isMain fields")
			return
		}

		updates[id] = database.InterestUpdate{
			IsActive: *interest.IsActive,
			IsMain:   *interest.IsMain,
		}
	}

	err = a.store.UpdateInterests(r.Context(), userID, updates)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			respondWithError(w, http.StatusNotFound, "Interest not found", err.Error())
			return
		}
		a.logger.ErrorContext(r.Context(), "update interests failed", "err", err)
		respondWithError(w, http.StatusInternalServerError, "Failed to update interests", err.Error())
		return
	}

	dbPath, err := a.store.DatabasePath(r.Context())
	if err != nil {
		a.logger.ErrorContext(r.Context(), "resolve scoring database failed", "err", err)
		respondWithError(w, http.StatusInternalServerError, "Interests updated, but weight recalculation failed", "Unable to open the scoring database")
		return
	}
	err = scripts.RunResnik(r.Context(), "internal/database/resnik.py", userID, dbPath)
	if err != nil {
		a.logger.ErrorContext(r.Context(), "run resnik script failed", "err", err)
		respondWithError(w, http.StatusInternalServerError, "Interests updated, but weight recalculation failed", err.Error())
		return
	}

	respondWithJSON(w, http.StatusOK, map[string]string{
		"details": "Interests updated and weights recalculated successfully",
	})
}

type sourcesResponse struct {
	ID       int64  `json:"id"`
	Name     string `json:"name"`
	URL      string `json:"url"`
	IsActive bool   `json:"is_active"`
}

type sourcesPayload struct {
	Name     string `json:"name"`
	URL      string `json:"url"`
	IsActive *bool  `json:"is_active"`
}

func (source sourcesPayload) validatedSourceUpdate() (database.SourcesUpdate, error) {
	name := strings.TrimSpace(source.Name)
	feedURL := strings.TrimSpace(source.URL)
	if name == "" || feedURL == "" || source.IsActive == nil {
		return database.SourcesUpdate{}, errors.New("Each source must include non-empty name and url fields and a boolean is_active field")
	}
	parsed, err := url.Parse(feedURL)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Hostname() == "" {
		return database.SourcesUpdate{}, errors.New("Each source URL must be an absolute HTTP or HTTPS URL")
	}
	return database.SourcesUpdate{Name: name, URL: feedURL, IsActive: *source.IsActive}, nil
}

func (a *app) fetchSources(w http.ResponseWriter, r *http.Request) {
	userID, ok := requestUserID(w, r)
	if !ok {
		return
	}
	sources, err := a.store.GetAllSources(r.Context(), userID)
	if err != nil {
		a.logger.ErrorContext(r.Context(), "fetch sources failed", "err", err)
		respondWithError(w, http.StatusInternalServerError, "Failed to fetch sources", err.Error())
		return
	}

	payload := make([]sourcesResponse, 0, len(sources))
	for _, source := range sources {
		payload = append(payload, sourcesResponse{
			ID:       source.ID,
			Name:     source.Name,
			URL:      source.URL,
			IsActive: source.IsActive,
		})
	}

	respondWithJSON(w, http.StatusOK, payload)
}

func (a *app) insertSources(w http.ResponseWriter, r *http.Request) {
	userID, ok := requestUserID(w, r)
	if !ok {
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 1048576)

	var payload []sourcesPayload
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	err := decoder.Decode(&payload)
	if err != nil {
		a.logger.WarnContext(r.Context(), "invalid sources payload", "err", err)
		respondWithError(w, http.StatusBadRequest, "Invalid sources payload", err.Error())
		return
	}

	if err := decoder.Decode(new(any)); err != io.EOF {
		respondWithError(w, http.StatusBadRequest, "Invalid sources payload", "Body must contain exactly one JSON array")
		return
	}

	if len(payload) == 0 {
		respondWithError(w, http.StatusBadRequest, "Invalid request payload", "At least one source is required")
		return
	}

	updates := make([]database.SourcesUpdate, 0, len(payload))
	for _, source := range payload {
		update, err := source.validatedSourceUpdate()
		if err != nil {
			respondWithError(w, http.StatusBadRequest, "Invalid request payload", err.Error())
			return
		}

		updates = append(updates, update)
	}

	err = a.store.InsertSources(r.Context(), userID, updates)
	if err != nil {
		a.logger.ErrorContext(r.Context(), "insert sources failed", "err", err)
		respondWithError(w, http.StatusInternalServerError, "Failed to insert sources", err.Error())
		return
	}

	respondWithJSON(w, http.StatusOK, map[string]string{
		"details": "Sources inserted successfully",
	})
}

func (a *app) updateSources(w http.ResponseWriter, r *http.Request) {
	userID, ok := requestUserID(w, r)
	if !ok {
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 1048576)

	var payload map[int64]sourcesPayload
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	err := decoder.Decode(&payload)
	if err != nil {
		a.logger.WarnContext(r.Context(), "invalid sources payload", "err", err)
		respondWithError(w, http.StatusBadRequest, "Invalid sources payload", err.Error())
		return
	}

	if err := decoder.Decode(new(any)); err != io.EOF {
		respondWithError(w, http.StatusBadRequest, "Invalid sources payload", "Body must contain exactly one JSON object")
		return
	}

	if len(payload) == 0 {
		respondWithError(w, http.StatusBadRequest, "Invalid request payload", "At least one source is required")
		return
	}

	updates := make(map[int64]database.SourcesUpdate, len(payload))
	for id, source := range payload {
		if id <= 0 {
			a.logger.WarnContext(r.Context(), "invalid source id", "id", id)
			respondWithError(w, http.StatusBadRequest, "Invalid source ID", "IDs must be positive integers")
			return
		}

		update, err := source.validatedSourceUpdate()
		if err != nil {
			respondWithError(w, http.StatusBadRequest, "Invalid request payload", err.Error())
			return
		}

		updates[id] = update
	}

	err = a.store.UpdateSources(r.Context(), userID, updates)
	if err != nil {
		if errors.Is(err, database.ErrSharedSourceName) {
			respondWithError(w, http.StatusConflict, "Cannot rename shared source", err.Error())
			return
		}
		if errors.Is(err, sql.ErrNoRows) {
			respondWithError(w, http.StatusNotFound, "Source not found", err.Error())
			return
		}
		a.logger.ErrorContext(r.Context(), "update sources failed", "err", err)
		respondWithError(w, http.StatusInternalServerError, "Failed to update sources", err.Error())
		return
	}

	respondWithJSON(w, http.StatusOK, map[string]string{
		"details": "Sources updated successfully",
	})
}
