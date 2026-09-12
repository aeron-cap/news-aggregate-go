package database

import (
	"context"
	"database/sql"
)

type Store struct {
	db *sql.DB
}

func NewStore(db *sql.DB) *Store {
	return &Store{db: db}
}

type Source struct {
	ID           int64  `json:"id"`
	Name         string `json:"name"`
	FeedURL      string `json:"feed_url"`
	IsActive     bool   `json:"is_active"`
	LastModified string `json:"last_modified"`
}

func (s *Store) GetSources(ctx context.Context) ([]Source, error) {
	const stmt = `SELECT id, name, feed_url, is_active FROM sources`

	rows, err := s.db.QueryContext(ctx, stmt)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	
	sources := []Source{}
	for rows.Next() {
		var src Source
		if err := rows.Scan(&src.ID, &src.Name, &src.FeedURL, &src.IsActive); err != nil {
			return nil, err
		}
		sources = append(sources, src)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return sources, nil
}

// TODO: 
// Get from Sources Table by ID 
// Write to Sources Table
// Delete from Sources Table
// 
// Get All from Interests Table
// Get from Interests Table by ID
// Write to Interests Table
// Delete from Interests Table
// 
// Get All from Articles Table
// Get from Articles Table by ID
// Write to Articles Table
// Delete from Articles Table
