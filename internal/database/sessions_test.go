package database

import (
	"context"
	"testing"
	"time"

	"github.com/aeron-cap/news-aggregator/internal/testutil"
	"github.com/google/uuid"
)

func TestExtendSession(t *testing.T) {
	day := 24 * time.Hour
	for _, tc := range []struct {
		name         string
		expiresIn    time.Duration
		wantExtended bool
	}{
		{name: "extends a session past its refresh point", expiresIn: 13 * day, wantExtended: true},
		{name: "leaves a recently extended session alone", expiresIn: 15*day - time.Hour},
		{name: "never revives an expired session", expiresIn: -time.Minute},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store, db := schemaTestStore(t)
			ctx := context.Background()
			now := time.Now().UTC().Truncate(time.Microsecond)
			original := now.Add(tc.expiresIn)
			if err := store.CreateAnonymousSession(ctx, "session-hash", original); err != nil {
				t.Fatal(err)
			}
			if err := store.CreateAnonymousSession(ctx, "other-hash", now.Add(day)); err != nil {
				t.Fatal(err)
			}

			newExpiry := now.Add(15 * day)
			extended, err := store.ExtendSession(ctx, "session-hash", newExpiry, now.Add(14*day))
			if err != nil {
				t.Fatal(err)
			}
			if extended != tc.wantExtended {
				t.Fatalf("extended = %v, want %v", extended, tc.wantExtended)
			}

			want := original
			if tc.wantExtended {
				want = newExpiry
			}
			var got, other time.Time
			if err := db.QueryRow(`SELECT expires_at FROM users WHERE token = 'session-hash'`).Scan(&got); err != nil {
				t.Fatal(err)
			}
			if !got.Equal(want) {
				t.Fatalf("expires_at = %v, want %v", got, want)
			}
			if err := db.QueryRow(`SELECT expires_at FROM users WHERE token = 'other-hash'`).Scan(&other); err != nil || !other.Equal(now.Add(day)) {
				t.Fatalf("other session changed: %v (err=%v)", other, err)
			}
		})
	}
}

func TestFetchSessionReturnsExpiry(t *testing.T) {
	store, _ := schemaTestStore(t)
	ctx := context.Background()
	expiresAt := time.Now().UTC().Truncate(time.Microsecond).Add(time.Hour)
	if err := store.CreateAnonymousSession(ctx, "session-hash", expiresAt); err != nil {
		t.Fatal(err)
	}
	userID, got, err := store.FetchSession(ctx, "session-hash")
	if err != nil || userID == uuid.Nil || !got.Equal(expiresAt) {
		t.Fatalf("FetchSession = (%s, %v, %v), want a user and expiry %v", userID, got, err, expiresAt)
	}
}

func TestDeleteExpiredSessionsCascades(t *testing.T) {
	store, db := schemaTestStore(t)
	now := time.Now().UTC()
	if _, err := db.Exec(`INSERT INTO users (id, token, expires_at) VALUES ($1, 'expired-hash', $2), ($3, 'active-hash', $4)`,
		testutil.ID(1), now.Add(-time.Minute), testutil.ID(2), now.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`
		INSERT INTO sources (id, name, url) VALUES ('00000000-0000-4000-8000-000000000001', 'Science', 'https://science.example/rss');
		INSERT INTO interests (id, keyword) VALUES ('00000000-0000-4000-8000-000000000001', 'science');
		INSERT INTO articles (id, source_id, title, url, batch_date) VALUES ('00000000-0000-4000-8000-000000000001', '00000000-0000-4000-8000-000000000001', 'Story', 'https://science.example/story', CURRENT_TIMESTAMP);
		INSERT INTO user_sources (user_id, source_id) SELECT id, '00000000-0000-4000-8000-000000000001'::uuid FROM users;
		INSERT INTO user_interests (user_id, interest_id) SELECT id, '00000000-0000-4000-8000-000000000001'::uuid FROM users;
		INSERT INTO user_articles (user_id, article_id) SELECT id, '00000000-0000-4000-8000-000000000001'::uuid FROM users;
	`); err != nil {
		t.Fatal(err)
	}

	deleted, err := store.DeleteExpiredSessions(context.Background(), now)
	if err != nil {
		t.Fatal(err)
	}
	if deleted != 1 {
		t.Fatalf("deleted = %d, want 1", deleted)
	}
	for _, table := range []string{"users", "user_sources", "user_interests", "user_articles"} {
		column := "user_id"
		if table == "users" {
			column = "id"
		}
		var expired, active int
		if err := db.QueryRow(`SELECT COUNT(*) FILTER (WHERE `+column+` = $1), COUNT(*) FILTER (WHERE `+column+` = $2) FROM `+table, testutil.ID(1), testutil.ID(2)).Scan(&expired, &active); err != nil {
			t.Fatal(err)
		}
		if expired != 0 || active != 1 {
			t.Fatalf("%s: expired rows=%d active rows=%d, want 0 and 1", table, expired, active)
		}
	}
	var shared int
	if err := db.QueryRow(`SELECT (SELECT COUNT(*) FROM sources) + (SELECT COUNT(*) FROM interests) + (SELECT COUNT(*) FROM articles)`).Scan(&shared); err != nil || shared != 3 {
		t.Fatalf("shared catalog rows = %d, want 3 (err=%v)", shared, err)
	}

	deleted, err = store.DeleteExpiredSessions(context.Background(), now)
	if err != nil || deleted != 0 {
		t.Fatalf("second run deleted=%d err=%v, want 0 and nil", deleted, err)
	}
}
