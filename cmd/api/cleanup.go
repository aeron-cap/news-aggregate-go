package main

import (
	"context"
	"time"
)

const (
	_sessionCleanupInterval = time.Hour
	_sessionCleanupTimeout  = 30 * time.Second
)

// cleanupExpiredSessions deletes expired sessions once at startup and then every interval until ctx ends.
func (a *app) cleanupExpiredSessions(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		a.deleteExpiredSessions(ctx)
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (a *app) deleteExpiredSessions(ctx context.Context) {
	runCtx, cancel := context.WithTimeout(ctx, _sessionCleanupTimeout)
	defer cancel()
	deleted, err := a.store.DeleteExpiredSessions(runCtx, time.Now())
	if err != nil {
		if ctx.Err() == nil {
			a.logger.ErrorContext(ctx, "delete expired sessions failed", "err", err)
		}
		return
	}
	if deleted > 0 {
		a.logger.InfoContext(ctx, "deleted expired sessions", "count", deleted)
	}
}
