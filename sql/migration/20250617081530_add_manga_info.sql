-- +goose Up
-- +goose StatementBegin
-- SELECT 'up SQL query';
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

CREATE TABLE manga (
    id uuid PRIMARY KEY DEFAULT uuid_generate_v4(),
    created_at timestamp NOT NULL DEFAULT now(),
    updated_at timestamp NOT NULL DEFAULT now(),
    title varchar(255) UNIQUE NOT NULL,
    cover_url text NOT NULL,
    alt_titles text [] NOT NULL,
    status boolean NOT NULL DEFAULT true,
    authors text [] NOT NULL,
    description text NOT NULL,
    genres text [] NOT NULL,
    last_chapter real NOT NULL DEFAULT 0,
    chapters_amount int NOT NULL DEFAULT 0
);

CREATE TABLE chapter (
    id uuid PRIMARY KEY DEFAULT uuid_generate_v4(),
    created_at timestamp NOT NULL DEFAULT now(),
    updated_at timestamp NOT NULL DEFAULT now(),
    manga_id uuid NOT NULL REFERENCES manga (id) ON DELETE CASCADE,
    name varchar(255) UNIQUE NOT NULL,
    number real NOT NULL DEFAULT 0,
    imgs text []
);

CREATE INDEX idx_chapter_manga_id ON chapter (manga_id);
CREATE INDEX idx_manga_id ON manga (id);

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS manga; -- noqa:
-- +goose StatementEnd
