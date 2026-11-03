package database

import (
	"context"
	"testing"
	"time"

	"github.com/aeron-cap/news-aggregator/internal/testutil"
)

func TestListActiveUserIDs(t *testing.T) {
	store, db := schemaTestStore(t)
	ids, err := store.ListActiveUserIDs(context.Background(), time.Now())
	if err != nil || len(ids) != 0 {
		t.Fatalf("empty table: ids=%v err=%v", ids, err)
	}
	now := time.Now().UTC()
	if _, err := db.Exec(`INSERT INTO users (id, token, expires_at, created_at) VALUES ($1, 'second', $2, $3), ($4, 'first', $2, $5), ($6, 'expired', $7, $5)`,
		testutil.ID(2), now.Add(time.Hour), now, testutil.ID(1), now.Add(-time.Minute), testutil.ID(3), now.Add(-time.Hour)); err != nil {
		t.Fatal(err)
	}
	ids, err = store.ListActiveUserIDs(context.Background(), now)
	if err != nil {
		t.Fatal(err)
	}
	if len(ids) != 2 || ids[0] != testutil.ID(1) || ids[1] != testutil.ID(2) {
		t.Fatalf("ids = %v, want active users oldest first", ids)
	}
}
