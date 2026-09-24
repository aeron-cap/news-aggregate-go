package database

import (
	"context"
	"database/sql"
	"fmt"
	"time"
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

type Interest struct {
	ID       int64          `json:"id"`
	Keyword  string         `json:"keyword"`
	Weight   float64        `json:"weight"`
	IsMain   bool           `json:"is_main"`
	Anchor   sql.NullString `json:"anchor"`
	IsActive bool           `json:"is_active"`
}

func (s *Store) GetInterests(ctx context.Context) ([]Interest, error) {
	const stmt = `SELECT id, keyword, weight, is_main, anchor, is_active FROM interests`

	rows, err := s.db.QueryContext(ctx, stmt)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	interests := []Interest{}
	for rows.Next() {
		var interest Interest
		if err := rows.Scan(&interest.ID, &interest.Keyword, &interest.Weight, &interest.IsMain, &interest.Anchor, &interest.IsActive); err != nil {
			return nil, err
		}
		interests = append(interests, interest)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return interests, nil
}

func (s *Store) UpdateInterestActivation(ctx context.Context, interestID int64, isActive bool) error {
	stmt, err := s.db.PrepareContext(ctx, `UPDATE interests SET is_active = ? WHERE id = ?`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	_, err = stmt.ExecContext(ctx, isActive, interestID)
	return err
}

func (s *Store) UpdateInterestsActivation(ctx context.Context, interestIDs []int64, isActive bool) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmt, err := tx.PrepareContext(ctx, `UPDATE interests SET is_active = ? WHERE id = ?`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, id := range interestIDs {
		_, err = stmt.ExecContext(ctx, isActive, id)
		if err != nil {
			return err
		}
	}

	return tx.Commit()
}

type Article struct {
	ID            int64          `json:"id"`
	SourceID      sql.NullInt64  `json:"source_id"`
	Title         string         `json:"title"`
	Summary       sql.NullString `json:"summary"`
	Author        sql.NullString `json:"author"`
	URL           string         `json:"url"`
	SourceDate    sql.NullTime   `json:"source_date"`
	WeightedScore float64        `json:"weighted_score"`
	BatchDate     time.Time      `json:"batch_date"`
	CreatedAt     time.Time      `json:"created_at"`
	ReadAt        sql.NullTime   `json:"read_at"`
	SourceName    sql.NullString `json:"source_name"`
}

func (s *Store) InsertArticles(ctx context.Context, articles []Article) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmt, err := tx.PrepareContext(ctx, `
        INSERT INTO articles
        (source_id, title, summary, author, url, source_date, weighted_score, batch_date)
        VALUES (?, ?, ?, ?, ?, ?, ?, ?)
        ON CONFLICT (url) DO UPDATE SET
        	weighted_score = excluded.weighted_score
        WHERE excluded.weighted_score > articles.weighted_score
    `)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, article := range articles {
		_, err := stmt.ExecContext(
			ctx,
			article.SourceID,
			article.Title,
			article.Summary,
			article.Author,
			article.URL,
			article.SourceDate,
			article.WeightedScore,
			article.BatchDate,
		)
		if err != nil {
			fmt.Printf("Warning: failed to insert article %s: %v\n", article.URL, err)
			continue
		}
	}

	return tx.Commit()
}

func (s *Store) DeleteArticlesByBatchDate(ctx context.Context, batchDate time.Time) error {
	stmt, err := s.db.PrepareContext(ctx, `DELETE FROM articles WHERE batch_date = ?`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	_, err = stmt.ExecContext(ctx, batchDate.Format("2006-01-02"))
	return err
}

func (s *Store) MarkArticleAsRead(ctx context.Context, articleID int64) error {
	stmt, err := s.db.PrepareContext(ctx, `UPDATE articles SET read_at = CURRENT_TIMESTAMP WHERE id = ?`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	_, err = stmt.ExecContext(ctx, articleID)
	return err
}

func (s *Store) GetUnreadArticles(ctx context.Context) ([]Article, error) {
	stmt := `
        SELECT a.*, s.name
        FROM articles a
        LEFT JOIN sources s ON a.source_id = s.id
        WHERE a.read_at IS NULL
        ORDER BY a.batch_date DESC, a.weighted_score DESC
    `

	rows, err := s.db.QueryContext(ctx, stmt)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	articles := []Article{}
	for rows.Next() {
		var article Article
		if err := rows.Scan(
			&article.ID,
			&article.SourceID,
			&article.Title,
			&article.Summary,
			&article.Author,
			&article.URL,
			&article.SourceDate,
			&article.WeightedScore,
			&article.BatchDate,
			&article.CreatedAt,
			&article.ReadAt,
			&article.SourceName,
		); err != nil {
			return nil, err
		}
		articles = append(articles, article)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return articles, nil
}

func (s *Store) CountUnreadArticles(ctx context.Context) (int, error) {
	stmt := `SELECT COUNT(*) FROM articles WHERE read_at IS NULL`

	row := s.db.QueryRowContext(ctx, stmt)
	var count int
	if err := row.Scan(&count); err != nil {
		return 0, err
	}

	return count, nil
}

func (s *Store) GetLastFetchDate(ctx context.Context) (time.Time, error) {
	const stmt = `SELECT MAX(batch_date) FROM articles WHERE read_at IS NOT NULL`

	row := s.db.QueryRowContext(ctx, stmt)

	var rawDateTime sql.NullString
	if err := row.Scan(&rawDateTime); err != nil {
		return time.Time{}, err
	}

	if !rawDateTime.Valid {
		return time.Time{}, nil
	}

	const sqliteTimestampLayout = "2006-01-02 15:04:05.999999-07:00"
	parsed, err := time.Parse(sqliteTimestampLayout, rawDateTime.String)
	if err != nil {
		return time.Time{}, fmt.Errorf("failed to parse last fetch date %q: %w", rawDateTime.String, err)
	}

	return parsed, nil
}

func (s *Store) GetLastSourceDate(ctx context.Context) (time.Time, error) {
	const stmt = `SELECT MAX(source_date) FROM articles WHERE read_at IS NULL`

	row := s.db.QueryRowContext(ctx, stmt)

	var rawDateTime sql.NullString
	if err := row.Scan(&rawDateTime); err != nil {
		return time.Time{}, err
	}

	if !rawDateTime.Valid {
		return time.Time{}, nil
	}

	const sqliteTimestampLayout = "2006-01-02 15:04:05Z07:00"
	parsed, err := time.Parse(sqliteTimestampLayout, rawDateTime.String)
	if err != nil {
		return time.Time{}, fmt.Errorf("failed to parse last fetch date %q: %w", rawDateTime.String, err)
	}

	return parsed, nil
}
