package database

import (
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/aeron-cap/news-aggregator/internal/testutil"
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

func TestUUIDMigrationPreservesRelationships(t *testing.T) {
	db, databaseURL := testutil.Postgres(t)
	legacy, err := os.ReadFile(filepath.Join("testdata", "legacy_schema.sql"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(string(legacy)); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`
        INSERT INTO users(id,token,expires_at) VALUES (1,'legacy-one',CURRENT_TIMESTAMP+INTERVAL '1 day'),(2,'legacy-two',CURRENT_TIMESTAMP+INTERVAL '1 day');
        INSERT INTO sources(id,name,url) VALUES (1,'Legacy','https://legacy.example/rss');
        INSERT INTO interests(id,keyword,anchor) VALUES (1,'software','software.n.01');
        INSERT INTO articles(id,source_id,title,url,batch_date) VALUES (1,1,'Legacy story','https://legacy.example/story',CURRENT_TIMESTAMP),(2,NULL,'Detached','https://legacy.example/detached',CURRENT_TIMESTAMP);
        INSERT INTO user_sources(user_id,source_id,is_active) VALUES (1,1,TRUE),(2,1,FALSE);
        INSERT INTO user_interests(user_id,interest_id,weight,is_main,is_active) VALUES (1,1,0.7,TRUE,TRUE),(2,1,0.3,FALSE,TRUE);
        INSERT INTO user_articles(user_id,article_id,read_at,weighted_score) VALUES (1,1,CURRENT_TIMESTAMP,2),(2,1,NULL,0.5);
    `); err != nil {
		t.Fatal(err)
	}
	if opened, err := Open(context.Background(), databaseURL); err == nil {
		opened.Close()
		t.Fatal("integer-key database was accepted without migration")
	}
	migration, err := os.ReadFile(filepath.Join("..", "..", "migrations", "001_uuid_ids.sql"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(string(migration)); err != nil {
		t.Fatalf("UUID migration: %v", err)
	}
	assertUUIDPrimaryKeys(t, db)
	opened, err := Open(context.Background(), databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer opened.Close()
	store := NewStore(opened)
	first, err := store.FetchUserID(context.Background(), "legacy-one")
	if err != nil || first == uuid.Nil {
		t.Fatalf("original session was lost: %s %v", first, err)
	}
	second, err := store.FetchUserID(context.Background(), "legacy-two")
	if err != nil || first == second {
		t.Fatalf("second session was lost: %s %v", second, err)
	}
	var articleID, sourceID uuid.UUID
	if err := db.QueryRow(`SELECT id,source_id FROM articles WHERE url='https://legacy.example/story'`).Scan(&articleID, &sourceID); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		id            uuid.UUID
		active, read  bool
		score, weight float64
	}{
		{first, true, true, 2, 0.7}, {second, false, false, 0.5, 0.3},
	} {
		var active, read bool
		var score, weight float64
		err := db.QueryRow(`SELECT us.is_active, ua.read_at IS NOT NULL, ua.weighted_score, ui.weight
            FROM user_sources us JOIN user_articles ua ON ua.user_id=us.user_id
            JOIN articles a ON a.id=ua.article_id AND a.source_id=us.source_id
            JOIN user_interests ui ON ui.user_id=us.user_id JOIN interests i ON i.id=ui.interest_id
            WHERE us.user_id=$1 AND ua.article_id=$2 AND i.keyword='software'`, tc.id, articleID).Scan(&active, &read, &score, &weight)
		if err != nil || active != tc.active || read != tc.read || score != tc.score || weight != tc.weight {
			t.Fatalf("migration changed personal state for %s: active=%v read=%v score=%v weight=%v err=%v", tc.id, active, read, score, weight, err)
		}
	}
	if _, err := db.Exec(`DELETE FROM users WHERE id=$1`, first); err != nil {
		t.Fatal(err)
	}
	var remaining int
	if err := db.QueryRow(`SELECT count(*) FROM user_articles WHERE user_id=$1`, second).Scan(&remaining); err != nil || remaining != 1 {
		t.Fatalf("cascade changed another reader: count=%d err=%v", remaining, err)
	}
	if _, err := db.Exec(`DELETE FROM sources WHERE id=$1`, sourceID); err != nil {
		t.Fatal(err)
	}
	var detached uuid.NullUUID
	if err := db.QueryRow(`SELECT source_id FROM articles WHERE id=$1`, articleID).Scan(&detached); err != nil || detached.Valid {
		t.Fatalf("ON DELETE SET NULL was lost: %+v err=%v", detached, err)
	}
}
