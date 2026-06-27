-- +goose Up
-- +goose StatementBegin
UPDATE users SET rest_timer = 180 WHERE rest_timer = 90;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
UPDATE users SET rest_timer = 90 WHERE rest_timer = 180;
-- +goose StatementEnd
