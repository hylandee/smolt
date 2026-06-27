-- +goose Up
-- +goose StatementBegin
UPDATE lift_progress SET increment_by = 5.0 WHERE exercise_name = 'Deadlift' AND increment_by = 2.5;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
UPDATE lift_progress SET increment_by = 2.5 WHERE exercise_name = 'Deadlift' AND increment_by = 5.0;
-- +goose StatementEnd
