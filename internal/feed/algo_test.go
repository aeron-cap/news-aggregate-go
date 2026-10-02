package feed

import (
	"testing"

	"github.com/aeron-cap/news-aggregator/internal/database"
)

func TestRelevanceOnlyUsesActiveInterests(t *testing.T) {
	ic := NewInterestConfig([]database.Interest{
		{Keyword: "sports", Weight: 1, IsActive: false},
		{Keyword: "science", Weight: 0.5, IsActive: true},
		{Keyword: "technology", Weight: 1, IsActive: false, IsMain: true},
	})
	article := Article{
		Title:   "Sports and science",
		Summary: "Science and technology",
		Link:    "https://example.com/science/sports",
	}
	if got, want := relevance(article, ic), 3.0; got != want {
		t.Errorf("relevance = %v, want %v", got, want)
	}
}

func TestRelevanceWithoutActiveInterests(t *testing.T) {
	ic := NewInterestConfig([]database.Interest{{Keyword: "science", Weight: 1}})
	if got := relevance(Article{Title: "Science"}, ic); got != 0 {
		t.Errorf("relevance = %v, want 0", got)
	}
}
