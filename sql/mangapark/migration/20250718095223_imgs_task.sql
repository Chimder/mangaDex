-- +goose Up
-- +goose StatementBegin

CREATE TABLE img_task (
    url varchar(255) NOT NULL UNIQUE,
    idx int NOT NULL,
    manga_id varchar(255) NOT NULL,
    chapter_name varchar(255) NOT NULL
)
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

-- +goose StatementEnd
