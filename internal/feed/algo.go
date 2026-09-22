package feed

import (
	"math"
	"regexp"
	"strings"
	"time"

	"github.com/aeron-cap/news-aggregator/internal/database"
)

type InterestConfig struct {
	keywords []database.Interest
	patterns []*regexp.Regexp
	weights  map[string]float64
}

const (
	titleWeight   = 3.0
	contentWeight = 2.0
	urlWeight     = 1.0
	halfLifeHours = 148.0
	relevanceW    = 0.5
	recencyW      = 0.5
)

func NewInterestConfig(keywords []database.Interest) InterestConfig {
	patterns := make([]*regexp.Regexp, len(keywords))
	weights := make(map[string]float64, len(keywords))

	for i, k := range keywords {
		patterns[i] = regexp.MustCompile(`(?i)\b` + regexp.QuoteMeta(k.Keyword) + `\b`)
		weights[k.Keyword] = k.Weight
	}

	return InterestConfig{
		keywords: keywords,
		patterns: patterns,
		weights:  weights,
	}
}

func relevance(article Article, ic InterestConfig) float64 {
	title := strings.ToLower(article.Title)
	content := strings.ToLower(article.Summary)
	url := strings.ToLower(article.Link)

	totalRelevance := 0.0
	for i, kw := range ic.keywords {
		re := ic.patterns[i]
		weight := ic.weights[kw.Keyword]

		if re.MatchString(title) {
			totalRelevance += titleWeight * weight * float64(len(re.FindAllStringIndex(title, -1)))
		}

		if re.MatchString(content) {
			totalRelevance += contentWeight * weight * float64(len(re.FindAllStringIndex(content, -1)))
		}

		if re.MatchString(url) {
			totalRelevance += urlWeight * weight * float64(len(re.FindAllStringIndex(url, -1)))
		}
	}

	return totalRelevance
}

func recency(article Article, currentTime time.Time) float64 {
	date, err := time.Parse(time.RFC3339, article.Date)
	if err != nil {
		return 0.0
	}
	ageHours := currentTime.Sub(date).Hours()
	if ageHours < 0 {
		return 1.0
	}

	return math.Pow(0.5, (ageHours / halfLifeHours))
}

func weightedScore(article Article) float64 {
	return (relevanceW * article.RelevanceScore) + (recencyW * article.RecencyScore)
}
