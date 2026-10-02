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
