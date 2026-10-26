package feed

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/aeron-cap/news-aggregator/internal/database"
	"github.com/google/uuid"
	"github.com/mmcdole/gofeed"
	"golang.org/x/net/html"
)

type Person struct {
	Name string
}

type Article struct {
	SourceID       uuid.UUID
	Title          string
	Summary        string
	Authors        []Person
	Link           string
	Date           string
	RelevanceScore float64
	RecencyScore   float64
	WeightedScore  float64
}

type rawSource struct {
	ID  uuid.UUID
	URL string
}

type rawFeed struct {
	SourceID uuid.UUID
	Feed     *gofeed.Feed
}

func CreateFeed(ctx context.Context, store *database.Store, userID uuid.UUID) (bool, error) {
	return createFeed(ctx, store, userID, defaultFeedHTTPClient)
}

func createFeed(ctx context.Context, store *database.Store, userID uuid.UUID, client *http.Client) (bool, error) {
	isRefreshNeeded, err := isRefreshNeeded(ctx, store, userID)
	if err != nil {
		return false, err
	}
	if !isRefreshNeeded {
		return false, nil
	}

	sources, err := store.GetActiveSources(ctx, userID)
	if err != nil {
		return false, fmt.Errorf("failed to get sources: %w", err)
	}
	if len(sources) == 0 {
		return false, nil
	}

	workers := min(len(sources), 8)
	sc := make(chan rawSource)
	feeds := make(chan rawFeed)
	workersDone := make(chan bool)
	done := make(chan bool)
	fetchErrors := make(chan error, len(sources))
	successfulFeeds := 0
	articles := []Article{}

	for i := 0; i < workers; i++ {
		fp := gofeed.NewParser()
		go func() {
			for s := range sc {
				feed, err := fetchSourceFeed(ctx, client, fp, s.URL)
				if err != nil {
					fetchErrors <- fmt.Errorf("fetch source %d: %w", s.ID, err)
					continue
				}
				feeds <- rawFeed{SourceID: s.ID, Feed: feed}
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
			article, err := parseSourceFeedToArticle(f.Feed, f.SourceID)
			if err != nil {
				continue
			}
			articles = append(articles, article...)
			successfulFeeds++
		}
		close(done)
	}()

	for _, source := range sources {
		sc <- rawSource{ID: source.ID, URL: source.URL}
	}
	close(sc)

	<-done

	if err := ctx.Err(); err != nil {
		return false, err
	}

	if successfulFeeds == 0 {
		close(fetchErrors)
		var failures []error
		for err := range fetchErrors {
			failures = append(failures, err)
		}
		if len(failures) > 0 {
			return false, fmt.Errorf("no sources could be fetched: %w", errors.Join(failures...))
		}
	}

	err = BuildFeed(ctx, store, userID, articles)
	if err != nil {
		return false, err
	}

	return true, nil
}

func parseSourceFeedToArticle(feed *gofeed.Feed, sourceID uuid.UUID) ([]Article, error) {
	if feed == nil {
		return nil, fmt.Errorf("feed is nil")
	}

	articles := []Article{}
	for _, item := range feed.Items {
		article := Article{
			SourceID: sourceID,
			Title:    getTitle(item),
			Summary:  getSummary(item),
			Authors:  getAuthors(item),
			Link:     getLink(item),
			Date:     getDate(item),
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

func isRefreshNeeded(ctx context.Context, store *database.Store, userID uuid.UUID) (bool, error) {
	unread, err := store.CountUnreadArticles(ctx, userID)
	if err != nil {
		return false, fmt.Errorf("failed to count unread articles: %w", err)
	}

	lastFetch, err := store.GetLastFetchDate(ctx, userID)
	if err != nil {
		return false, fmt.Errorf("failed to get last fetch: %w", err)
	}

	needBuffer := unread < 10
	isStale := time.Since(lastFetch) > 12*time.Hour

	return needBuffer || isStale, nil
}
