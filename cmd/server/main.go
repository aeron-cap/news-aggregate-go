package main

import (
	"fmt"
	"net/http"
	"time"
)

func main() {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", health)
	mux.HandleFunc("GET /feeds", getFeeds)
	
	srv := &http.Server{
		Addr: ":6767",
		Handler: mux,
		ReadTimeout: 5 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout: 60 * time.Second,
	}

	fmt.Println("Starting server on :6767")
	err := srv.ListenAndServe()
	if err != nil {
		panic(err)
	}
}

// TODOs
// add timeout to limit time i have to wait for the feeds to fetch
// i think we cant get the html to parse, so we just need headline + other info and a link to main source
// spin up the db to store articles 
// setup interests 
// find nice algo to fetch based on interests and sort by date
// manage sources
// interaction with fetched articles affect algorithm