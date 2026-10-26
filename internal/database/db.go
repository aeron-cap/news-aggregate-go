package database

import (
	"context"
	"database/sql"
	_ "embed"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
)

//go:embed schema.sql
var schemaSQL string

//go:embed seed.sql
var seed string

const DefaultURL = "postgresql://news_aggregator@127.0.0.1:5432/news_aggregator?sslmode=disable"

func ConnectionURL() string {
	if url := os.Getenv("DATABASE_URL"); url != "" {
		return url
	}
	return DefaultURL
}

func Open(ctx context.Context, databaseURL string) (*sql.DB, error) {
	config, err := pgx.ParseConfig(databaseURL)
	if err != nil {
		return nil, fmt.Errorf("invalid PostgreSQL connection configuration")
	}
	config.RuntimeParams["timezone"] = "UTC"
	config.ConnectTimeout = 5 * time.Second
	db := stdlib.OpenDB(*config)

	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(30 * time.Minute)

	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

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
	if err := validateExistingIDTypes(ctx, tx); err != nil {
		tx.Rollback()
		db.Close()
		return nil, err
	}
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

func validateExistingIDTypes(ctx context.Context, tx *sql.Tx) error {
	var table, column string
	err := tx.QueryRowContext(ctx, `SELECT table_name, column_name FROM information_schema.columns
		WHERE table_schema = current_schema()
		AND table_name IN ('users','sources','interests','articles','user_sources','user_interests','user_articles')
		AND column_name IN ('id','user_id','source_id','interest_id','article_id')
		AND data_type <> 'uuid' ORDER BY table_name, column_name LIMIT 1`).Scan(&table, &column)
	if err == sql.ErrNoRows {
		return nil
	}
	if err != nil {
		return err
	}
	return fmt.Errorf("database column %s.%s must be UUID; run migrations/001_uuid_ids.sql before starting", table, column)
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
			if columns[column] == "" {
				return fmt.Errorf("database table %s is missing column %s; migrate the existing database before starting", table, column)
			}
			if (column == "id" || strings.HasSuffix(column, "_id")) && columns[column] != "uuid" {
				return fmt.Errorf("database column %s.%s must be UUID; run migrations/001_uuid_ids.sql before starting", table, column)
			}
		}
	}
	return nil
}

func tableColumns(ctx context.Context, tx *sql.Tx, table string) (map[string]string, error) {
	rows, err := tx.QueryContext(ctx, `SELECT column_name, data_type FROM information_schema.columns
		WHERE table_schema = current_schema() AND table_name = $1`, table)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	columns := map[string]string{}
	for rows.Next() {
		var name, dataType string
		if err := rows.Scan(&name, &dataType); err != nil {
			return nil, err
		}
		columns[name] = dataType
	}
	return columns, rows.Err()
}
