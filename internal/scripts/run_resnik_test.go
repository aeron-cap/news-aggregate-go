package scripts_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/aeron-cap/news-aggregator/internal/database"
	"github.com/aeron-cap/news-aggregator/internal/scripts"
	"github.com/aeron-cap/news-aggregator/internal/testutil"
	"github.com/google/uuid"
)

func TestRunResnikUpdatesOnlyRequestedPostgresUser(t *testing.T) {
	if os.Getenv("PYTHON_BIN") == "" {
		t.Skip("set PYTHON_BIN and install WordNet corpora to test the real scorer")
	}
	_, databaseURL := testutil.Postgres(t)
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	db, err := database.Open(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store := database.NewStore(db)
	readers := []uuid.UUID{}
	for _, token := range []string{"scoring-reader-one", "scoring-reader-two"} {
		if err := store.CreateAnonymousSession(ctx, token, time.Now().Add(time.Hour)); err != nil {
			t.Fatal(err)
		}
		id, err := store.FetchUserID(ctx, token)
		if err != nil {
			t.Fatal(err)
		}
		readers = append(readers, id)
	}
	if _, err := db.ExecContext(ctx, `UPDATE user_interests SET weight = 0.37, is_active = TRUE,
		is_main = TRUE WHERE interest_id = (SELECT id FROM interests WHERE keyword = 'software')`); err != nil {
		t.Fatal(err)
	}
	// The child must receive the exact URL selected by Go, not the parent's URL.
	t.Setenv("DATABASE_URL", "postgresql://unused@127.0.0.1:1/unused?sslmode=disable")
	if err := scripts.RunResnik(ctx, filepath.Join("..", "database", "resnik.py"), readers[0], databaseURL); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		userID uuid.UUID
		want   float64
	}{{readers[0], 1}, {readers[1], 0.37}} {
		var weight float64
		if err := db.QueryRowContext(ctx, `SELECT weight FROM user_interests WHERE user_id = $1
			AND interest_id = (SELECT id FROM interests WHERE keyword = 'software')`, tc.userID).Scan(&weight); err != nil || weight != tc.want {
			t.Fatalf("user %s weight=%v want=%v err=%v", tc.userID, weight, tc.want, err)
		}
	}
}
