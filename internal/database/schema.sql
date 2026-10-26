CREATE TABLE IF NOT EXISTS users (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    token TEXT NOT NULL UNIQUE,
    expires_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS sources (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name TEXT NOT NULL,
    url TEXT NOT NULL UNIQUE,
    is_seeded BOOLEAN NOT NULL DEFAULT FALSE
);

CREATE TABLE IF NOT EXISTS user_sources (
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    source_id UUID NOT NULL REFERENCES sources(id) ON DELETE CASCADE,
    is_active BOOLEAN NOT NULL DEFAULT TRUE,
    PRIMARY KEY (user_id, source_id)
);

CREATE INDEX IF NOT EXISTS idx_user_sources_user_active ON user_sources(user_id, is_active);

CREATE TABLE IF NOT EXISTS interests (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    keyword TEXT NOT NULL UNIQUE,
    anchor TEXT
);

CREATE INDEX IF NOT EXISTS idx_interests_keyword ON interests(keyword);

CREATE TABLE IF NOT EXISTS user_interests (
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    interest_id UUID NOT NULL REFERENCES interests(id) ON DELETE CASCADE,
    weight DOUBLE PRECISION NOT NULL DEFAULT 1,
    is_main BOOLEAN NOT NULL DEFAULT FALSE,
    is_active BOOLEAN NOT NULL DEFAULT FALSE,
    PRIMARY KEY (user_id, interest_id)
);

CREATE TABLE IF NOT EXISTS articles (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    source_id UUID REFERENCES sources(id) ON DELETE SET NULL,
    title TEXT NOT NULL,
    summary TEXT,
    author TEXT,
    url TEXT NOT NULL,
    source_date TIMESTAMPTZ,
    tags TEXT,
    batch_date TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (source_id, url)
);

CREATE INDEX IF NOT EXISTS idx_articles_batch ON articles(batch_date);

CREATE TABLE IF NOT EXISTS user_articles (
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    article_id UUID NOT NULL REFERENCES articles(id) ON DELETE CASCADE,
    read_at TIMESTAMPTZ,
    weighted_score DOUBLE PRECISION NOT NULL DEFAULT 0,
    PRIMARY KEY (article_id, user_id)
);

CREATE INDEX IF NOT EXISTS idx_user_articles_user_read_score ON user_articles(user_id, read_at, weighted_score);