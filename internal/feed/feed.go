package feed

import (
	"time"
)

var keywords = []string{
	"go",
	"golang",
	"frontend",
	"front end",
	"backend",
	"back end",
	"coding",
	"programming",
	"software engineering",
	"software",
	"typescript",
	"ts",
	"database",
	"orm",
	"orms",
	"sql",
	"sqlite",
	"postgres",
	"postgresql",
	"mysql",
	"linux",
}

func BuildFeed(articles []Article) []Article {
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
