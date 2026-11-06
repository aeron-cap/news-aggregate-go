package database

import (
	"context"
	"database/sql"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
)

func assertUUIDPrimaryKeys(t *testing.T, db *sql.DB) {
	t.Helper()
	var total, invalid int
	err := db.QueryRow(`SELECT count(*), count(*) FILTER (WHERE cols.data_type <> 'uuid')
        FROM information_schema.table_constraints constraints
        JOIN information_schema.key_column_usage keys
          ON keys.constraint_name = constraints.constraint_name AND keys.table_schema = constraints.table_schema
        JOIN information_schema.columns cols
          ON cols.table_schema = keys.table_schema AND cols.table_name = keys.table_name AND cols.column_name = keys.column_name
        WHERE constraints.table_schema = current_schema() AND constraints.constraint_type = 'PRIMARY KEY'`).Scan(&total, &invalid)
	if err != nil || total != 10 || invalid != 0 {
		t.Fatalf("primary key columns: total=%d non-UUID=%d err=%v", total, invalid, err)
	}
}

func TestUUIDDefaultsAndNullableSource(t *testing.T) {
	store, db := schemaTestStore(t)
	assertUUIDPrimaryKeys(t, db)
	if err := store.CreateAnonymousSession(context.Background(), "uuid-default", time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	id, err := store.FetchUserID(context.Background(), "uuid-default")
	if err != nil || id == uuid.Nil || id.Version() != 4 {
		t.Fatalf("generated user UUID=%s err=%v", id, err)
	}
	var articleID uuid.UUID
	if err := db.QueryRow(`INSERT INTO articles(title,url,batch_date) VALUES ('No source','https://example.com/no-source',CURRENT_TIMESTAMP) RETURNING id`).Scan(&articleID); err != nil {
		t.Fatal(err)
	}
	var sourceID uuid.NullUUID
	if err := db.QueryRow(`SELECT source_id FROM articles WHERE id=$1`, articleID).Scan(&sourceID); err != nil || sourceID.Valid {
		t.Fatalf("nullable source UUID=%+v err=%v", sourceID, err)
	}
	encoded, err := json.Marshal(Article{ID: articleID, SourceID: sourceID})
	var payload map[string]any
	if err != nil || json.Unmarshal(encoded, &payload) != nil || payload["id"] != articleID.String() || payload["source_id"] != nil {
		t.Fatalf("UUID JSON representation: %s err=%v", encoded, err)
	}
}
