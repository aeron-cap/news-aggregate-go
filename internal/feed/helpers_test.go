package feed

import (
	"fmt"
	"math/rand"
	"slices"
	"testing"
	"time"
)

func links(articles []Article) []string {
	out := make([]string, len(articles))
	for i, article := range articles {
		out[i] = article.Link
	}
	return out
}

func TestGiveTopArticlesIgnoresInputOrder(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	now := time.Now().UTC()
	scores := []float64{0, 0.25, 0.5}
	for run := 0; run < 200; run++ {
		n := 1 + rng.Intn(20)
		articles := make([]Article, n)
		for i := range articles {
			articles[i] = Article{
				Link:          fmt.Sprintf("https://example.com/%d", i),
				Date:          now.Add(-time.Duration(rng.Intn(3)) * time.Hour).Format(time.RFC3339),
				WeightedScore: scores[rng.Intn(len(scores))],
			}
		}
		want := links(giveTopArticles(10, slices.Clone(articles)))
		shuffled := slices.Clone(articles)
		rng.Shuffle(len(shuffled), func(i, j int) { shuffled[i], shuffled[j] = shuffled[j], shuffled[i] })
		if got := links(giveTopArticles(10, shuffled)); !slices.Equal(got, want) {
			t.Fatalf("run %d: shuffled input picked %v, want %v", run, got, want)
		}
	}
}

func TestSortByWeightedScoreTieBreaks(t *testing.T) {
	articles := []Article{
		{Link: "b", Date: "2026-01-01T00:00:00Z", WeightedScore: 1},
		{Link: "a", Date: "2026-01-01T00:00:00Z", WeightedScore: 1},
		{Link: "c", Date: "2026-02-01T00:00:00Z", WeightedScore: 1},
		{Link: "d", Date: "2025-01-01T00:00:00Z", WeightedScore: 2},
	}
	got := links(sortByWeightedScore("desc", articles))
	if want := []string{"d", "c", "a", "b"}; !slices.Equal(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}
