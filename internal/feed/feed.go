package feed

import (
	"context"
	"database/sql"
	"time"

	"github.com/aeron-cap/news-aggregator/internal/database"
)

func BuildFeed(ctx context.Context, s *database.Store, articles []Article) error {
	interests, err := s.GetInterests(ctx)
	if err != nil {
		return err
	}
	interestConfig := NewInterestConfig(interests)

	currentTime := time.Now().UTC()
	for i, _ := range articles {
		articles[i].RelevanceScore = relevance(articles[i], interestConfig)
		if articles[i].RelevanceScore != 0 {
			articles[i].RecencyScore = recency(articles[i], currentTime)
		} else {
			articles[i].RecencyScore = 0
		}

		articles[i].WeightedScore = weightedScore(articles[i])
	}

	articles = giveTopArticles(10, limitAuthors(1, filterOldArticles(articles, 1, 0, 0)))

	dbArticles := make([]database.Article, 0, len(articles))
	
    for _, article := range articles {
        var summary sql.NullString
        if article.Summary != "" {
            summary = sql.NullString{String: article.Summary, Valid: true}
        }
	
        var author sql.NullString
        if len(article.Authors) > 0 {
            author = sql.NullString{String: article.Authors[0].Name, Valid: true}
        }
	
        var sourceDate sql.NullTime
        if article.Date != "" {
            parsedDate, err := time.Parse(time.RFC3339, article.Date)
            if err == nil {
                sourceDate = sql.NullTime{Time: parsedDate, Valid: true}
            }
        }
	
        dbArticle := database.Article{
        	SourceID:      sql.NullInt64{Int64: article.SourceID, Valid: true},
            Title:         article.Title,
            Summary:       summary,
            Author:        author,
            URL:           article.Link,
            SourceDate:    sourceDate,
            WeightedScore: article.WeightedScore,
            BatchDate:     currentTime,
        }
	
        dbArticles = append(dbArticles, dbArticle)
    }
	
    if err := s.InsertArticles(ctx, dbArticles); err != nil {
        return err
    }
	
    return nil
}