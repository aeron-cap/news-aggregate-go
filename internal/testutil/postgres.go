// Package testutil provides isolated PostgreSQL schemas for integration tests.
package testutil

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
)

func Postgres(t *testing.T) (*sql.DB, string) {
	t.Helper()
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("set TEST_DATABASE_URL to run PostgreSQL integration tests")
	}
	config, err := pgx.ParseConfig(databaseURL)
	if err != nil {
		t.Fatal("invalid TEST_DATABASE_URL")
	}
	admin := stdlib.OpenDB(*config)
	admin.SetMaxOpenConns(1)
	t.Cleanup(func() { admin.Close() })
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	random := make([]byte, 12)
	if _, err := rand.Read(random); err != nil {
		t.Fatal(err)
	}
	schema := "test_" + hex.EncodeToString(random)
	quoted := pgx.Identifier{schema}.Sanitize()
	if _, err := admin.ExecContext(ctx, "CREATE SCHEMA "+quoted); err != nil {
		t.Fatalf("create test schema: %v", err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if _, err := admin.ExecContext(ctx, "DROP SCHEMA "+quoted+" CASCADE"); err != nil {
			t.Errorf("drop test schema: %v", err)
		}
	})
	// options is understood by both pgx and Python's libpq driver.
	parsed, err := url.Parse(databaseURL)
	if err != nil || (parsed.Scheme != "postgres" && parsed.Scheme != "postgresql") {
		t.Fatal("TEST_DATABASE_URL must be a postgresql:// URL")
	}
	query := parsed.Query()
	query.Set("options", strings.TrimSpace(query.Get("options")+" -c search_path="+schema+" -c timezone=UTC"))
	// libpq connection URIs require percent-encoded spaces, not form '+' encoding.
	parsed.RawQuery = strings.ReplaceAll(query.Encode(), "+", "%20")
	scopedURL := parsed.String()
	config, err = pgx.ParseConfig(scopedURL)
	if err != nil {
		t.Fatal("invalid schema-scoped PostgreSQL configuration")
	}
	db := stdlib.OpenDB(*config)
	db.SetMaxOpenConns(5)
	t.Cleanup(func() { db.Close() })
	return db, scopedURL
}
