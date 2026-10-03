CREATE TABLE IF NOT EXISTS users (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    last_seen_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_users_last_seen ON users(last_seen_at);

CREATE TABLE IF NOT EXISTS sources (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL,
    url TEXT NOT NULL UNIQUE,
    is_seeded BOOLEAN NOT NULL DEFAULT 0
);

CREATE TABLE IF NOT EXISTS user_sources (
    user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    source_id INTEGER NOT NULL REFERENCES sources(id) ON DELETE CASCADE,
    is_active BOOLEAN NOT NULL DEFAULT 1,
    PRIMARY KEY (user_id, source_id)
);

CREATE INDEX IF NOT EXISTS idx_user_sources_user_active ON user_sources(user_id, is_active);

CREATE TABLE IF NOT EXISTS interests (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    keyword TEXT NOT NULL UNIQUE,
    anchor TEXT
);

CREATE INDEX IF NOT EXISTS idx_interests_keyword ON interests(keyword);

CREATE TABLE IF NOT EXISTS user_interests (
    user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    interest_id INTEGER NOT NULL REFERENCES interests(id) ON DELETE CASCADE,
    weight REAL NOT NULL DEFAULT 1,
    is_main BOOLEAN NOT NULL DEFAULT 0,
    is_active BOOLEAN NOT NULL DEFAULT 0,
    PRIMARY KEY (user_id, interest_id)
);

CREATE TABLE IF NOT EXISTS articles (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    source_id INTEGER REFERENCES sources(id) ON DELETE SET NULL,
    title TEXT NOT NULL,
    summary TEXT,
    author TEXT,
    url TEXT NOT NULL,
    source_date DATETIME,
    tags TEXT,
    batch_date DATETIME NOT NULL,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (source_id, url)
);

CREATE INDEX IF NOT EXISTS idx_articles_batch ON articles(batch_date);

CREATE TABLE IF NOT EXISTS user_articles (
    user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    article_id INTEGER NOT NULL REFERENCES articles(id) ON DELETE CASCADE,
    read_at DATETIME,
    weighted_score REAL NOT NULL DEFAULT 0,
    PRIMARY KEY (article_id, user_id)
);

CREATE INDEX IF NOT EXISTS idx_user_articles_user_read_score ON user_articles(user_id, read_at, weighted_score);