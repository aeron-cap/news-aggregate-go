package feed

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/aeron-cap/news-aggregator/internal/database"
	"github.com/mmcdole/gofeed"
	"golang.org/x/net/html"
)

type Person struct {
	Name string
}

type Article struct {
	Title          string
	Summary        string
	Authors        []Person
	Link           string
	Date           string
	RelevanceScore float64
	RecencyScore   float64
	WeightedScore  float64
}

func CreateFeed(store *database.Store) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	isRefreshNeeded, err := isRefreshNeeded(ctx, store)
	if err != nil {
		return err
	}
	if !isRefreshNeeded {
		return nil
	}

	sources, err := store.GetSources(ctx)
	if err != nil {
		return fmt.Errorf("failed to get sources: %w", err)
	}

	var workers = len(sources) + 1
	sc := make(chan string)
	feeds := make(chan *gofeed.Feed)
	workersDone := make(chan bool)
	done := make(chan bool)
	articles := []Article{}

	for i := 0; i < workers; i++ {
		fp := gofeed.NewParser()
		go func() {
			for s := range sc {
				feed, err := fp.ParseURLWithContext(s, ctx)
				if err != nil {
					continue
				}
				feeds <- feed
			}
			workersDone <- true
		}()
	}

	go func() {
		for i := 0; i < workers; i++ {
			<-workersDone
		}
		close(feeds)
	}()

	go func() {
		for f := range feeds {
			article, err := parseSourceFeedToArticle(f)
			if err != nil {
				continue
			}
			articles = append(articles, article...)
		}
		close(done)
	}()

	for _, source := range sources {
		sc <- source.URL
	}
	close(sc)

	<-done

	err = BuildFeed(ctx, store, articles)
	if err != nil {
		return err
	}

	return nil
}

func parseSourceFeedToArticle(feed *gofeed.Feed) ([]Article, error) {
	if feed == nil {
		return nil, fmt.Errorf("feed is nil")
	}

	articles := []Article{}
	for _, item := range feed.Items {
		article := Article{
			Title:   getTitle(item),
			Summary: getSummary(item),
			Authors: getAuthors(item),
			Link:    getLink(item),
			Date:    getDate(item),
		}

		articles = append(articles, article)
	}

	return articles, nil
}

func getTitle(item *gofeed.Item) string {
	if item.Title != "" {
		return item.Title
	}
	return ""
}

func getSummary(item *gofeed.Item) string {
	raw := item.Content

	z := html.NewTokenizer(strings.NewReader(raw))
	var body strings.Builder
	skipping := false
	for {
		tt := z.Next()
		if tt == html.ErrorToken {
			break
		}
		switch tt {
		case html.StartTagToken, html.EndTagToken:
			name, _ := z.TagName()
			tag := string(name)

			if tag == "script" || tag == "style" {
				skipping = tt == html.StartTagToken
			}

		case html.TextToken:
			if !skipping {
				body.WriteString(string(z.Text()))
			}
		}
	}
	return body.String()
}

func getAuthors(item *gofeed.Item) []Person {
	persons := []Person{}
	authors := item.Authors
	for _, author := range authors {
		persons = append(persons, Person{Name: author.Name})
	}

	return persons
}

func getLink(item *gofeed.Item) string {
	item.Link = strings.TrimSpace(item.Link)
	return item.Link
}

func getDate(item *gofeed.Item) string {
	if item.UpdatedParsed != nil {
		return item.UpdatedParsed.UTC().Format(time.RFC3339)
	}

	if item.PublishedParsed != nil {
		return item.PublishedParsed.UTC().Format(time.RFC3339)
	}

	return time.Now().UTC().Format(time.RFC3339)
}

func isRefreshNeeded(ctx context.Context, store *database.Store) (bool, error) {
	unread, err := store.CountUnreadArticles(ctx)
	if err != nil {
		return false, fmt.Errorf("failed to count unread articles: %w", err)
	}

	lastFetch, err := store.GetLastFetchDate(ctx)
	if err != nil {
		return false, fmt.Errorf("failed to get last fetch: %w", err)
	}

	needBuffer := unread < 10
	isStale := time.Since(lastFetch) > time.Hour

	return needBuffer && isStale, nil
}
