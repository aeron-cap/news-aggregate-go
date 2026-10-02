package main

import (
	"context"
	"database/sql"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/aeron-cap/news-aggregator/internal/database"
)

func TestChangeInterestsRejectsInvalidPayload(t *testing.T) {
	for _, body := range []string{
		`{}`,
		`null`,
		`[]`,
		`{"id":1,"is_active":true}`,
		`{"abc":{"isActive":true,"isMain":false}}`,
		`{"0":{"isActive":true,"isMain":false}}`,
		`{"-1":{"isActive":true,"isMain":false}}`,
		`{"1":{"isActive":true}}`,
		`{"1":{"isMain":true}}`,
		`{"1":null}`,
		`{"1":{"isActive":null,"isMain":false}}`,
		`{"1":{"isActive":"true","isMain":false}}`,
		`{"1":{"isActive":true,"isMain":false,"weight":1}}`,
		`{"1":{"isActive":true,"isMain":false}} {}`,
		`{"1":{"isActive":true,"isMain":false}} garbage`,
		`{"1":{"isActive":true,"isMain":false},"2":{"isActive":false}}`,
	} {
		t.Run(body, func(t *testing.T) {
			a := &app{logger: slog.New(slog.NewTextHandler(io.Discard, nil))}
			w := httptest.NewRecorder()
			r := httptest.NewRequest(http.MethodPost, "/change-interests", strings.NewReader(body))
			a.changeInterests(w, r)
			if w.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400; body: %s", w.Code, w.Body.String())
			}
		})
	}
}

func TestChangeInterestsMissingIDRollsBack(t *testing.T) {
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(`CREATE TABLE interests (id INTEGER PRIMARY KEY, keyword TEXT, weight REAL, is_main BOOLEAN, anchor TEXT, is_active BOOLEAN);
		INSERT INTO interests VALUES (1, 'science', 1, 0, NULL, 0)`); err != nil {
		t.Fatal(err)
	}
	a := &app{
		store:  database.NewStore(db),
		cache:  &feedCache{},
		logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/change-interests", strings.NewReader(`{"1":{"isActive":true,"isMain":true},"999":{"isActive":false,"isMain":false}}`))
	a.changeInterests(w, r)
	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404; body: %s", w.Code, w.Body.String())
	}
	interests, err := a.store.GetInterests(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if interests[0].IsActive || interests[0].IsMain {
		t.Fatal("interest flags changed despite missing ID in batch")
	}
}
