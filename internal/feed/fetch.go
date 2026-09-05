package feed

import (
	"fmt"
	"strings"
	// "time"

	"github.com/mmcdole/gofeed"
	"golang.org/x/net/html"
)

type Person struct {
	Name	string
}

type Article struct {
	Title   string
	Content string
	Authors []Person 
	Link    string
	// Date    time.Time
}

func Fetch() ([]Article, error) {
	fp := gofeed.NewParser()
	fp.UserAgent = "desktop:com.example.feedreader:v1.0.0 (by /u/A3ron)"
	feed, err := fp.ParseURL("https://hnrss.org/frontpage?points=100")
	if err != nil {
		fmt.Println("Error fetching feed:", err)
		return nil, err
	}

	articles, err := generateArticles(feed)
	if err != nil {
		fmt.Println("Error generating articles:", err)
		return nil, err
	}

	return articles, nil
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