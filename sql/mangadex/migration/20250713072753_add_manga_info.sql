-- +goose Up
-- +goose StatementBegin
-- SELECT 'up SQL query';
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

CREATE TABLE IF NOT EXISTS manga (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    type TEXT NOT NULL,
    title_en TEXT NOT NULL,
    description_en TEXT,
    latest_uploaded_chapter INTEGER,
    alt_titles TEXT [] NOT NULL DEFAULT '{}',
    tags TEXT [] NOT NULL DEFAULT '{}',
    original_language TEXT,
    last_chapter TEXT,
    status TEXT,
    publication_demographic TEXT,
    year INTEGER,
    content_rating TEXT,
    state TEXT,
    created_at TIMESTAMP NOT NULL DEFAULT now(),
    updated_at TIMESTAMP NOT NULL DEFAULT now(),
    is_locked BOOLEAN NOT NULL DEFAULT false,
    links_mal TEXT
);

CREATE INDEX IF NOT EXISTS idx_manga_title ON manga (title_en);
CREATE INDEX IF NOT EXISTS idx_manga_id ON manga (id);

CREATE TABLE IF NOT EXISTS chapter (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    created_at TIMESTAMP NOT NULL DEFAULT now(),
    updated_at TIMESTAMP NOT NULL DEFAULT now(),
    manga_id UUID NOT NULL REFERENCES manga (id) ON DELETE CASCADE,
    title VARCHAR(255) NOT NULL UNIQUE,
    number REAL NOT NULL DEFAULT 0,
    imgs TEXT [] DEFAULT '{}'
);

CREATE INDEX IF NOT EXISTS idx_chapter_manga_id ON chapter (manga_id);

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS chapter CASCADE;
DROP TABLE IF EXISTS manga CASCADE;
-- +goose StatementEnd
