package database

import (
	"context"
	"database/sql"
	_ "embed"
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

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		db.Close()
		return nil, err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, schemaSQL); err != nil {
		tx.Rollback()
		db.Close()
		return nil, fmt.Errorf("failed to execute schema SQL: %w", err)
	}

	if err := validateSchema(ctx, tx); err != nil {
		tx.Rollback()
		db.Close()
		return nil, err
	}
	if _, err := tx.ExecContext(ctx, seed); err != nil {
		tx.Rollback()
		db.Close()
		return nil, fmt.Errorf("failed to seed db: %w", err)
	}
	if err := tx.Commit(); err != nil {
		db.Close()
		return nil, err
	}

	return db, nil
}

func validateSchema(ctx context.Context, tx *sql.Tx) error {
	for table, required := range map[string][]string{
		"users":          {"id", "token", "expires_at", "created_at"},
		"sources":        {"id", "name", "url", "is_seeded"},
		"interests":      {"id", "keyword", "anchor"},
		"articles":       {"id", "source_id", "title", "summary", "author", "url", "source_date", "tags", "batch_date", "created_at"},
		"user_sources":   {"user_id", "source_id", "is_active"},
		"user_interests": {"user_id", "interest_id", "weight", "is_main", "is_active"},
		"user_articles":  {"user_id", "article_id", "read_at", "weighted_score"},
	} {
		columns, err := tableColumns(ctx, tx, table)
		if err != nil {
			return err
		}
		for _, column := range required {
			if !columns[column] {
				return fmt.Errorf("database table %s is missing column %s; use a fresh DB_PATH or migrate the existing database", table, column)
			}
		}
	}
	return nil
}

func tableColumns(ctx context.Context, tx *sql.Tx, table string) (map[string]bool, error) {
	rows, err := tx.QueryContext(ctx, `PRAGMA table_info(`+table+`)`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	columns := map[string]bool{}
	for rows.Next() {
		var position, notNull, primaryKey int
		var name, columnType string
		var defaultValue any
		if err := rows.Scan(&position, &name, &columnType, &notNull, &defaultValue, &primaryKey); err != nil {
			return nil, err
		}
		columns[name] = true
	}
	return columns, rows.Err()
}
