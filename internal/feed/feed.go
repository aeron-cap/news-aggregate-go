package feed

import (
	"context"
	"time"

	"github.com/aeron-cap/news-aggregator/internal/database"
)

func BuildFeed(ctx context.Context, s *database.Store, articles []Article) []Article {
	keywords, err := s.GetInterests(ctx)
	if err != nil {
		return nil
	}
	interests := NewInterests(keywords)

	currentTime := time.Now().UTC()
	for i, _ := range articles {
		articles[i].RelevanceScore = relevance(articles[i], interests)
		if articles[i].RelevanceScore != 0 {
			articles[i].RecencyScore = recency(articles[i], currentTime)
		} else {
			articles[i].RecencyScore = 0
		}

		articles[i].WeightedScore = weightedScore(articles[i])
	}

	// date can be edited using frontend later
	articles = filterOldArticles(articles, 1, 0, 0)
	return giveTopArticles(10, articles)
}
