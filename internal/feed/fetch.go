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
	Content        string
	Authors        []Person
	Link           string
	Date           string
	RelevanceScore float64
	RecencyScore   float64
	WeightedScore  float64
}

func Fetch(store *database.Store) ([]Article, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	
	sources, err := store.GetSources(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get sources: %w", err)
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
			feedOutput, err := generateArticles(f)
			if err != nil {
				continue
			}
			articles = append(articles, feedOutput...)
		}
		close(done)
	}()

	for _, source := range sources {
		sc <- source.URL
	}
	close(sc)

	<-done
	return BuildFeed(ctx, store, articles), nil
}

func generateArticles(feed *gofeed.Feed) ([]Article, error) {
	if feed == nil {
		return nil, fmt.Errorf("feed is nil")
	}

	articles := []Article{}
	for _, item := range feed.Items {
		article := Article{
			Title:   getTitle(item),
			Content: getContent(item),
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

func getContent(item *gofeed.Item) string {
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

func getAuthors(feed *gofeed.Item) []Person {
	persons := []Person{}
	authors := feed.Authors
	for _, author := range authors {
		persons = append(persons, Person{Name: author.Name})
	}

	return persons
}

func getLink(feed *gofeed.Item) string {
	feed.Link = strings.TrimSpace(feed.Link)
	return feed.Link
}

func getDate(feed *gofeed.Item) string {
	if feed.UpdatedParsed != nil {
		return feed.UpdatedParsed.UTC().Format(time.RFC3339)
	}

	if feed.PublishedParsed != nil {
		return feed.PublishedParsed.UTC().Format(time.RFC3339)
	}

	return time.Now().UTC().Format(time.RFC3339)
}
