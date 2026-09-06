package feed

import "sort"

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
