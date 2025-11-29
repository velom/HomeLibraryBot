-- +goose Up
-- Remove author column from books table

-- +goose StatementBegin
ALTER TABLE books DROP COLUMN author;
-- +goose StatementEnd

-- +goose Down
-- Restore author column to books table

-- +goose StatementBegin
ALTER TABLE books ADD COLUMN author String AFTER name;
-- +goose StatementEnd
