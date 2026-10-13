package feed

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

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

func feedTestStore(t *testing.T) (*database.Store, *sql.DB) {
	t.Helper()
	schema, err := os.ReadFile(filepath.Join("..", "database", "schema.sql"))
	if err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(`PRAGMA foreign_keys=ON`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(string(schema)); err != nil {
		t.Fatal(err)
	}
	store := database.NewStore(db)
	for _, token := range []string{"reader-1", "reader-2"} {
		if err := store.CreateAnonymousSession(context.Background(), token, time.Now().Add(time.Hour)); err != nil {
			t.Fatal(err)
		}
	}
	for _, userID := range []int64{1, 2} {
		if err := store.InsertSources(context.Background(), userID, []database.SourcesUpdate{{Name: "Shared", URL: "https://example.com/rss", IsActive: true}}); err != nil {
			t.Fatal(err)
		}
	}
	return store, db
}

func TestPersonalFeedRankingAndReadIsolation(t *testing.T) {
	store, db := feedTestStore(t)
	ctx := context.Background()
	if _, err := db.Exec(`INSERT INTO interests(id,keyword) VALUES(1,'science'),(2,'technology')`); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ userID, interestID int64 }{{1, 1}, {2, 2}} {
		if err := store.UpdateInterests(ctx, tc.userID, map[int64]database.InterestUpdate{tc.interestID: {IsActive: true, IsMain: true}}); err != nil {
			t.Fatal(err)
		}
	}
	now := time.Now().UTC().Format(time.RFC3339)
	if err := BuildFeed(ctx, store, 1, []Article{
		{SourceID: 1, Title: "Science discovery", Link: "https://example.com/science", Date: now},
		{SourceID: 1, Title: "Technology launch", Link: "https://example.com/technology", Date: now},
	}); err != nil {
		t.Fatal(err)
	}
	first, err := GetFeed(ctx, store, 1)
	if err != nil || len(first) != 2 || first[0].Title != "Science discovery" {
		t.Fatalf("first feed=%+v err=%v", first, err)
	}
	second, err := GetFeed(ctx, store, 2)
	if err != nil || len(second) != 2 || second[0].Title != "Technology launch" {
		t.Fatalf("second feed=%+v err=%v", second, err)
	}
	if err := store.MarkArticleAsRead(ctx, 1, first[0].ID); err != nil {
		t.Fatal(err)
	}
	first, err = GetFeed(ctx, store, 1)
	if err != nil || len(first) != 1 || first[0].Title != "Technology launch" {
		t.Fatalf("read first feed=%+v err=%v", first, err)
	}
	second, err = GetFeed(ctx, store, 2)
	if err != nil || len(second) != 2 {
		t.Fatalf("reading affected other user: %+v err=%v", second, err)
	}
}

func TestBuildFeedKeepsCandidatesAndDeduplicatesCopies(t *testing.T) {
	store, db := feedTestStore(t)
	ctx := context.Background()
	if err := store.InsertSources(ctx, 1, []database.SourcesUpdate{{Name: "Other", URL: "https://other.example/rss", IsActive: true}}); err != nil {
		t.Fatal(err)
	}
	articles := []Article{}
	for i := 0; i < 12; i++ {
		articles = append(articles, Article{SourceID: 1, Title: fmt.Sprintf("Story %d", i), Link: fmt.Sprintf("https://example.com/%d", i), Date: time.Now().UTC().Format(time.RFC3339)})
	}
	duplicate := articles[0]
	if err := db.QueryRow(`SELECT id FROM sources WHERE url = 'https://other.example/rss'`).Scan(&duplicate.SourceID); err != nil {
		t.Fatal(err)
	}
	articles = append(articles, duplicate)
	if err := BuildFeed(ctx, store, 1, articles); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM articles`).Scan(&count); err != nil || count != 13 {
		t.Fatalf("candidate count=%d err=%v", count, err)
	}
	items, err := GetFeed(ctx, store, 1)
	if err != nil || len(items) != 10 {
		t.Fatalf("feed=%+v err=%v", items, err)
	}
	seen := map[string]bool{}
	for _, item := range items {
		if seen[item.URL] {
			t.Fatal("feed contains duplicate URL")
		}
		seen[item.URL] = true
	}
	if err := store.MarkArticleAsRead(ctx, 1, 1); err != nil {
		t.Fatal(err)
	}
	items, err = store.GetUnreadArticles(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range items {
		if item.URL == duplicate.Link {
			t.Fatal("another source copy resurrected a read article")
		}
	}
}

func TestCreateFeedFetchesOnlySelectedReaderSources(t *testing.T) {
	store, db := feedTestStore(t)
	if _, err := db.Exec(`UPDATE user_sources SET is_active=0`); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		title := "Science story"
		if r.URL.Path == "/technology" {
			title = "Technology story"
		}
		w.Header().Set("Content-Type", "application/rss+xml")
		fmt.Fprintf(w, `<?xml version="1.0"?><rss version="2.0"><channel><title>Test feed</title><link>https://example.com</link><description>Test</description><item><title>%s</title><link>https://example.com%s</link><pubDate>%s</pubDate></item></channel></rss>`, title, r.URL.Path, time.Now().UTC().Format(time.RFC1123Z))
	}))
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	for _, tc := range []struct {
		userID int64
		path   string
	}{{1, "/science"}, {2, "/technology"}} {
		if err := store.InsertSources(ctx, tc.userID, []database.SourcesUpdate{{Name: tc.path, URL: server.URL + tc.path, IsActive: true}}); err != nil {
			t.Fatal(err)
		}
	}
	if refreshed, err := CreateFeed(ctx, store, 1); err != nil || !refreshed {
		t.Fatalf("reader 1 refresh=%v err=%v", refreshed, err)
	}
	items, err := GetFeed(ctx, store, 2)
	if err != nil || len(items) != 0 {
		t.Fatalf("reader 2 saw another reader's source: %+v err=%v", items, err)
	}
	if refreshed, err := CreateFeed(ctx, store, 2); err != nil || !refreshed {
		t.Fatalf("reader 2 refresh=%v err=%v", refreshed, err)
	}
	for _, tc := range []struct {
		userID int64
		title  string
	}{{1, "Science story"}, {2, "Technology story"}} {
		items, err := GetFeed(ctx, store, tc.userID)
		if err != nil || len(items) != 1 || items[0].Title != tc.title {
			t.Fatalf("reader %d feed=%+v err=%v", tc.userID, items, err)
		}
	}
}

func TestRelevanceWithoutActiveInterests(t *testing.T) {
	ic := NewInterestConfig([]database.Interest{{Keyword: "science", Weight: 1}})
	if got := relevance(Article{Title: "Science"}, ic); got != 0 {
		t.Errorf("relevance = %v, want 0", got)
	}
}
