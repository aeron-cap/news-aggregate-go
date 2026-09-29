package main

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/aeron-cap/news-aggregator/internal/database"
)

type feedCache struct {
	mu			sync.Mutex
	value		[]byte
	validUntil	time.Time
}

type app struct {
	store *database.Store
	cache *feedCache
}

const (
	_shutdownPeriod      = 15 * time.Second
	_shutdownHardPeriod  = 3 * time.Second
	_readinessDrainDelay = 5 * time.Second
)

var isShuttingDown atomic.Bool

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	dbPath := filepath.Join("internal/database", "news.db")
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		fmt.Printf("Failed to create database directory: %v\n", err)
	}

	db, err := database.Open(ctx, dbPath)
	if err != nil {
		log.Fatalf("Failed to open database: %v", err)
		return
	}
	defer db.Close()

	app := &app{
		store: database.NewStore(db),
		cache: &feedCache{},
	}
	
	mux := routes(app)
	
	ongoingCtx, stopOngoingGracefully := context.WithCancel(context.Background())
	
	srv := &http.Server{
		Addr: ":6767",
		Handler: mux,
		ReadTimeout: 5 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout: 60 * time.Second,
		BaseContext: func(_ net.Listener) context.Context {
			return ongoingCtx
		},
	}

	go func() {
		log.Println("Starting server on :6767")
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			panic(err)
		}
	}()

	<-ctx.Done()
	stop()
	isShuttingDown.Store(true)
	log.Println("Received shutdown signal, shutting down.")

	time.Sleep(_readinessDrainDelay)
	log.Println("Readiness check propagated, now waiting for ongoing requests to finish.")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), _shutdownPeriod)
	defer cancel()
	err = srv.Shutdown(shutdownCtx)
	stopOngoingGracefully()
	if err != nil {
		log.Println("Failed to wait for ongoing requests to finish, waiting for forced cancellation.")
		time.Sleep(_shutdownHardPeriod)
	}

	log.Println("Server shut down gracefully.")
}
