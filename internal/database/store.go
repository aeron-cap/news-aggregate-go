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
	ID       int64  `json:"id"`
	Name     string `json:"name"`
	URL      string `json:"url"`
	IsActive bool   `json:"is_active"`
}

func (s *Store) GetSources(ctx context.Context) ([]Source, error) {
	const stmt = `SELECT id, name, url, is_active FROM sources WHERE is_active = 1`

	rows, err := s.db.QueryContext(ctx, stmt)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	sources := []Source{}
	for rows.Next() {
		var src Source
		if err := rows.Scan(&src.ID, &src.Name, &src.URL, &src.IsActive); err != nil {
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
func (s *Store) InsertSources(ctx context.Context, sources []Source) error {
	stmt, err := s.db.PrepareContext(ctx, `INSERT INTO sources (name, url, is_active) VALUES (?, ?, ?)`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, src := range sources {
		if _, err := stmt.ExecContext(ctx, src.Name, src.URL, src.IsActive); err != nil {
			return err
		}
	}

	return nil
}

// Delete from Sources Table

type Interest struct {
	ID      int64  `json:"id"`
	Keyword string `json:"keyword"`
	Weight  string `json:"weight"`
}

func (s *Store) GetInterests(ctx context.Context) ([]Interest, error) {
	const stmt = `SELECT id, keyword, weight FROM interests`

	rows, err := s.db.QueryContext(ctx, stmt)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	interests := []Interest{}
	for rows.Next() {
		var interest Interest
		if err := rows.Scan(&interest.ID, &interest.Keyword, &interest.Weight); err != nil {
			return nil, err
		}
		interests = append(interests, interest)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return interests, nil
}

// Get from Interests Table by ID
// Write to Interests Table
func (s *Store) InsertInterests(ctx context.Context, interests []Interest) error {
	stmt, err := s.db.PrepareContext(ctx, `INSERT INTO interests (keyword, weight) VALUES (?, ?)`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, interest := range interests {
		if _, err := stmt.ExecContext(ctx, interest.Keyword, interest.Weight); err != nil {
			return err
		}
	}

	return nil
}

// Delete from Interests Table
//
// Get All from Articles Table
// Get from Articles Table by ID
// Write to Articles Table
// Delete from Articles Table
