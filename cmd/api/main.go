package main

import (
	"context"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/aeron-cap/news-aggregator/internal/config"
	"github.com/aeron-cap/news-aggregator/internal/database"
)

type app struct {
	store       *database.Store
	logger      *slog.Logger
	databaseURL string
}

const (
	_shutdownPeriod      = 15 * time.Second
	_shutdownHardPeriod  = 3 * time.Second
	_readinessDrainDelay = 5 * time.Second
)

var isShuttingDown atomic.Bool

func main() {
	sigCtx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))

	if err := config.LoadEnv(".env"); err != nil {
		logger.Error("Failed to load configuration", "err", err)
		os.Exit(1)
	}

	databaseURL := database.ConnectionURL()
	db, err := database.Open(sigCtx, databaseURL)
	if err != nil {
		logger.Error("Failed to open database", "err", err)
		os.Exit(1)
	}
	defer db.Close()

	app := &app{
		store:       database.NewStore(db),
		logger:      logger,
		databaseURL: databaseURL,
	}

	mux := routes(app)

	ongoingCtx, stopOngoingGracefully := context.WithCancel(sigCtx)

	srv := &http.Server{
		Addr:         ":6767",
		Handler:      mux,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  60 * time.Second,
		BaseContext: func(_ net.Listener) context.Context {
			return ongoingCtx
		},
	}

	go func() {
		logger.Info("Starting server on :6767")
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			panic(err)
		}
	}()

	<-sigCtx.Done()
	stop()
	isShuttingDown.Store(true)
	logger.Info("Received shutdown signal, shutting down.")

	time.Sleep(_readinessDrainDelay)
	logger.Info("Readiness check propagated, now waiting for ongoing requests to finish.")

	shutdownCtx, cancel := context.WithTimeout(ongoingCtx, _shutdownPeriod)
	defer cancel()
	err = srv.Shutdown(shutdownCtx)
	stopOngoingGracefully()
	if err != nil {
		logger.Info("Failed to wait for ongoing requests to finish, waiting for forced cancellation.")
		time.Sleep(_shutdownHardPeriod)
	}

	logger.Info("Server shut down gracefully.")
}
