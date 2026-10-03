package database

import (
	"context"
	"database/sql"
	"errors"
	"testing"
)

func TestUpdateInterests(t *testing.T) {
	for _, tc := range []struct {
		name    string
		updates map[int64]InterestUpdate
		missing bool
	}{
		{
			name: "updates both flags independently",
			updates: map[int64]InterestUpdate{
				1: {IsActive: true, IsMain: true},
				2: {IsActive: false, IsMain: false},
			},
		},
		{
			name: "rolls back when an ID does not exist",
			updates: map[int64]InterestUpdate{
				1:   {IsActive: true, IsMain: true},
				2:   {IsActive: false, IsMain: false},
				999: {IsActive: true, IsMain: false},
			},
			missing: true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db, err := sql.Open("sqlite3", ":memory:")
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			db.SetMaxOpenConns(1)
			if _, err := db.Exec(schemaSQL); err != nil {
				t.Fatal(err)
			}
			if _, err := db.Exec(`INSERT INTO interests (id, keyword, is_active, is_main) VALUES (1, 'science', 0, 0), (2, 'sports', 1, 1)`); err != nil {
				t.Fatal(err)
			}
			store := NewStore(db)
			err = store.UpdateInterests(context.Background(), tc.updates)
			if tc.missing {
				if !errors.Is(err, sql.ErrNoRows) {
					t.Fatalf("expected missing interest error, got %v", err)
				}
			} else if err != nil {
				t.Fatal(err)
			}

			interests, err := store.GetInterests(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			for _, interest := range interests {
				want := tc.updates[interest.ID]
				if tc.missing {
					want = InterestUpdate{IsActive: interest.ID == 2, IsMain: interest.ID == 2}
				}
				if interest.IsActive != want.IsActive || interest.IsMain != want.IsMain {
					t.Errorf("interest %d: got active=%v main=%v, want %+v", interest.ID, interest.IsActive, interest.IsMain, want)
				}
			}
		})
	}
}

func sourceTestStore(t *testing.T) (*Store, *sql.DB) {
	t.Helper()
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(schemaSQL); err != nil {
		t.Fatal(err)
	}
	return NewStore(db), db
}

func TestGetSourcesOnlyActive(t *testing.T) {
	store, db := sourceTestStore(t)
	if _, err := db.Exec(`INSERT INTO sources (name, url, is_active) VALUES ('Science', 'https://science.example/rss', 1), ('Sports', 'https://sports.example/rss', 0)`); err != nil {
		t.Fatal(err)
	}
	sources, err := store.GetSources(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(sources) != 1 || sources[0].Name != "Science" || !sources[0].IsActive {
		t.Fatalf("feed sources include inactive rows: %+v", sources)
	}
}

func TestInsertSourcesAtomic(t *testing.T) {
	for _, duplicate := range []bool{false, true} {
		t.Run(map[bool]string{false: "success", true: "duplicate rolls back entire batch"}[duplicate], func(t *testing.T) {
			store, db := sourceTestStore(t)
			sources := []SourcesUpdate{
				{Name: "Science", URL: "https://science.example/rss", IsActive: true},
				{Name: "Sports", URL: "https://sports.example/rss", IsActive: false},
			}
			if duplicate {
				sources[1].URL = sources[0].URL
			}
			err := store.InsertSources(context.Background(), sources)
			if duplicate && err == nil {
				t.Fatal("duplicate URL should fail")
			}
			if !duplicate && err != nil {
				t.Fatal(err)
			}
			var count int
			if err := db.QueryRow(`SELECT COUNT(*) FROM sources`).Scan(&count); err != nil {
				t.Fatal(err)
			}
			want := 2
			if duplicate {
				want = 0
			}
			if count != want {
				t.Fatalf("source count=%d, want %d", count, want)
			}
			if !duplicate {
				var active bool
				if err := db.QueryRow(`SELECT is_active FROM sources WHERE name = 'Sports'`).Scan(&active); err != nil || active {
					t.Fatalf("inactive source: active=%v err=%v", active, err)
				}
			}
		})
	}
}

func TestUpdateSourcesAtomic(t *testing.T) {
	for _, mode := range []string{"success", "missing", "duplicate"} {
		t.Run(mode, func(t *testing.T) {
			store, db := sourceTestStore(t)
			if _, err := db.Exec(`INSERT INTO sources (id, name, url, is_active) VALUES (1, 'Science', 'https://science.example/rss', 1), (2, 'Sports', 'https://sports.example/rss', 0)`); err != nil {
				t.Fatal(err)
			}
			updates := map[int64]SourcesUpdate{
				1: {Name: "New Science", URL: "https://new-science.example/rss", IsActive: false},
				2: {Name: "New Sports", URL: "https://new-sports.example/rss", IsActive: true},
			}
			if mode == "missing" {
				updates[999] = SourcesUpdate{Name: "Missing", URL: "https://missing.example/rss"}
			}
			if mode == "duplicate" {
				updates[2] = updates[1]
			}
			err := store.UpdateSources(context.Background(), updates)
			if mode == "success" && err != nil {
				t.Fatal(err)
			}
			if mode != "success" && err == nil {
				t.Fatal("expected update error")
			}
			if mode == "missing" && !errors.Is(err, sql.ErrNoRows) {
				t.Fatalf("expected sql.ErrNoRows, got %v", err)
			}
			for id := int64(1); id <= 2; id++ {
				var got SourcesUpdate
				if err := db.QueryRow(`SELECT name, url, is_active FROM sources WHERE id = ?`, id).Scan(&got.Name, &got.URL, &got.IsActive); err != nil {
					t.Fatal(err)
				}
				want := updates[id]
				if mode != "success" {
					want = SourcesUpdate{Name: "Science", URL: "https://science.example/rss", IsActive: true}
					if id == 2 {
						want = SourcesUpdate{Name: "Sports", URL: "https://sports.example/rss", IsActive: false}
					}
				}
				if got != want {
					t.Errorf("source %d: got %+v, want %+v", id, got, want)
				}
			}
		})
	}
}
