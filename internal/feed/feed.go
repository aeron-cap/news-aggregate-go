package feed

import (
	"context"
	"database/sql"
	"sort"
	"strings"
	"time"

	"github.com/aeron-cap/news-aggregator/internal/database"
)

func BuildFeed(ctx context.Context, store *database.Store, userID int64, articles []Article) error {
	interests, err := store.GetInterests(ctx, userID)
	if err != nil {
		return err
	}
	config := NewInterestConfig(interests)
	now := time.Now().UTC()
	candidates := make([]Article, 0, len(articles))
	for _, article := range articles {
		if strings.TrimSpace(article.Link) == "" || strings.TrimSpace(article.Title) == "" {
			continue
		}
		article.RelevanceScore = relevance(article, config)
		article.RecencyScore = recency(article, now)
		article.WeightedScore = weightedScore(article)
		candidates = append(candidates, article)
	}
	candidates = limitAuthors(1, filterOldArticles(candidates, 1, 0, 0))
	unique := make([]Article, 0, len(candidates))
	seenURLs := map[string]bool{}
	for _, article := range candidates {
		if !seenURLs[article.Link] {
			seenURLs[article.Link] = true
			unique = append(unique, article)
		}
	}
	candidates = giveTopArticles(10, unique)
	stored := make([]database.Article, 0, len(candidates))
	for _, article := range candidates {
		date, err := time.Parse(time.RFC3339, article.Date)
		sourceDate := sql.NullTime{Time: date, Valid: err == nil}
		author := sql.NullString{}
		if len(article.Authors) > 0 {
			author = sql.NullString{String: article.Authors[0].Name, Valid: true}
		}
		stored = append(stored, database.Article{
			SourceID: sql.NullInt64{Int64: article.SourceID, Valid: true},
			Title:    article.Title,
			Summary:  sql.NullString{String: article.Summary, Valid: article.Summary != ""},
			Author:   author, URL: article.Link, SourceDate: sourceDate, BatchDate: now,
		})
	}
	if err := store.InsertArticles(ctx, stored); err != nil {
		return err
	}
	_, err = GetFeed(ctx, store, userID)
	return err
}

func GetFeed(ctx context.Context, store *database.Store, userID int64) ([]database.Article, error) {
	articles, err := store.GetUnreadArticles(ctx, userID)
	if err != nil {
		return nil, err
	}
	interests, err := store.GetInterests(ctx, userID)
	if err != nil {
		return nil, err
	}
	config := NewInterestConfig(interests)
	now := time.Now().UTC()
	for i := range articles {
		stored := &articles[i]
		date := stored.BatchDate
		if stored.SourceDate.Valid {
			date = stored.SourceDate.Time
		}
		candidate := Article{Title: stored.Title, Summary: stored.Summary.String, Link: stored.URL, Date: date.Format(time.RFC3339)}
		candidate.RelevanceScore = relevance(candidate, config)
		candidate.RecencyScore = recency(candidate, now)
		stored.WeightedScore = weightedScore(candidate)
	}
	if err := store.SaveArticleScores(ctx, userID, articles); err != nil {
		return nil, err
	}
	sort.SliceStable(articles, func(i, j int) bool {
		if articles[i].WeightedScore != articles[j].WeightedScore {
			return articles[i].WeightedScore > articles[j].WeightedScore
		}
		if !articles[i].BatchDate.Equal(articles[j].BatchDate) {
			return articles[i].BatchDate.After(articles[j].BatchDate)
		}
		return articles[i].ID < articles[j].ID
	})
	result := []database.Article{}
	seenURLs, seenAuthors := map[string]bool{}, map[string]bool{}
	for _, article := range articles {
		date := article.BatchDate
		if article.SourceDate.Valid {
			date = article.SourceDate.Time
		}
		if date.Before(now.AddDate(-1, 0, 0)) || seenURLs[article.URL] {
			continue
		}
		author := strings.ToLower(strings.TrimSpace(article.Author.String))
		if author != "" && seenAuthors[author] {
			continue
		}
		seenURLs[article.URL] = true
		if author != "" {
			seenAuthors[author] = true
		}
		result = append(result, article)
		if len(result) == 10 {
			break
		}
	}
	return result, nil
}
