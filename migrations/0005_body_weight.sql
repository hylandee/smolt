-- +goose Up
CREATE TABLE body_weight_readings (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    recorded_at DATE NOT NULL,
    weight_kg REAL NOT NULL,
    UNIQUE(user_id, recorded_at)
);

-- +goose Down
DROP TABLE body_weight_readings;
