package feed

import (
	"sort"
	"strings"
	"time"
)

func sortByDate(by string, articles []Article) []Article {
	switch by {
	case "asc":
		sort.Slice(articles, func(i, j int) bool {
			return articles[j].Date > articles[i].Date
		})
	case "desc":
		sort.Slice(articles, func(i, j int) bool {
			return articles[j].Date < articles[i].Date
		})
	default:
		sort.Slice(articles, func(i, j int) bool {
			return articles[j].Date > articles[i].Date
		})
	}

	return articles
}

func sortByWeightedScore(by string, articles []Article) []Article {
	switch by {
	case "asc":
		sort.Slice(articles, func(i, j int) bool {
			return articles[j].WeightedScore > articles[i].WeightedScore
		})
	case "desc":
		sort.Slice(articles, func(i, j int) bool {
			return articles[j].WeightedScore < articles[i].WeightedScore
		})
	default:
		sort.Slice(articles, func(i, j int) bool {
			return articles[j].WeightedScore > articles[i].WeightedScore
		})
	}

	return articles
}

func giveTopArticles(num int, articles []Article) []Article {
	articles = sortByWeightedScore("desc", articles)
	
	if num > len(articles) {
		return articles
	}

	return articles[:num]
}

func filterOldArticles(articles []Article, years, month, days int ) []Article {
	cutoff := time.Now().AddDate(-years, -month, -days)
	filtered := []Article{}
	for _, article := range articles {
		if article.Date != "" {
			articleDate, err := time.Parse(time.RFC3339, article.Date)
			if err != nil {
				continue
			}
			if articleDate.After(cutoff) {
				filtered = append(filtered, article)
			}
		}
	}

	return filtered
}

func limitAuthors(perAuthor int, articles []Article) []Article {
	articles = sortByWeightedScore("desc", articles)

	seen := map[string]int{}
	limited := []Article{}
	for _, article := range articles {
		if len(article.Authors) == 0 {
			limited = append(limited, article)
			continue
		}
		key := strings.ToLower(article.Authors[0].Name)
		if seen[key] < perAuthor {
			seen[key]++
			limited = append(limited, article)
		}
	}

	return limited
} 