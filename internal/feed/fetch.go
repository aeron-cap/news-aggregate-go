package feed

import (
	"fmt"
	"strings"
	"time"

	"github.com/mmcdole/gofeed"
	"golang.org/x/net/html"
)

type Person struct {
	Name string
}

type Article struct {
	Title   string
	Content string
	Authors []Person
	Link    string
	Date    string
	RelevanceScore float64
	RecencyScore float64
	WeightedScore	float64
}

// XML Feeds for Tech News, Community Aggregators, and Curator Blogs
// Community Aggregators (XML):
// - Lobste.rs (RSS 2.0)
//   https://lobste.rs/rss
// - Hacker News via HNRSS - Top Submissions (RSS 2.0)
//   https://hnrss.org/frontpage
// - Hacker News via HNRSS - Top Submissions (Atom 1.0)
//   https://hnrss.org/frontpage.atom
// - Hacker News via HNRSS - Show HN / Personal Projects (RSS 2.0)
//   https://hnrss.org/show?points=25
// - Hacker News Official (RSS 2.0)
//   https://news.ycombinator.com/rss
// - Reddit r/programming (RSS 2.0 / Atom)
//   https://www.reddit.com/r/programming/.rss
// - Reddit r/golang (RSS 2.0 / Atom)
//   https://www.reddit.com/r/golang/.rss
// - Reddit r/selfhosted (RSS 2.0 / Atom)
//   https://www.reddit.com/r/selfhosted/.rss
// - DEV Community (RSS 2.0)
//   https://dev.to/feed
//
// Curator Blogs & Linklogs (XML):
// - Simon Willison - All Posts (Atom 1.0)
//   https://simonwillison.net/atom/everything/
// - Simon Willison - Links Only (Atom 1.0)
//   https://simonwillison.net/atom/links/
// - Daring Fireball (RSS 2.0)
//   https://daringfireball.net/feeds/main
// - Waxy.org (RSS 2.0)
//   https://waxy.org/feed/
// - Kottke.org (RSS 2.0)
//   https://feeds.kottke.org/main
// - Dan Luu (Atom 1.0)
//   https://danluu.com/atom.xml
// - Eli Bendersky (Atom 1.0)
//   https://eli.thegreenplace.net/feeds/all.atom.xml
// - Brandur Leach (Atom 1.0)
//   https://brandur.org/articles.atom

func Fetch() ([]Article, error) {
	fp := gofeed.NewParser()
	// for reddit
	fp.UserAgent = "desktop:com.example.feedreader:v1.0.0 (by /u/A3ron)"

	feed, err := fp.ParseURL("https://lobste.rs/rss")
	if err != nil {
		fmt.Println("Error fetching feed:", err)
		return nil, err
	}

	articles, err := generateArticles(feed)
	if err != nil {
		fmt.Println("Error generating articles:", err)
		return nil, err
	}

	return BuildFeed(articles), nil 
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
	if feed.UpdatedParsed == nil {
		return feed.PublishedParsed.UTC().Format(time.RFC3339)
	}
	
	return feed.UpdatedParsed.UTC().Format(time.RFC3339)
}