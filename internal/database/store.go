package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

type Store struct{ db *sql.DB }

func NewStore(db *sql.DB) *Store { return &Store{db: db} }

var ErrSharedSourceName = errors.New("shared source names cannot be changed; use a different feed URL")

func (s *Store) CreateAnonymousSession(ctx context.Context, tokenHash string, expiresAt time.Time) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var userID uuid.UUID
	if err := tx.QueryRowContext(ctx, `INSERT INTO users (token, expires_at) VALUES ($1, $2) RETURNING id`, tokenHash, expiresAt.UTC()).Scan(&userID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO user_sources (user_id, source_id) SELECT $1, id FROM sources WHERE is_seeded = TRUE`, userID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO user_interests (user_id, interest_id) SELECT $1, id FROM interests`, userID); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) FetchUserID(ctx context.Context, tokenHash string) (uuid.UUID, error) {
	var userID uuid.UUID
	err := s.db.QueryRowContext(ctx, `SELECT id FROM users WHERE token = $1 AND expires_at > $2`, tokenHash, time.Now().UTC()).Scan(&userID)
	if err != nil {
		return uuid.Nil, fmt.Errorf("find session: %w", err)
	}
	return userID, nil
}

func (s *Store) UpdateToken(ctx context.Context, oldTokenHash, newTokenHash string, expiresAt time.Time) error {
	now := time.Now().UTC()
	if !expiresAt.After(now) {
		return fmt.Errorf("session expiry must be in the future")
	}
	result, err := s.db.ExecContext(ctx, `UPDATE users SET token = $1, expires_at = $2 WHERE token = $3 AND expires_at > $4`, newTokenHash, expiresAt.UTC(), oldTokenHash, now)
	if err != nil {
		return err
	}
	return requireAffected(result, "valid session")
}

func requireAffected(result sql.Result, entity string) error {
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count == 0 {
		return fmt.Errorf("%s not found: %w", entity, sql.ErrNoRows)
	}
	return nil
}

type Source struct {
	ID       uuid.UUID `json:"id"`
	Name     string    `json:"name"`
	URL      string    `json:"url"`
	IsActive bool      `json:"is_active"`
}

const sourceVisibility = `(s.is_seeded = TRUE OR us.user_id IS NOT NULL)`
const activeSourceVisibility = sourceVisibility + ` AND COALESCE(us.is_active, s.is_seeded) = TRUE`

func (s *Store) GetActiveSources(ctx context.Context, userID uuid.UUID) ([]Source, error) {
	return s.getSources(ctx, userID, activeSourceVisibility)
}

func (s *Store) GetAllSources(ctx context.Context, userID uuid.UUID) ([]Source, error) {
	return s.getSources(ctx, userID, sourceVisibility)
}
func (s *Store) getSources(ctx context.Context, userID uuid.UUID, predicate string) ([]Source, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT s.id, s.name, s.url, COALESCE(us.is_active, s.is_seeded)
        FROM sources s LEFT JOIN user_sources us ON us.source_id = s.id AND us.user_id = $1
        WHERE `+predicate+` ORDER BY s.id`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	sources := []Source{}
	for rows.Next() {
		var source Source
		if err := rows.Scan(&source.ID, &source.Name, &source.URL, &source.IsActive); err != nil {
			return nil, err
		}
		sources = append(sources, source)
	}
	return sources, rows.Err()
}

type SourcesUpdate struct {
	Name     string `json:"name"`
	URL      string `json:"url"`
	IsActive bool   `json:"is_active"`
}

func sourceForURL(ctx context.Context, tx *sql.Tx, source SourcesUpdate) (uuid.UUID, error) {

	if _, err := tx.ExecContext(ctx, `INSERT INTO sources (name, url) VALUES ($1, $2) ON CONFLICT(url) DO NOTHING`, source.Name, source.URL); err != nil {
		return uuid.Nil, err
	}
	var id uuid.UUID
	err := tx.QueryRowContext(ctx, `SELECT id FROM sources WHERE url = $1`, source.URL).Scan(&id)
	return id, err
}

func setSubscription(ctx context.Context, tx *sql.Tx, userID, sourceID uuid.UUID, active bool) error {
	_, err := tx.ExecContext(ctx, `INSERT INTO user_sources (user_id, source_id, is_active) VALUES ($1, $2, $3)
        ON CONFLICT(user_id, source_id) DO UPDATE SET is_active = excluded.is_active`, userID, sourceID, active)
	return err
}

func (s *Store) InsertSources(ctx context.Context, userID uuid.UUID, sources []SourcesUpdate) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	seen := map[string]bool{}
	for _, source := range sources {
		source.Name, source.URL = strings.TrimSpace(source.Name), strings.TrimSpace(source.URL)
		if seen[source.URL] {
			return fmt.Errorf("duplicate feed URL in source batch")
		}
		seen[source.URL] = true
		sourceID, err := sourceForURL(ctx, tx, source)
		if err != nil {
			return err
		}
		if err := setSubscription(ctx, tx, userID, sourceID, source.IsActive); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) UpdateSources(ctx context.Context, userID uuid.UUID, updates map[uuid.UUID]SourcesUpdate) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	type change struct {
		oldID, newID uuid.UUID
		seeded       bool
		update       SourcesUpdate
	}
	changes := []change{}
	seen := map[string]bool{}
	for id, update := range updates {
		update.Name, update.URL = strings.TrimSpace(update.Name), strings.TrimSpace(update.URL)
		if seen[update.URL] {
			return fmt.Errorf("duplicate feed URL in source batch")
		}
		seen[update.URL] = true
		var name, url string
		var seeded bool
		err := tx.QueryRowContext(ctx, `SELECT s.name, s.url, s.is_seeded FROM sources s
            LEFT JOIN user_sources us ON us.source_id = s.id AND us.user_id = $1
            WHERE s.id = $2 AND `+sourceVisibility, userID, id).Scan(&name, &url, &seeded)
		if err != nil {
			return fmt.Errorf("find source %s: %w", id, err)
		}
		targetID := id
		if update.URL != url {
			targetID, err = sourceForURL(ctx, tx, update)
			if err != nil {
				return err
			}
		} else if update.Name != name {
			var others int
			if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM user_sources WHERE source_id = $1 AND user_id != $2`, id, userID).Scan(&others); err != nil {
				return err
			}
			if seeded || others > 0 {
				return ErrSharedSourceName
			}
			if _, err := tx.ExecContext(ctx, `UPDATE sources SET name = $1 WHERE id = $2`, update.Name, id); err != nil {
				return err
			}
		}
		changes = append(changes, change{id, targetID, seeded, update})
	}
	for _, change := range changes {
		if change.oldID == change.newID {
			continue
		}
		if change.seeded {
			if err := setSubscription(ctx, tx, userID, change.oldID, false); err != nil {
				return err
			}
		} else if _, err := tx.ExecContext(ctx, `DELETE FROM user_sources WHERE user_id = $1 AND source_id = $2`, userID, change.oldID); err != nil {
			return err
		}
	}
	for _, change := range changes {
		if err := setSubscription(ctx, tx, userID, change.newID, change.update.IsActive); err != nil {
			return err
		}
	}
	return tx.Commit()
}

type Interest struct {
	ID       uuid.UUID      `json:"id"`
	Keyword  string         `json:"keyword"`
	Weight   float64        `json:"weight"`
	IsMain   bool           `json:"is_main"`
	Anchor   sql.NullString `json:"anchor"`
	IsActive bool           `json:"is_active"`
}

func (s *Store) GetInterests(ctx context.Context, userID uuid.UUID) ([]Interest, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT i.id, i.keyword, COALESCE(ui.weight, 1), COALESCE(ui.is_main, FALSE), i.anchor, COALESCE(ui.is_active, FALSE)
        FROM interests i LEFT JOIN user_interests ui ON ui.interest_id = i.id AND ui.user_id = $1 ORDER BY i.id`, userID)
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
	return interests, rows.Err()
}

type InterestUpdate struct {
	IsActive bool
	IsMain   bool
}

func (s *Store) UpdateInterests(ctx context.Context, userID uuid.UUID, updates map[uuid.UUID]InterestUpdate) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for id, update := range updates {
		result, err := tx.ExecContext(ctx, `INSERT INTO user_interests (user_id, interest_id, is_active, is_main, weight)
            SELECT $1, id, $2, $3, CASE WHEN $4 THEN 1 ELSE 0 END FROM interests WHERE id = $5
            ON CONFLICT(user_id, interest_id) DO UPDATE SET is_active = excluded.is_active, is_main = excluded.is_main,
                weight = CASE WHEN excluded.is_active = FALSE THEN 0 ELSE user_interests.weight END`, userID, update.IsActive, update.IsMain, update.IsActive, id)
		if err != nil {
			return err
		}
		if err := requireAffected(result, "interest"); err != nil {
			return err
		}
	}
	return tx.Commit()
}

type Article struct {
	ID            uuid.UUID      `json:"id"`
	SourceID      uuid.NullUUID  `json:"source_id"`
	Title         string         `json:"title"`
	Summary       sql.NullString `json:"summary"`
	Author        sql.NullString `json:"author"`
	URL           string         `json:"url"`
	SourceDate    sql.NullTime   `json:"source_date"`
	Tags          sql.NullString `json:"tags"`
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
	for _, article := range articles {
		_, err := tx.ExecContext(ctx, `INSERT INTO articles (source_id, title, summary, author, url, source_date, tags, batch_date)
            VALUES ($1, $2, $3, $4, $5, $6, $7, $8) ON CONFLICT(source_id, url) DO UPDATE SET
                title = excluded.title, summary = excluded.summary, author = excluded.author,
                source_date = excluded.source_date, tags = excluded.tags, batch_date = excluded.batch_date`,
			article.SourceID, article.Title, article.Summary, article.Author, article.URL, article.SourceDate, article.Tags, article.BatchDate.UTC())
		if err != nil {
			return fmt.Errorf("insert article: %w", err)
		}
	}
	return tx.Commit()
}

func (s *Store) DeleteArticlesByBatchDate(ctx context.Context, batchDate time.Time) error {
	batchDate = batchDate.UTC()
	start := time.Date(batchDate.Year(), batchDate.Month(), batchDate.Day(), 0, 0, 0, 0, time.UTC)
	_, err := s.db.ExecContext(ctx, `DELETE FROM articles WHERE batch_date >= $1 AND batch_date < $2`, start, start.AddDate(0, 0, 1))
	return err
}

func (s *Store) MarkArticleAsRead(ctx context.Context, userID, articleID uuid.UUID) error {
	result, err := s.db.ExecContext(ctx, `INSERT INTO user_articles (user_id, article_id, read_at)
        SELECT $1, a.id, CURRENT_TIMESTAMP FROM articles a JOIN sources s ON s.id = a.source_id
        LEFT JOIN user_sources us ON us.source_id = s.id AND us.user_id = $2
        WHERE a.id = $3 AND `+activeSourceVisibility+`
        ON CONFLICT(user_id, article_id) DO UPDATE SET read_at = COALESCE(user_articles.read_at, excluded.read_at)`, userID, userID, articleID)
	if err != nil {
		return err
	}
	return requireAffected(result, "article")
}

const unreadArticleVisibility = activeSourceVisibility + ` AND NOT EXISTS (
    SELECT 1 FROM user_articles reads JOIN articles read_article ON read_article.id = reads.article_id
    WHERE reads.user_id = $1 AND reads.read_at IS NOT NULL AND read_article.url = a.url)`

func (s *Store) GetUnreadArticles(ctx context.Context, userID uuid.UUID) ([]Article, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT a.id, a.source_id, a.title, a.summary, a.author, a.url, a.source_date, a.tags,
        COALESCE(ua.weighted_score, 0), a.batch_date, a.created_at, ua.read_at, s.name
        FROM articles a JOIN sources s ON s.id = a.source_id
        LEFT JOIN user_sources us ON us.source_id = s.id AND us.user_id = $1
        LEFT JOIN user_articles ua ON ua.article_id = a.id AND ua.user_id = $1
        WHERE `+unreadArticleVisibility+` ORDER BY a.batch_date DESC, COALESCE(ua.weighted_score, 0) DESC, a.id`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	articles := []Article{}
	for rows.Next() {
		var article Article
		if err := rows.Scan(&article.ID, &article.SourceID, &article.Title, &article.Summary, &article.Author, &article.URL,
			&article.SourceDate, &article.Tags, &article.WeightedScore, &article.BatchDate, &article.CreatedAt, &article.ReadAt, &article.SourceName); err != nil {
			return nil, err
		}
		articles = append(articles, article)
	}
	return articles, rows.Err()
}

func (s *Store) SaveArticleScores(ctx context.Context, userID uuid.UUID, articles []Article) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, article := range articles {
		result, err := tx.ExecContext(ctx, `INSERT INTO user_articles (user_id, article_id, weighted_score)
            SELECT $1, a.id, $2 FROM articles a JOIN sources s ON s.id = a.source_id
            LEFT JOIN user_sources us ON us.source_id = s.id AND us.user_id = $3
            WHERE a.id = $4 AND `+activeSourceVisibility+`
            ON CONFLICT(user_id, article_id) DO UPDATE SET weighted_score = excluded.weighted_score`, userID, article.WeightedScore, userID, article.ID)
		if err != nil {
			return err
		}
		if err := requireAffected(result, "article"); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) CountUnreadArticles(ctx context.Context, userID uuid.UUID) (int, error) {
	var count int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(DISTINCT a.url) FROM articles a JOIN sources s ON s.id = a.source_id
        LEFT JOIN user_sources us ON us.source_id = s.id AND us.user_id = $1 WHERE `+unreadArticleVisibility, userID).Scan(&count)
	return count, err
}

func scanTimestamp(row *sql.Row) (time.Time, error) {
	var value sql.NullTime
	if err := row.Scan(&value); err != nil {
		return time.Time{}, err
	}
	if !value.Valid {
		return time.Time{}, nil
	}
	return value.Time.UTC(), nil
}

func (s *Store) GetLastFetchDate(ctx context.Context, userID uuid.UUID) (time.Time, error) {
	return scanTimestamp(s.db.QueryRowContext(ctx, `SELECT MIN(latest_fetch) FROM (
        SELECT COALESCE(MAX(a.batch_date), '1970-01-01 00:00:00') AS latest_fetch FROM sources s
        LEFT JOIN user_sources us ON us.source_id = s.id AND us.user_id = $1
        LEFT JOIN articles a ON a.source_id = s.id
        WHERE `+activeSourceVisibility+` GROUP BY s.id)`, userID))
}

func (s *Store) GetLastSourceDate(ctx context.Context, userID uuid.UUID) (time.Time, error) {
	return scanTimestamp(s.db.QueryRowContext(ctx, `SELECT MAX(a.source_date) FROM articles a JOIN sources s ON s.id = a.source_id
        LEFT JOIN user_sources us ON us.source_id = s.id AND us.user_id = $1 WHERE `+unreadArticleVisibility, userID))
}
