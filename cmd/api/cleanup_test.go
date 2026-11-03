package main

import (
	"context"
	"database/sql"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/aeron-cap/news-aggregator/internal/database"
	"github.com/aeron-cap/news-aggregator/internal/testutil"
)

func TestCleanupExpiredSessionsDeletesOnStart(t *testing.T) {
	a, db := schemaTestApp(t)
	if _, err := db.Exec(`INSERT INTO users (id, token, expires_at) VALUES ($1, 'expired', $2), ($3, 'active', $4)`,
		testutil.ID(1), time.Now().UTC().Add(-time.Minute), testutil.ID(2), time.Now().UTC().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		a.cleanupExpiredSessions(ctx, time.Hour)
		close(done)
	}()

	deadline := time.Now().Add(5 * time.Second)
	for {
		var count int
		if err := db.QueryRow(`SELECT COUNT(*) FROM users`).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count == 1 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("users = %d, want expired session deleted", count)
		}
		time.Sleep(20 * time.Millisecond)
	}
	cancel()
	<-done
}

func TestCleanupExpiredSessionsStopsOnCancel(t *testing.T) {
	db, err := sql.Open("pgx", "postgresql://news_aggregator@127.0.0.1:1/news_aggregator")
	if err != nil {
		t.Fatal(err)
	}
	db.Close()
	a := &app{store: database.NewStore(db), logger: slog.New(slog.NewTextHandler(io.Discard, nil))}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		a.cleanupExpiredSessions(ctx, time.Millisecond)
		close(done)
	}()
	time.Sleep(10 * time.Millisecond)
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("cleanup loop did not stop after cancel")
	}
}
