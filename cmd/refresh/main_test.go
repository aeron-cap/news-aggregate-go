package main

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/aeron-cap/news-aggregator/internal/database"
	"github.com/aeron-cap/news-aggregator/internal/feed"
	"github.com/aeron-cap/news-aggregator/internal/testutil"
	"github.com/google/uuid"
)

type fakeUsers struct {
	ids []uuid.UUID
	err error
}

func (f fakeUsers) ListActiveUserIDs(context.Context, time.Time) ([]uuid.UUID, error) {
	return f.ids, f.err
}

var discard = slog.New(slog.NewTextHandler(io.Discard, nil))

func TestRunKeepsGoingAfterFailures(t *testing.T) {
	users := fakeUsers{ids: []uuid.UUID{testutil.ID(1), testutil.ID(2), testutil.ID(3)}}
	var called []uuid.UUID
	err := run(context.Background(), discard, users, func(ctx context.Context, userID uuid.UUID) (bool, error) {
		called = append(called, userID)
		if userID == testutil.ID(1) {
			return false, errors.New("source unavailable")
		}
		return userID == testutil.ID(2), nil
	}, time.Second)
	if err != nil {
		t.Fatalf("run() = %v, want nil when only some users fail", err)
	}
	if len(called) != 3 {
		t.Fatalf("refreshed %d users, want 3", len(called))
	}
}

func TestRunFailsWhenEveryUserFails(t *testing.T) {
	users := fakeUsers{ids: []uuid.UUID{testutil.ID(1), testutil.ID(2)}}
	err := run(context.Background(), discard, users, func(context.Context, uuid.UUID) (bool, error) {
		return false, errors.New("network down")
	}, time.Second)
	if !errors.Is(err, errAllFailed) {
		t.Fatalf("run() = %v, want errAllFailed", err)
	}
}

func TestRunWithNoUsers(t *testing.T) {
	err := run(context.Background(), discard, fakeUsers{}, func(context.Context, uuid.UUID) (bool, error) {
		t.Fatal("refresh called without users")
		return false, nil
	}, time.Second)
	if err != nil {
		t.Fatalf("run() = %v, want nil", err)
	}
}

func TestRunListError(t *testing.T) {
	err := run(context.Background(), discard, fakeUsers{err: errors.New("db down")}, nil, time.Second)
	if err == nil {
		t.Fatal("run() = nil, want list error")
	}
}

func TestRunGivesEachUserItsOwnTimeout(t *testing.T) {
	users := fakeUsers{ids: []uuid.UUID{testutil.ID(1), testutil.ID(2)}}
	var refreshed []uuid.UUID
	err := run(context.Background(), discard, users, func(ctx context.Context, userID uuid.UUID) (bool, error) {
		if _, ok := ctx.Deadline(); !ok {
			t.Fatal("per-user context has no deadline")
		}
		if userID == testutil.ID(1) {
			<-ctx.Done()
			return false, ctx.Err()
		}
		if err := ctx.Err(); err != nil {
			t.Fatalf("second user started with a finished context: %v", err)
		}
		refreshed = append(refreshed, userID)
		return true, nil
	}, 20*time.Millisecond)
	if err != nil {
		t.Fatalf("run() = %v, want nil", err)
	}
	if len(refreshed) != 1 || refreshed[0] != testutil.ID(2) {
		t.Fatalf("refreshed = %v, want only the second user", refreshed)
	}
}

func TestRunStopsWhenCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	users := fakeUsers{ids: []uuid.UUID{testutil.ID(1), testutil.ID(2)}}
	calls := 0
	err := run(ctx, discard, users, func(context.Context, uuid.UUID) (bool, error) {
		calls++
		cancel()
		return true, nil
	}, time.Second)
	if !errors.Is(err, context.Canceled) || calls != 1 {
		t.Fatalf("run() = %v after %d calls, want context.Canceled after 1", err, calls)
	}
}

func TestRunRefreshesOnlyActiveUsers(t *testing.T) {
	db, _ := testutil.Postgres(t)
	schema, err := os.ReadFile(filepath.Join("..", "..", "migrations", "001_initial_schema.sql"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(string(schema)); err != nil {
		t.Fatalf("load initial schema: %v", err)
	}
	now := time.Now().UTC()
	if _, err := db.Exec(`INSERT INTO users (id, token, expires_at) VALUES ($1, 'active', $2), ($3, 'expired', $4)`,
		testutil.ID(1), now.Add(time.Hour), testutil.ID(2), now.Add(-time.Hour)); err != nil {
		t.Fatal(err)
	}
	store := database.NewStore(db)
	var refreshed []uuid.UUID
	// No sources exist, so CreateFeed returns without any network calls.
	err = run(context.Background(), discard, store, func(ctx context.Context, userID uuid.UUID) (bool, error) {
		refreshed = append(refreshed, userID)
		return feed.CreateFeed(ctx, store, userID)
	}, time.Second)
	if err != nil {
		t.Fatalf("run() = %v", err)
	}
	if len(refreshed) != 1 || refreshed[0] != testutil.ID(1) {
		t.Fatalf("refreshed = %v, want only the active user", refreshed)
	}
}
