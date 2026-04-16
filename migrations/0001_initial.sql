-- migrations/0001_initial.sql
-- StrongLifts 5x5 workout tracker schema

CREATE TABLE IF NOT EXISTS workout_sessions (
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    workout_type TEXT    NOT NULL CHECK(workout_type IN ('A', 'B')),
    created_at   TEXT    NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now')),
    notes        TEXT
);

CREATE TABLE IF NOT EXISTS exercise_sets (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    session_id    INTEGER NOT NULL REFERENCES workout_sessions(id) ON DELETE CASCADE,
    exercise_name TEXT    NOT NULL,
    set_number    INTEGER NOT NULL,
    reps          INTEGER NOT NULL,
    weight        REAL    NOT NULL DEFAULT 0.0,
    completed     INTEGER NOT NULL DEFAULT 1   -- stored as 0/1
);

CREATE INDEX IF NOT EXISTS idx_exercise_sets_session ON exercise_sets(session_id);
CREATE INDEX IF NOT EXISTS idx_exercise_sets_exercise ON exercise_sets(exercise_name);

CREATE TABLE IF NOT EXISTS body_metrics (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    recorded_at TEXT    NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now')),
    weight_kg   REAL    NOT NULL,
    notes       TEXT
);

CREATE INDEX IF NOT EXISTS idx_body_metrics_date ON body_metrics(recorded_at);
