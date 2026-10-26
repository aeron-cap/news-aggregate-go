-- One-time migration from the previous BIGINT schema. Stop the API first.
-- Run with psql -v ON_ERROR_STOP=1 -f migrations/001_uuid_ids.sql.
BEGIN;
LOCK TABLE users, sources, interests, articles, user_sources, user_interests, user_articles IN ACCESS EXCLUSIVE MODE;

DO $$ BEGIN
    IF (SELECT data_type FROM information_schema.columns WHERE table_schema = current_schema() AND table_name = 'users' AND column_name = 'id') NOT IN ('bigint', 'integer') THEN
        RAISE EXCEPTION 'Expected the previous integer-key schema; this migration runs once';
    END IF;
END $$;

ALTER TABLE users ADD COLUMN _uuid_id UUID NOT NULL DEFAULT gen_random_uuid();
ALTER TABLE sources ADD COLUMN _uuid_id UUID NOT NULL DEFAULT gen_random_uuid();
ALTER TABLE interests ADD COLUMN _uuid_id UUID NOT NULL DEFAULT gen_random_uuid();
ALTER TABLE articles ADD COLUMN _uuid_id UUID NOT NULL DEFAULT gen_random_uuid();

ALTER TABLE user_sources ADD COLUMN _uuid_user_id UUID;
UPDATE user_sources child SET _uuid_user_id = parent._uuid_id FROM users parent WHERE child.user_id = parent.id;
ALTER TABLE user_sources ADD COLUMN _uuid_source_id UUID;
UPDATE user_sources child SET _uuid_source_id = parent._uuid_id FROM sources parent WHERE child.source_id = parent.id;
ALTER TABLE user_interests ADD COLUMN _uuid_user_id UUID;
UPDATE user_interests child SET _uuid_user_id = parent._uuid_id FROM users parent WHERE child.user_id = parent.id;
ALTER TABLE user_interests ADD COLUMN _uuid_interest_id UUID;
UPDATE user_interests child SET _uuid_interest_id = parent._uuid_id FROM interests parent WHERE child.interest_id = parent.id;
ALTER TABLE articles ADD COLUMN _uuid_source_id UUID;
UPDATE articles child SET _uuid_source_id = parent._uuid_id FROM sources parent WHERE child.source_id = parent.id;
ALTER TABLE user_articles ADD COLUMN _uuid_user_id UUID;
UPDATE user_articles child SET _uuid_user_id = parent._uuid_id FROM users parent WHERE child.user_id = parent.id;
ALTER TABLE user_articles ADD COLUMN _uuid_article_id UUID;
UPDATE user_articles child SET _uuid_article_id = parent._uuid_id FROM articles parent WHERE child.article_id = parent.id;

ALTER TABLE user_sources DROP CONSTRAINT user_sources_user_id_fkey;
ALTER TABLE user_sources DROP CONSTRAINT user_sources_source_id_fkey;
ALTER TABLE user_interests DROP CONSTRAINT user_interests_user_id_fkey;
ALTER TABLE user_interests DROP CONSTRAINT user_interests_interest_id_fkey;
ALTER TABLE articles DROP CONSTRAINT articles_source_id_fkey;
ALTER TABLE user_articles DROP CONSTRAINT user_articles_user_id_fkey;
ALTER TABLE user_articles DROP CONSTRAINT user_articles_article_id_fkey;

ALTER TABLE users ALTER COLUMN id DROP IDENTITY IF EXISTS;
ALTER TABLE users ALTER COLUMN id DROP DEFAULT;
ALTER TABLE users ALTER COLUMN id TYPE UUID USING _uuid_id;
ALTER TABLE users ALTER COLUMN id SET DEFAULT gen_random_uuid();
ALTER TABLE sources ALTER COLUMN id DROP IDENTITY IF EXISTS;
ALTER TABLE sources ALTER COLUMN id DROP DEFAULT;
ALTER TABLE sources ALTER COLUMN id TYPE UUID USING _uuid_id;
ALTER TABLE sources ALTER COLUMN id SET DEFAULT gen_random_uuid();
ALTER TABLE interests ALTER COLUMN id DROP IDENTITY IF EXISTS;
ALTER TABLE interests ALTER COLUMN id DROP DEFAULT;
ALTER TABLE interests ALTER COLUMN id TYPE UUID USING _uuid_id;
ALTER TABLE interests ALTER COLUMN id SET DEFAULT gen_random_uuid();
ALTER TABLE articles ALTER COLUMN id DROP IDENTITY IF EXISTS;
ALTER TABLE articles ALTER COLUMN id DROP DEFAULT;
ALTER TABLE articles ALTER COLUMN id TYPE UUID USING _uuid_id;
ALTER TABLE articles ALTER COLUMN id SET DEFAULT gen_random_uuid();

ALTER TABLE user_sources ALTER COLUMN user_id TYPE UUID USING _uuid_user_id;
ALTER TABLE user_sources ALTER COLUMN source_id TYPE UUID USING _uuid_source_id;
ALTER TABLE user_interests ALTER COLUMN user_id TYPE UUID USING _uuid_user_id;
ALTER TABLE user_interests ALTER COLUMN interest_id TYPE UUID USING _uuid_interest_id;
ALTER TABLE articles ALTER COLUMN source_id TYPE UUID USING _uuid_source_id;
ALTER TABLE user_articles ALTER COLUMN user_id TYPE UUID USING _uuid_user_id;
ALTER TABLE user_articles ALTER COLUMN article_id TYPE UUID USING _uuid_article_id;

ALTER TABLE users DROP COLUMN _uuid_id;
ALTER TABLE sources DROP COLUMN _uuid_id;
ALTER TABLE interests DROP COLUMN _uuid_id;
ALTER TABLE articles DROP COLUMN _uuid_id;
ALTER TABLE user_sources DROP COLUMN _uuid_user_id;
ALTER TABLE user_sources DROP COLUMN _uuid_source_id;
ALTER TABLE user_interests DROP COLUMN _uuid_user_id;
ALTER TABLE user_interests DROP COLUMN _uuid_interest_id;
ALTER TABLE articles DROP COLUMN _uuid_source_id;
ALTER TABLE user_articles DROP COLUMN _uuid_user_id;
ALTER TABLE user_articles DROP COLUMN _uuid_article_id;

ALTER TABLE user_sources ADD CONSTRAINT user_sources_user_id_fkey FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE;
ALTER TABLE user_sources ADD CONSTRAINT user_sources_source_id_fkey FOREIGN KEY (source_id) REFERENCES sources(id) ON DELETE CASCADE;
ALTER TABLE user_interests ADD CONSTRAINT user_interests_user_id_fkey FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE;
ALTER TABLE user_interests ADD CONSTRAINT user_interests_interest_id_fkey FOREIGN KEY (interest_id) REFERENCES interests(id) ON DELETE CASCADE;
ALTER TABLE articles ADD CONSTRAINT articles_source_id_fkey FOREIGN KEY (source_id) REFERENCES sources(id) ON DELETE SET NULL;
ALTER TABLE user_articles ADD CONSTRAINT user_articles_user_id_fkey FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE;
ALTER TABLE user_articles ADD CONSTRAINT user_articles_article_id_fkey FOREIGN KEY (article_id) REFERENCES articles(id) ON DELETE CASCADE;

COMMIT;
