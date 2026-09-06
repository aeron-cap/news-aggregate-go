package feed

import (
	"math"
	"regexp"
	"strings"
	"time"
)

type Interests struct {
	keywords []string
	patterns []*regexp.Regexp
}

const (
	titleWeight   = 3.0
	contentWeight = 2.0
	urlWeight     = 1.0
	halfLifeHours = 24.0
	relevanceW    = 0.7
	recencyW      = 0.3
)

func NewInterests(keywords []string) Interests {
	patterns := make([]*regexp.Regexp, len(keywords))
	for i, k := range keywords {
		patterns[i] = regexp.MustCompile(`(?i)\b` + regexp.QuoteMeta(k) + `\b`)
	}
	return Interests{keywords: keywords, patterns: patterns}
}

func relevance(article Article, interests Interests) float64 {
	title := strings.ToLower(article.Title)
	content := strings.ToLower(article.Content)
	url := strings.ToLower(article.Link)

	totalRelevance := 0.0
	for _, re := range interests.patterns {
		if re.MatchString(title) {
			totalRelevance += titleWeight * float64(len(re.FindAllStringIndex(title, -1)))
		}

		if re.MatchString(content) {
			totalRelevance += contentWeight * float64(len(re.FindAllStringIndex(content, -1)))
		}

		if re.MatchString(url) {
			totalRelevance += urlWeight * float64(len(re.FindAllStringIndex(url, -1)))
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
	if (ageHours < 0) {
		return 1.0
	}
	
	return math.Pow(0.5, (ageHours / halfLifeHours))
}

func weightedScore(article Article) float64 {
	return (relevanceW * article.RelevanceScore) + (recencyW * article.RecencyScore)
}