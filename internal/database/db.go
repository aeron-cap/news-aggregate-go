package database

import (
	"context"
	_ "embed"
	"database/sql"
	"fmt"
	_ "github.com/mattn/go-sqlite3"
)

//go:embed schema.sql
var schemaSQL string

//go:embed seed.sql
var seed string

func Open(ctx context.Context, dbPath string) (*sql.DB, error) {
	dsn := fmt.Sprintf("%s?_journal_mode=WAL&_foreign_keys=ON&_busy_timeout=5000&parseTime=true&_time_format=sqlite", dbPath)

	db, err := sql.Open("sqlite3", dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	db.SetConnMaxLifetime(0)

	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	if _, err := db.ExecContext(ctx, schemaSQL); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to execute schema SQL: %w", err)
	}

	if _, err := db.ExecContext(ctx, seed); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to seed db: %w", err)
	}

	return db, nil
}
