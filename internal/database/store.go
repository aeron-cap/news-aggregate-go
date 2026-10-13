package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
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
	result, err := tx.ExecContext(ctx, `INSERT INTO users (token, expires_at) VALUES (?, ?)`, tokenHash, expiresAt.UTC())
	if err != nil {
		return err
	}
	userID, err := result.LastInsertId()
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO user_sources (user_id, source_id) SELECT ?, id FROM sources WHERE is_seeded = 1`, userID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO user_interests (user_id, interest_id) SELECT ?, id FROM interests`, userID); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) FetchUserID(ctx context.Context, tokenHash string) (int64, error) {
	var userID int64
	err := s.db.QueryRowContext(ctx, `SELECT id FROM users WHERE token = ? AND julianday(expires_at) > julianday(?)`, tokenHash, time.Now().UTC()).Scan(&userID)
	if err != nil {
		return 0, fmt.Errorf("find session: %w", err)
	}
	return userID, nil
}

func (s *Store) UpdateToken(ctx context.Context, oldTokenHash, newTokenHash string, expiresAt time.Time) error {
	now := time.Now().UTC()
	if !expiresAt.After(now) {
		return fmt.Errorf("session expiry must be in the future")
	}
	result, err := s.db.ExecContext(ctx, `UPDATE users SET token = ?, expires_at = ? WHERE token = ? AND julianday(expires_at) > julianday(?)`, newTokenHash, expiresAt.UTC(), oldTokenHash, now)
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
	ID       int64  `json:"id"`
	Name     string `json:"name"`
	URL      string `json:"url"`
	IsActive bool   `json:"is_active"`
}

const sourceVisibility = `(s.is_seeded = 1 OR us.user_id IS NOT NULL)`
const activeSourceVisibility = sourceVisibility + ` AND COALESCE(us.is_active, s.is_seeded) = 1`

func (s *Store) GetActiveSources(ctx context.Context, userID int64) ([]Source, error) {
	return s.getSources(ctx, userID, activeSourceVisibility)
}

func (s *Store) GetAllSources(ctx context.Context, userID int64) ([]Source, error) {
	return s.getSources(ctx, userID, sourceVisibility)
}
func (s *Store) getSources(ctx context.Context, userID int64, predicate string) ([]Source, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT s.id, s.name, s.url, COALESCE(us.is_active, s.is_seeded)
        FROM sources s LEFT JOIN user_sources us ON us.source_id = s.id AND us.user_id = ?
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

func sourceForURL(ctx context.Context, tx *sql.Tx, source SourcesUpdate) (int64, error) {

	if _, err := tx.ExecContext(ctx, `INSERT INTO sources (name, url) VALUES (?, ?) ON CONFLICT(url) DO NOTHING`, source.Name, source.URL); err != nil {
		return 0, err
	}
	var id int64
	err := tx.QueryRowContext(ctx, `SELECT id FROM sources WHERE url = ?`, source.URL).Scan(&id)
	return id, err
}

func setSubscription(ctx context.Context, tx *sql.Tx, userID, sourceID int64, active bool) error {
	_, err := tx.ExecContext(ctx, `INSERT INTO user_sources (user_id, source_id, is_active) VALUES (?, ?, ?)
        ON CONFLICT(user_id, source_id) DO UPDATE SET is_active = excluded.is_active`, userID, sourceID, active)
	return err
}

func (s *Store) InsertSources(ctx context.Context, userID int64, sources []SourcesUpdate) error {
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

func (s *Store) UpdateSources(ctx context.Context, userID int64, updates map[int64]SourcesUpdate) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	type change struct {
		oldID, newID int64
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
            LEFT JOIN user_sources us ON us.source_id = s.id AND us.user_id = ?
            WHERE s.id = ? AND `+sourceVisibility, userID, id).Scan(&name, &url, &seeded)
		if err != nil {
			return fmt.Errorf("find source %d: %w", id, err)
		}
		targetID := id
		if update.URL != url {
			targetID, err = sourceForURL(ctx, tx, update)
			if err != nil {
				return err
			}
		} else if update.Name != name {
			var others int
			if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM user_sources WHERE source_id = ? AND user_id != ?`, id, userID).Scan(&others); err != nil {
				return err
			}
			if seeded || others > 0 {
				return ErrSharedSourceName
			}
			if _, err := tx.ExecContext(ctx, `UPDATE sources SET name = ? WHERE id = ?`, update.Name, id); err != nil {
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
		} else if _, err := tx.ExecContext(ctx, `DELETE FROM user_sources WHERE user_id = ? AND source_id = ?`, userID, change.oldID); err != nil {
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
	ID       int64          `json:"id"`
	Keyword  string         `json:"keyword"`
	Weight   float64        `json:"weight"`
	IsMain   bool           `json:"is_main"`
	Anchor   sql.NullString `json:"anchor"`
	IsActive bool           `json:"is_active"`
}

func (s *Store) GetInterests(ctx context.Context, userID int64) ([]Interest, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT i.id, i.keyword, COALESCE(ui.weight, 1), COALESCE(ui.is_main, 0), i.anchor, COALESCE(ui.is_active, 0)
        FROM interests i LEFT JOIN user_interests ui ON ui.interest_id = i.id AND ui.user_id = ? ORDER BY i.id`, userID)
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

func (s *Store) UpdateInterests(ctx context.Context, userID int64, updates map[int64]InterestUpdate) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for id, update := range updates {
		result, err := tx.ExecContext(ctx, `INSERT INTO user_interests (user_id, interest_id, is_active, is_main, weight)
            SELECT ?, id, ?, ?, CASE WHEN ? THEN 1 ELSE 0 END FROM interests WHERE id = ?
            ON CONFLICT(user_id, interest_id) DO UPDATE SET is_active = excluded.is_active, is_main = excluded.is_main,
                weight = CASE WHEN excluded.is_active = 0 THEN 0 ELSE user_interests.weight END`, userID, update.IsActive, update.IsMain, update.IsActive, id)
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
	ID            int64          `json:"id"`
	SourceID      sql.NullInt64  `json:"source_id"`
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
            VALUES (?, ?, ?, ?, ?, ?, ?, ?) ON CONFLICT(source_id, url) DO UPDATE SET
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
	_, err := s.db.ExecContext(ctx, `DELETE FROM articles WHERE date(batch_date) = ?`, batchDate.UTC().Format("2006-01-02"))
	return err
}

func (s *Store) MarkArticleAsRead(ctx context.Context, userID, articleID int64) error {
	result, err := s.db.ExecContext(ctx, `INSERT INTO user_articles (user_id, article_id, read_at)
        SELECT ?, a.id, CURRENT_TIMESTAMP FROM articles a JOIN sources s ON s.id = a.source_id
        LEFT JOIN user_sources us ON us.source_id = s.id AND us.user_id = ?
        WHERE a.id = ? AND `+activeSourceVisibility+`
        ON CONFLICT(user_id, article_id) DO UPDATE SET read_at = COALESCE(user_articles.read_at, excluded.read_at)`, userID, userID, articleID)
	if err != nil {
		return err
	}
	return requireAffected(result, "article")
}

const unreadArticleVisibility = activeSourceVisibility + ` AND NOT EXISTS (
    SELECT 1 FROM user_articles reads JOIN articles read_article ON read_article.id = reads.article_id
    WHERE reads.user_id = ? AND reads.read_at IS NOT NULL AND read_article.url = a.url)`

func (s *Store) GetUnreadArticles(ctx context.Context, userID int64) ([]Article, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT a.id, a.source_id, a.title, a.summary, a.author, a.url, a.source_date, a.tags,
        COALESCE(ua.weighted_score, 0), a.batch_date, a.created_at, ua.read_at, s.name
        FROM articles a JOIN sources s ON s.id = a.source_id
        LEFT JOIN user_sources us ON us.source_id = s.id AND us.user_id = ?
        LEFT JOIN user_articles ua ON ua.article_id = a.id AND ua.user_id = ?
        WHERE `+unreadArticleVisibility+` ORDER BY a.batch_date DESC, COALESCE(ua.weighted_score, 0) DESC, a.id`, userID, userID, userID)
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

func (s *Store) SaveArticleScores(ctx context.Context, userID int64, articles []Article) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, article := range articles {
		result, err := tx.ExecContext(ctx, `INSERT INTO user_articles (user_id, article_id, weighted_score)
            SELECT ?, a.id, ? FROM articles a JOIN sources s ON s.id = a.source_id
            LEFT JOIN user_sources us ON us.source_id = s.id AND us.user_id = ?
            WHERE a.id = ? AND `+activeSourceVisibility+`
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

func (s *Store) CountUnreadArticles(ctx context.Context, userID int64) (int, error) {
	var count int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(DISTINCT a.url) FROM articles a JOIN sources s ON s.id = a.source_id
        LEFT JOIN user_sources us ON us.source_id = s.id AND us.user_id = ? WHERE `+unreadArticleVisibility, userID, userID).Scan(&count)
	return count, err
}

func scanTimestamp(row *sql.Row) (time.Time, error) {
	var value sql.NullString
	if err := row.Scan(&value); err != nil {
		return time.Time{}, err
	}
	if !value.Valid {
		return time.Time{}, nil
	}
	for _, layout := range []string{time.RFC3339Nano, "2006-01-02 15:04:05.999999999-07:00", "2006-01-02 15:04:05.999999999", "2006-01-02"} {
		if parsed, err := time.Parse(layout, value.String); err == nil {
			return parsed.UTC(), nil
		}
	}
	return time.Time{}, fmt.Errorf("invalid database timestamp %q", value.String)
}

func (s *Store) GetLastFetchDate(ctx context.Context, userID int64) (time.Time, error) {
	return scanTimestamp(s.db.QueryRowContext(ctx, `SELECT MIN(latest_fetch) FROM (
        SELECT COALESCE(MAX(a.batch_date), '1970-01-01 00:00:00') AS latest_fetch FROM sources s
        LEFT JOIN user_sources us ON us.source_id = s.id AND us.user_id = ?
        LEFT JOIN articles a ON a.source_id = s.id
        WHERE `+activeSourceVisibility+` GROUP BY s.id)`, userID))
}

func (s *Store) GetLastSourceDate(ctx context.Context, userID int64) (time.Time, error) {
	return scanTimestamp(s.db.QueryRowContext(ctx, `SELECT MAX(a.source_date) FROM articles a JOIN sources s ON s.id = a.source_id
        LEFT JOIN user_sources us ON us.source_id = s.id AND us.user_id = ? WHERE `+unreadArticleVisibility, userID, userID))
}

func (s *Store) DatabasePath(ctx context.Context) (string, error) {
	rows, err := s.db.QueryContext(ctx, `PRAGMA database_list`)
	if err != nil {
		return "", err
	}
	defer rows.Close()
	for rows.Next() {
		var sequence int
		var name, path string
		if err := rows.Scan(&sequence, &name, &path); err != nil {
			return "", err
		}
		if name == "main" && path != "" {
			return path, nil
		}
	}
	if err := rows.Err(); err != nil {
		return "", err
	}
	return "", fmt.Errorf("Python scoring requires a file-backed database")
}
