-- +goose Up
-- +goose StatementBegin
DROP TABLE IF EXISTS books;
-- +goose StatementEnd

-- +goose StatementBegin
DROP TABLE IF EXISTS participants;
-- +goose StatementEnd

-- +goose StatementBegin
DROP TABLE IF EXISTS events;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TABLE IF NOT EXISTS books (
    name String,
    author String,
    is_readable Bool
) ENGINE = MergeTree()
ORDER BY name;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TABLE IF NOT EXISTS participants (
    name String,
    is_parent Bool
) ENGINE = MergeTree()
ORDER BY name;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TABLE IF NOT EXISTS events (
    date DateTime,
    book_name String,
    participant_name String
) ENGINE = MergeTree()
ORDER BY date;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS events;
-- +goose StatementEnd

-- +goose StatementBegin
DROP TABLE IF EXISTS participants;
-- +goose StatementEnd

-- +goose StatementBegin
DROP TABLE IF EXISTS books;
-- +goose StatementEnd
