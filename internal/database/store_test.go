package database

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"
)

func schemaTestStore(t *testing.T) (*Store, *sql.DB) {
	t.Helper()
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(`PRAGMA foreign_keys = ON`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(schemaSQL); err != nil {
		t.Fatalf("load schema.sql: %v", err)
	}
	return NewStore(db), db
}

func seedTestUsers(t *testing.T, store *Store) {
	t.Helper()
	for _, token := range []string{"user-one-hash", "user-two-hash"} {
		if err := store.CreateAnonymousSession(context.Background(), token, time.Now().UTC().Add(time.Hour)); err != nil {
			t.Fatal(err)
		}
	}
}

// These interfaces describe the user-scoped API needed by the new schema.
// Runtime assertions let the tests compile while the store migration is pending.
type userInterestStore interface {
	GetInterests(context.Context, int64) ([]Interest, error)
	UpdateInterests(context.Context, int64, map[int64]InterestUpdate) error
}

type userSourceStore interface {
	GetActiveSources(context.Context, int64) ([]Source, error)
	GetAllSources(context.Context, int64) ([]Source, error)
	InsertSources(context.Context, int64, []SourcesUpdate) error
	UpdateSources(context.Context, int64, map[int64]SourcesUpdate) error
}

func interestStoreForUserTests(t *testing.T, store *Store) userInterestStore {
	t.Helper()
	scoped, ok := any(store).(userInterestStore)
	if !ok {
		t.Fatal("Store.GetInterests and Store.UpdateInterests must accept userID after ctx")
	}
	return scoped
}

func TestSchemaSeedData(t *testing.T) {
	_, db := schemaTestStore(t)
	if _, err := db.Exec(seed); err != nil {
		t.Fatalf("load seed.sql: %v", err)
	}
	var sources, interests, unseeded int
	if err := db.QueryRow(`SELECT COUNT(*), COALESCE(SUM(is_seeded = 0), 0) FROM sources`).Scan(&sources, &unseeded); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM interests`).Scan(&interests); err != nil {
		t.Fatal(err)
	}
	if sources == 0 || interests == 0 || unseeded != 0 {
		t.Fatalf("seeded catalog: sources=%d interests=%d unseeded=%d", sources, interests, unseeded)
	}
	if _, err := db.Exec(seed); err != nil {
		t.Fatalf("repeat seed.sql: %v", err)
	}
	for _, tc := range []struct {
		table string
		want  int
	}{
		{"sources", sources}, {"interests", interests}, {"users", 0},
		{"user_sources", 0}, {"user_interests", 0}, {"user_articles", 0},
	} {
		var count int
		if err := db.QueryRow(`SELECT COUNT(*) FROM ` + tc.table).Scan(&count); err != nil || count != tc.want {
			t.Errorf("%s count=%d, want %d; err=%v", tc.table, count, tc.want, err)
		}
	}
}

func TestSchemaArticleCopiesAndPersonalReadState(t *testing.T) {
	store, db := schemaTestStore(t)
	seedTestUsers(t, store)
	if _, err := db.Exec(`
		INSERT INTO sources (id, name, url) VALUES (1, 'Feed A', 'https://a.example/rss'), (2, 'Feed B', 'https://b.example/rss');
		INSERT INTO articles (id, source_id, title, url, batch_date)
		VALUES (1, 1, 'Article', 'https://example.com/article', CURRENT_TIMESTAMP),
		       (2, 2, 'Article', 'https://example.com/article', CURRENT_TIMESTAMP);
		INSERT INTO user_articles (user_id, article_id, read_at, weighted_score)
		VALUES (1, 1, CURRENT_TIMESTAMP, 1), (2, 1, NULL, 0.4)
	`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO articles (source_id, title, url, batch_date) VALUES (1, 'Duplicate', 'https://example.com/article', CURRENT_TIMESTAMP)`); err == nil {
		t.Fatal("same-source duplicate article accepted")
	}
	var read bool
	var score float64
	if err := db.QueryRow(`SELECT read_at IS NOT NULL, weighted_score FROM user_articles WHERE user_id = 2 AND article_id = 1`).Scan(&read, &score); err != nil || read || score != 0.4 {
		t.Fatalf("user 2 article state: read=%v score=%v err=%v", read, score, err)
	}
	if _, err := db.Exec(`DELETE FROM users WHERE id = 1`); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM user_articles WHERE user_id = 1`).Scan(&count); err != nil || count != 0 {
		t.Fatalf("deleted user's article state remains: count=%d err=%v", count, err)
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM user_articles WHERE user_id = 2`).Scan(&count); err != nil || count != 1 {
		t.Fatalf("other user's article state changed: count=%d err=%v", count, err)
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM articles`).Scan(&count); err != nil || count != 2 {
		t.Fatalf("user deletion removed collected articles: count=%d err=%v", count, err)
	}
}

func TestUpdateInterests(t *testing.T) {
	for _, tc := range []struct {
		name    string
		updates map[int64]InterestUpdate
		missing bool
	}{
		{
			name: "updates both flags independently",
			updates: map[int64]InterestUpdate{
				1: {IsActive: true, IsMain: true},
				2: {IsActive: false, IsMain: false},
			},
		},
		{
			name: "rolls back when an ID does not exist",
			updates: map[int64]InterestUpdate{
				1:   {IsActive: true, IsMain: true},
				2:   {IsActive: false, IsMain: false},
				999: {IsActive: true, IsMain: false},
			},
			missing: true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			base, db := schemaTestStore(t)
			seedTestUsers(t, base)
			if _, err := db.Exec(`
				INSERT INTO interests (id, keyword) VALUES (1, 'science'), (2, 'sports');
				INSERT INTO user_interests (user_id, interest_id, is_active, is_main)
				VALUES (1, 1, 0, 0), (1, 2, 1, 1), (2, 1, 1, 0), (2, 2, 0, 1)
			`); err != nil {
				t.Fatal(err)
			}
			store := interestStoreForUserTests(t, base)
			err := store.UpdateInterests(context.Background(), 1, tc.updates)
			if tc.missing {
				if !errors.Is(err, sql.ErrNoRows) {
					t.Fatalf("expected missing interest error, got %v", err)
				}
			} else if err != nil {
				t.Fatal(err)
			}

			interests, err := store.GetInterests(context.Background(), 1)
			if err != nil {
				t.Fatal(err)
			}
			if len(interests) != 2 {
				t.Fatalf("user 1 interests=%+v, want both configured interests", interests)
			}
			for _, interest := range interests {
				want := tc.updates[interest.ID]
				if tc.missing {
					want = InterestUpdate{IsActive: interest.ID == 2, IsMain: interest.ID == 2}
				}
				if interest.IsActive != want.IsActive || interest.IsMain != want.IsMain {
					t.Errorf("interest %d: got active=%v main=%v, want %+v", interest.ID, interest.IsActive, interest.IsMain, want)
				}
			}
			var active, main bool
			for id := int64(1); id <= 2; id++ {
				if err := db.QueryRow(`SELECT is_active, is_main FROM user_interests WHERE user_id = 2 AND interest_id = ?`, id).Scan(&active, &main); err != nil {
					t.Fatal(err)
				}
				if active != (id == 1) || main != (id == 2) {
					t.Fatalf("user 2 interest %d changed: active=%v main=%v", id, active, main)
				}
			}
		})
	}
}

func sourceTestStore(t *testing.T) (userSourceStore, *sql.DB) {
	t.Helper()
	store, db := schemaTestStore(t)
	seedTestUsers(t, store)
	scoped, ok := any(store).(userSourceStore)
	if !ok {
		t.Fatal("Store source methods must accept userID after ctx")
	}
	return scoped, db
}

func TestGetSourcesOnlyActive(t *testing.T) {
	store, db := sourceTestStore(t)
	if _, err := db.Exec(`
		INSERT INTO sources (id, name, url, is_seeded) VALUES (1, 'Science', 'https://science.example/rss', 1), (2, 'Sports', 'https://sports.example/rss', 1);
		INSERT INTO user_sources (user_id, source_id, is_active) VALUES (1, 1, 1), (1, 2, 0), (2, 1, 0), (2, 2, 1)
	`); err != nil {
		t.Fatal(err)
	}
	sources, err := store.GetActiveSources(context.Background(), 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(sources) != 1 || sources[0].Name != "Science" || !sources[0].IsActive {
		t.Fatalf("feed sources include inactive rows: %+v", sources)
	}
	other, err := store.GetActiveSources(context.Background(), 2)
	if err != nil || len(other) != 1 || other[0].Name != "Sports" {
		t.Fatalf("user 2 sources=%+v err=%v, want only Sports", other, err)
	}
}

func TestGetAllSourcesIncludesSeededAndSubscribedSources(t *testing.T) {
	store, db := sourceTestStore(t)
	if _, err := db.Exec(`
		INSERT INTO sources (id, name, url, is_seeded)
		VALUES (1, 'Default', 'https://default.example/rss', 1),
		       (2, 'Personal A', 'https://a.example/rss', 0),
		       (3, 'Personal B', 'https://b.example/rss', 0);
		INSERT INTO user_sources (user_id, source_id, is_active)
		VALUES (1, 1, 0), (1, 2, 1), (2, 1, 1), (2, 3, 1)
	`); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		userID int64
		ownID  int64
		active bool
	}{{1, 2, false}, {2, 3, true}} {
		sources, err := store.GetAllSources(context.Background(), tc.userID)
		if err != nil || len(sources) != 2 {
			t.Fatalf("user %d sources=%+v err=%v, want default plus own source", tc.userID, sources, err)
		}
		found := make(map[int64]Source)
		for _, source := range sources {
			found[source.ID] = source
		}
		defaultSource, defaultFound := found[1]
		ownSource, ownFound := found[tc.ownID]
		if !defaultFound || !ownFound || defaultSource.IsActive != tc.active || !ownSource.IsActive {
			t.Fatalf("user %d has incorrect source visibility or status: %+v", tc.userID, sources)
		}
	}
}

func TestInsertSourcesReusesSharedFeedForAnotherUser(t *testing.T) {
	store, db := sourceTestStore(t)
	source := SourcesUpdate{Name: "Science", URL: "https://science.example/rss", IsActive: true}
	if err := store.InsertSources(context.Background(), 1, []SourcesUpdate{source}); err != nil {
		t.Fatal(err)
	}
	source.IsActive = false
	if err := store.InsertSources(context.Background(), 2, []SourcesUpdate{source}); err != nil {
		t.Fatalf("subscribe another user to an existing feed: %v", err)
	}
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM sources`).Scan(&count); err != nil || count != 1 {
		t.Fatalf("shared source count=%d, want 1; err=%v", count, err)
	}
	for userID := int64(1); userID <= 2; userID++ {
		var active bool
		if err := db.QueryRow(`SELECT is_active FROM user_sources WHERE user_id = ?`, userID).Scan(&active); err != nil || active != (userID == 1) {
			t.Fatalf("user %d subscription active=%v err=%v", userID, active, err)
		}
	}
}

func TestInsertSourcesAtomic(t *testing.T) {
	for _, duplicate := range []bool{false, true} {
		t.Run(map[bool]string{false: "success", true: "duplicate rolls back entire batch"}[duplicate], func(t *testing.T) {
			store, db := sourceTestStore(t)
			sources := []SourcesUpdate{
				{Name: "Science", URL: "https://science.example/rss", IsActive: true},
				{Name: "Sports", URL: "https://sports.example/rss", IsActive: false},
			}
			if duplicate {
				sources[1].URL = sources[0].URL
			}
			err := store.InsertSources(context.Background(), 1, sources)
			if duplicate && err == nil {
				t.Fatal("duplicate URL should fail")
			}
			if !duplicate && err != nil {
				t.Fatal(err)
			}
			var count int
			if err := db.QueryRow(`SELECT COUNT(*) FROM sources`).Scan(&count); err != nil {
				t.Fatal(err)
			}
			want := 2
			if duplicate {
				want = 0
			}
			if count != want {
				t.Fatalf("source count=%d, want %d", count, want)
			}
			var subscriptions int
			if err := db.QueryRow(`SELECT COUNT(*) FROM user_sources WHERE user_id = 1`).Scan(&subscriptions); err != nil || subscriptions != want {
				t.Fatalf("subscription count=%d, want %d; err=%v", subscriptions, want, err)
			}
			if !duplicate {
				var active bool
				if err := db.QueryRow(`SELECT us.is_active FROM user_sources us JOIN sources s ON s.id = us.source_id WHERE us.user_id = 1 AND s.name = 'Sports'`).Scan(&active); err != nil || active {
					t.Fatalf("inactive source: active=%v err=%v", active, err)
				}
				var seeded bool
				if err := db.QueryRow(`SELECT is_seeded FROM sources WHERE name = 'Sports'`).Scan(&seeded); err != nil || seeded {
					t.Fatalf("personal source marked seeded: seeded=%v err=%v", seeded, err)
				}
			}
		})
	}
}

func TestUpdateSourcesAtomic(t *testing.T) {
	for _, mode := range []string{"success", "missing", "duplicate"} {
		t.Run(mode, func(t *testing.T) {
			store, db := sourceTestStore(t)
			if _, err := db.Exec(`
				INSERT INTO sources (id, name, url) VALUES (1, 'Science', 'https://science.example/rss'), (2, 'Sports', 'https://sports.example/rss');
				INSERT INTO user_sources (user_id, source_id, is_active) VALUES (1, 1, 1), (1, 2, 0), (2, 1, 1), (2, 2, 0)
			`); err != nil {
				t.Fatal(err)
			}
			updates := map[int64]SourcesUpdate{
				1: {Name: "New Science", URL: "https://new-science.example/rss", IsActive: false},
				2: {Name: "New Sports", URL: "https://new-sports.example/rss", IsActive: true},
			}
			if mode == "missing" {
				updates[999] = SourcesUpdate{Name: "Missing", URL: "https://missing.example/rss"}
			}
			if mode == "duplicate" {
				updates[2] = updates[1]
			}
			err := store.UpdateSources(context.Background(), 1, updates)
			if mode == "success" && err != nil {
				t.Fatal(err)
			}
			if mode != "success" && err == nil {
				t.Fatal("expected update error")
			}
			if mode == "missing" && !errors.Is(err, sql.ErrNoRows) {
				t.Fatalf("expected sql.ErrNoRows, got %v", err)
			}
			for id := int64(1); id <= 2; id++ {
				var got SourcesUpdate
				if err := db.QueryRow(`SELECT s.name, s.url, us.is_active FROM sources s JOIN user_sources us ON us.source_id = s.id WHERE us.user_id = 1 AND s.id = ?`, id).Scan(&got.Name, &got.URL, &got.IsActive); err != nil {
					t.Fatal(err)
				}
				want := updates[id]
				if mode != "success" {
					want = SourcesUpdate{Name: "Science", URL: "https://science.example/rss", IsActive: true}
					if id == 2 {
						want = SourcesUpdate{Name: "Sports", URL: "https://sports.example/rss", IsActive: false}
					}
				}
				if got != want {
					t.Errorf("source %d: got %+v, want %+v", id, got, want)
				}
				var otherActive bool
				if err := db.QueryRow(`SELECT is_active FROM user_sources WHERE user_id = 2 AND source_id = ?`, id).Scan(&otherActive); err != nil || otherActive != (id == 1) {
					t.Fatalf("user 2 source %d changed: active=%v err=%v", id, otherActive, err)
				}
			}
		})
	}
}

func TestUpdateToken(t *testing.T) {
	for _, tc := range []struct {
		name       string
		oldToken   string
		newToken   string
		expired    bool
		pastExpiry bool
		wantError  bool
		wantNoRows bool
	}{
		{name: "preserves user ID and invalidates old token", oldToken: "old", newToken: "new"},
		{name: "rejects unknown token", oldToken: "missing", newToken: "new", wantError: true, wantNoRows: true},
		{name: "rejects expired token", oldToken: "old", newToken: "new", expired: true, wantError: true, wantNoRows: true},
		{name: "rejects token belonging to another user", oldToken: "old", newToken: "other", wantError: true},
		{name: "rejects expiry in the past", oldToken: "old", newToken: "new", pastExpiry: true, wantError: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store, db := schemaTestStore(t)
			ctx := context.Background()
			originalExpiry := time.Now().UTC().Add(time.Hour)
			if tc.expired {
				originalExpiry = time.Now().UTC().Add(-time.Hour)
			}
			if err := store.CreateAnonymousSession(ctx, "old", originalExpiry); err != nil {
				t.Fatal(err)
			}
			if err := store.CreateAnonymousSession(ctx, "other", time.Now().UTC().Add(time.Hour)); err != nil {
				t.Fatal(err)
			}
			newExpiry := time.Now().UTC().Add(24 * time.Hour)
			if tc.pastExpiry {
				newExpiry = time.Now().UTC().Add(-time.Hour)
			}

			err := store.UpdateToken(ctx, tc.oldToken, tc.newToken, newExpiry)
			if (err != nil) != tc.wantError {
				t.Fatalf("error = %v, wantError = %v", err, tc.wantError)
			}
			if tc.wantNoRows && !errors.Is(err, sql.ErrNoRows) {
				t.Fatalf("error = %v, want sql.ErrNoRows", err)
			}

			var token string
			var expiry time.Time
			if err := db.QueryRow(`SELECT token, expires_at FROM users WHERE id = 1`).Scan(&token, &expiry); err != nil {
				t.Fatal(err)
			}
			wantToken, wantExpiry := "old", originalExpiry
			if !tc.wantError {
				wantToken, wantExpiry = "new", newExpiry
				if _, err := store.FetchUserID(ctx, "old"); !errors.Is(err, sql.ErrNoRows) {
					t.Fatalf("old token lookup error = %v, want sql.ErrNoRows", err)
				}
				if id, err := store.FetchUserID(ctx, "new"); err != nil || id != 1 {
					t.Fatalf("new token lookup = (%d, %v), want (1, nil)", id, err)
				}
			}
			if token != wantToken || !expiry.Equal(wantExpiry) {
				t.Fatalf("session = (%q, %v), want (%q, %v)", token, expiry, wantToken, wantExpiry)
			}
			if id, err := store.FetchUserID(ctx, "other"); err != nil || id != 2 {
				t.Fatalf("other session = (%d, %v), want (2, nil)", id, err)
			}
			var count int
			if err := db.QueryRow(`SELECT COUNT(*) FROM users`).Scan(&count); err != nil || count != 2 {
				t.Fatalf("user count = (%d, %v), want (2, nil)", count, err)
			}
		})
	}
}
