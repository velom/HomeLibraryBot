-- +goose Up
-- +goose StatementBegin
ALTER TABLE books ADD COLUMN labels Array(String);
-- +goose StatementEnd
-- +goose StatementBegin
ALTER TABLE books MODIFY SETTING enable_block_number_column = 1, enable_block_offset_column = 1;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE books DROP COLUMN labels;
-- +goose StatementEnd
-- +goose StatementBegin
ALTER TABLE books MODIFY SETTING enable_block_number_column = 0, enable_block_offset_column = 0;
-- +goose StatementEnd
