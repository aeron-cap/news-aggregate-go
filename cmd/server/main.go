package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/aeron-cap/news-aggregator/internal/database"
)

type app struct {
	store *database.Store
}

func main() {
	ctx := context.Background()

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
	}
	
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", health)
	mux.HandleFunc("GET /feeds", app.getFeeds)
	
	srv := &http.Server{
		Addr: ":6767",
		Handler: mux,
		ReadTimeout: 5 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout: 60 * time.Second,
	}

	fmt.Println("Starting server on :6767")
	err = srv.ListenAndServe()
	if err != nil {
		panic(err)
	}
}

// TODOs
// spin up the db to store articles 
// setup interests 
// find nice algo to fetch based on interests and sort by date
// adjust algo to favor based on weight of interest keywords
// manage sources
// i think we cant get the html to parse, so we just need headline + other info and a link to main source
// interaction with fetched articles affect algorithm
// add blacklist