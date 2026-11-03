// Command refresh rebuilds the feed of every user with an active session.
// It runs from a systemd timer (deploy/service/pocket-news-refresh.timer) or cron.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/aeron-cap/news-aggregator/internal/config"
	"github.com/aeron-cap/news-aggregator/internal/database"
	"github.com/aeron-cap/news-aggregator/internal/feed"
	"github.com/google/uuid"
)

// _perUserTimeout keeps one slow user from stalling the rest of the run.
const _perUserTimeout = time.Minute

var errAllFailed = errors.New("every user feed refresh failed")

type userLister interface {
	ListActiveUserIDs(ctx context.Context, now time.Time) ([]uuid.UUID, error)
}

// refreshFunc rebuilds one user's feed and reports whether anything was refreshed.
type refreshFunc func(ctx context.Context, userID uuid.UUID) (bool, error)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))

	if err := config.LoadEnv(".env"); err != nil {
		logger.Error("Failed to load configuration", "err", err)
		os.Exit(1)
	}

	db, err := database.Open(ctx, database.ConnectionURL())
	if err != nil {
		logger.Error("Failed to open database", "err", err)
		os.Exit(1)
	}
	store := database.NewStore(db)

	err = run(ctx, logger, store, func(ctx context.Context, userID uuid.UUID) (bool, error) {
		return feed.CreateFeed(ctx, store, userID)
	}, _perUserTimeout)
	db.Close()
	if err != nil {
		logger.Error("Feed refresh failed", "err", err)
		os.Exit(1)
	}
}

// run refreshes each active user with its own timeout. Per-user failures are logged and
// skipped; it only returns an error if users can't be listed, the run is cancelled, or every user failed.
func run(ctx context.Context, logger *slog.Logger, users userLister, refresh refreshFunc, perUserTimeout time.Duration) error {
	userIDs, err := users.ListActiveUserIDs(ctx, time.Now())
	if err != nil {
		return fmt.Errorf("list users: %w", err)
	}

	var refreshed, upToDate, failed int
	for _, userID := range userIDs {
		if err := ctx.Err(); err != nil {
			return err
		}
		userCtx, cancel := context.WithTimeout(ctx, perUserTimeout)
		changed, err := refresh(userCtx, userID)
		cancel()
		switch {
		case err != nil:
			failed++
			logger.Error("Refresh user feed failed", "user_id", userID, "err", err)
		case changed:
			refreshed++
		default:
			upToDate++
		}
	}

	logger.Info("Feed refresh finished", "users", len(userIDs), "refreshed", refreshed, "up_to_date", upToDate, "failed", failed)
	if len(userIDs) > 0 && failed == len(userIDs) {
		return errAllFailed
	}
	return nil
}
