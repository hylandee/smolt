-- +goose Up

-- +goose StatementBegin
CREATE TABLE IF NOT EXISTS users (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    username TEXT UNIQUE NOT NULL,
    password_hash TEXT NOT NULL,
    unit_pref TEXT DEFAULT 'lb_in',
    distance_unit_pref TEXT DEFAULT 'mi',
    theme_pref TEXT DEFAULT 'light',
    keep_awake INTEGER DEFAULT 1,
    bar_weight REAL DEFAULT 20.0,
    rest_timer INTEGER DEFAULT 90,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    deleted_at DATETIME
);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TABLE IF NOT EXISTS workout_sessions (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id INTEGER NOT NULL,
    workout_name TEXT NOT NULL,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    finished_at DATETIME,
    notes TEXT,
    FOREIGN KEY (user_id) REFERENCES users(id)
);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TABLE IF NOT EXISTS exercise_sets (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    session_id INTEGER NOT NULL,
    exercise_name TEXT NOT NULL,
    set_number INTEGER NOT NULL,
    target_reps INTEGER DEFAULT 5,
    actual_reps INTEGER,
    weight REAL NOT NULL,
    weight_unit TEXT DEFAULT 'kg',
    completed BOOLEAN DEFAULT 0,
    FOREIGN KEY (session_id) REFERENCES workout_sessions(id)
);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TABLE IF NOT EXISTS standalone_workouts (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id INTEGER NOT NULL,
    title TEXT,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    notes TEXT,
    FOREIGN KEY (user_id) REFERENCES users(id)
);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TABLE IF NOT EXISTS standalone_workout_items (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    workout_id INTEGER NOT NULL,
    position INTEGER NOT NULL,
    exercise_name TEXT NOT NULL,
    exercise_type TEXT NOT NULL,
    sets INTEGER,
    target_reps INTEGER,
    weight REAL,
    time_minutes INTEGER,
    distance_miles REAL,
    FOREIGN KEY (workout_id) REFERENCES standalone_workouts(id)
);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TABLE IF NOT EXISTS standalone_workout_item_sets (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    workout_item_id INTEGER NOT NULL,
    position INTEGER NOT NULL,
    target_reps INTEGER NOT NULL,
    weight REAL NOT NULL,
    FOREIGN KEY (workout_item_id) REFERENCES standalone_workout_items(id) ON DELETE CASCADE
);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TABLE IF NOT EXISTS lift_progress (
    user_id INTEGER NOT NULL,
    exercise_name TEXT NOT NULL,
    current_weight REAL NOT NULL,
    increment_by REAL NOT NULL,
    fail_streak INTEGER DEFAULT 0,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (user_id, exercise_name),
    FOREIGN KEY (user_id) REFERENCES users(id)
);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TABLE IF NOT EXISTS progression_overrides (
    user_id INTEGER NOT NULL,
    exercise_name TEXT NOT NULL,
    skip_next_increment INTEGER DEFAULT 0,
    PRIMARY KEY (user_id, exercise_name),
    FOREIGN KEY (user_id) REFERENCES users(id)
);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TABLE IF NOT EXISTS body_metrics (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id INTEGER NOT NULL,
    recorded_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    weight REAL NOT NULL,
    weight_unit TEXT DEFAULT 'kg',
    notes TEXT,
    FOREIGN KEY (user_id) REFERENCES users(id)
);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TABLE IF NOT EXISTS user_sessions (
    session_id TEXT PRIMARY KEY,
    user_id INTEGER NOT NULL,
    expires_at DATETIME NOT NULL,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (user_id) REFERENCES users(id)
);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE INDEX IF NOT EXISTS idx_standalone_workouts_user_id ON standalone_workouts(user_id);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE INDEX IF NOT EXISTS idx_standalone_workout_items_workout_id ON standalone_workout_items(workout_id);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE INDEX IF NOT EXISTS idx_standalone_workout_item_sets_item_id ON standalone_workout_item_sets(workout_item_id);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE INDEX IF NOT EXISTS idx_user_sessions_user_id ON user_sessions(user_id);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE INDEX IF NOT EXISTS idx_user_sessions_expires_at ON user_sessions(expires_at);
-- +goose StatementEnd

-- +goose Down

-- +goose StatementBegin
DROP TABLE IF EXISTS user_sessions;
-- +goose StatementEnd

-- +goose StatementBegin
DROP TABLE IF EXISTS body_metrics;
-- +goose StatementEnd

-- +goose StatementBegin
DROP TABLE IF EXISTS progression_overrides;
-- +goose StatementEnd

-- +goose StatementBegin
DROP TABLE IF EXISTS lift_progress;
-- +goose StatementEnd

-- +goose StatementBegin
DROP TABLE IF EXISTS standalone_workout_item_sets;
-- +goose StatementEnd

-- +goose StatementBegin
DROP TABLE IF EXISTS standalone_workout_items;
-- +goose StatementEnd

-- +goose StatementBegin
DROP TABLE IF EXISTS standalone_workouts;
-- +goose StatementEnd

-- +goose StatementBegin
DROP TABLE IF EXISTS exercise_sets;
-- +goose StatementEnd

-- +goose StatementBegin
DROP TABLE IF EXISTS workout_sessions;
-- +goose StatementEnd

-- +goose StatementBegin
DROP TABLE IF EXISTS users;
-- +goose StatementEnd
