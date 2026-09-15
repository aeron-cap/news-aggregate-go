CREATE TABLE IF NOT EXISTS sources (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL,
    url TEXT NOT NULL UNIQUE,
    is_active BOOLEAN NOT NULL DEFAULT 1
);

CREATE INDEX IF NOT EXISTS idx_sources_name_active ON sources(name, is_active);

CREATE TABLE IF NOT EXISTS interests (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    keyword TEXT NOT NULL UNIQUE,
    weight REAL NOT NULL DEFAULT 1,
    is_main BOOLEAN NOT NULL DEFAULT 0,
    anchor TEXT,
    is_active BOOLEAN NOT NULL DEFAULT 0
);

CREATE INDEX IF NOT EXISTS idx_interests_keyword_active ON interests(keyword, is_active);

CREATE TABLE IF NOT EXISTS articles (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    source_id INTEGER REFERENCES sources(id) ON DELETE SET NULL,
    title TEXT NOT NULL,
    summary TEXT,
    author TEXT,
    url TEXT NOT NULL UNIQUE,
    source_date DATETIME,
    relevance_score REAL NOT NULL DEFAULT 0,
    recency_score REAL NOT NULL DEFAULT 0,
    weighted_score REAL NOT NULL DEFAULT 0,
    batch_date DATE NOT NULL,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_articles_batch_weighted ON articles(batch_date, weighted_score DESC);