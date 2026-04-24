package db

import (
	"database/sql"
	"fmt"
	"strings"

	_ "github.com/mattn/go-sqlite3"
)

type DB struct {
	conn *sql.DB
}

func New(dsn string) (*DB, error) {
	conn, err := sql.Open("sqlite3", dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Test connection
	if err := conn.Ping(); err != nil {
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	return &DB{conn: conn}, nil
}

func (d *DB) Close() error {
	return d.conn.Close()
}

func (d *DB) Conn() *sql.DB {
	return d.conn
}

// CreateSchema creates all required tables
func (d *DB) CreateSchema() error {
	statements := []string{
		`CREATE TABLE IF NOT EXISTS users (
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
		)`,
		`CREATE TABLE IF NOT EXISTS workout_sessions (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			user_id INTEGER NOT NULL,
			workout_name TEXT NOT NULL,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			finished_at DATETIME,
			notes TEXT,
			FOREIGN KEY (user_id) REFERENCES users(id)
		)`,
		`CREATE TABLE IF NOT EXISTS exercise_sets (
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
		)`,
		`CREATE TABLE IF NOT EXISTS standalone_workouts (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			user_id INTEGER NOT NULL,
			title TEXT,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			notes TEXT,
			FOREIGN KEY (user_id) REFERENCES users(id)
		)`,
		`CREATE TABLE IF NOT EXISTS standalone_workout_items (
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
		)`,
		`CREATE TABLE IF NOT EXISTS standalone_workout_item_sets (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			workout_item_id INTEGER NOT NULL,
			position INTEGER NOT NULL,
			target_reps INTEGER NOT NULL,
			weight REAL NOT NULL,
			FOREIGN KEY (workout_item_id) REFERENCES standalone_workout_items(id) ON DELETE CASCADE
		)`,
		`CREATE INDEX IF NOT EXISTS idx_standalone_workouts_user_id ON standalone_workouts(user_id)`,
		`CREATE INDEX IF NOT EXISTS idx_standalone_workout_items_workout_id ON standalone_workout_items(workout_id)`,
		`CREATE INDEX IF NOT EXISTS idx_standalone_workout_item_sets_item_id ON standalone_workout_item_sets(workout_item_id)`,
		`CREATE TABLE IF NOT EXISTS lift_progress (
			user_id INTEGER NOT NULL,
			exercise_name TEXT NOT NULL,
			current_weight REAL NOT NULL,
			increment_by REAL NOT NULL,
			fail_streak INTEGER DEFAULT 0,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			PRIMARY KEY (user_id, exercise_name),
			FOREIGN KEY (user_id) REFERENCES users(id)
		)`,
		`CREATE TABLE IF NOT EXISTS progression_overrides (
			user_id INTEGER NOT NULL,
			exercise_name TEXT NOT NULL,
			skip_next_increment INTEGER DEFAULT 0,
			PRIMARY KEY (user_id, exercise_name),
			FOREIGN KEY (user_id) REFERENCES users(id)
		)`,
		`CREATE TABLE IF NOT EXISTS body_metrics (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			user_id INTEGER NOT NULL,
			recorded_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			weight REAL NOT NULL,
			weight_unit TEXT DEFAULT 'kg',
			notes TEXT,
			FOREIGN KEY (user_id) REFERENCES users(id)
		)`,
		`CREATE TABLE IF NOT EXISTS user_sessions (
			session_id TEXT PRIMARY KEY,
			user_id INTEGER NOT NULL,
			expires_at DATETIME NOT NULL,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (user_id) REFERENCES users(id)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_user_sessions_user_id ON user_sessions(user_id)`,
		`CREATE INDEX IF NOT EXISTS idx_user_sessions_expires_at ON user_sessions(expires_at
		)`,
	}

	for _, stmt := range statements {
		if _, err := d.conn.Exec(stmt); err != nil {
			return fmt.Errorf("failed to execute schema statement: %w", err)
		}
	}

	// Backward compatibility: existing DBs may not have deleted_at yet.
	if _, err := d.conn.Exec(`ALTER TABLE users ADD COLUMN deleted_at DATETIME`); err != nil {
		if !strings.Contains(strings.ToLower(err.Error()), "duplicate column name") {
			return fmt.Errorf("failed to ensure users.deleted_at column: %w", err)
		}
	}
	if _, err := d.conn.Exec(`ALTER TABLE users ADD COLUMN distance_unit_pref TEXT DEFAULT 'mi'`); err != nil {
		if !strings.Contains(strings.ToLower(err.Error()), "duplicate column name") {
			return fmt.Errorf("failed to ensure users.distance_unit_pref column: %w", err)
		}
	}
	if _, err := d.conn.Exec(`ALTER TABLE users ADD COLUMN theme_pref TEXT DEFAULT 'light'`); err != nil {
		if !strings.Contains(strings.ToLower(err.Error()), "duplicate column name") {
			return fmt.Errorf("failed to ensure users.theme_pref column: %w", err)
		}
	}
	if _, err := d.conn.Exec(`ALTER TABLE users ADD COLUMN keep_awake INTEGER DEFAULT 1`); err != nil {
		if !strings.Contains(strings.ToLower(err.Error()), "duplicate column name") {
			return fmt.Errorf("failed to ensure users.keep_awake column: %w", err)
		}
	}
	if _, err := d.conn.Exec(`ALTER TABLE standalone_workouts ADD COLUMN title TEXT`); err != nil {
		if !strings.Contains(strings.ToLower(err.Error()), "duplicate column name") {
			return fmt.Errorf("failed to ensure standalone_workouts.title column: %w", err)
		}
	}

	if _, err := d.conn.Exec(`UPDATE users SET unit_pref = 'lb_in' WHERE unit_pref IS NULL OR unit_pref = ''`); err != nil {
		return fmt.Errorf("failed to migrate default unit preferences: %w", err)
	}
	if _, err := d.conn.Exec(`UPDATE users SET unit_pref = 'kg_cm' WHERE lower(unit_pref) = 'kg' OR lower(unit_pref) = 'metric'`); err != nil {
		return fmt.Errorf("failed to migrate metric unit preferences: %w", err)
	}
	if _, err := d.conn.Exec(`UPDATE users SET unit_pref = 'lb_in' WHERE lower(unit_pref) = 'lb' OR lower(unit_pref) = 'imperial'`); err != nil {
		return fmt.Errorf("failed to migrate imperial unit preferences: %w", err)
	}
	if _, err := d.conn.Exec(`UPDATE users SET distance_unit_pref = 'mi' WHERE distance_unit_pref IS NULL OR distance_unit_pref = ''`); err != nil {
		return fmt.Errorf("failed to migrate default distance unit preferences: %w", err)
	}
	if _, err := d.conn.Exec(`UPDATE users SET distance_unit_pref = 'km' WHERE lower(distance_unit_pref) IN ('km', 'kilometer', 'kilometers', 'kilometre', 'kilometres')`); err != nil {
		return fmt.Errorf("failed to migrate metric distance unit preferences: %w", err)
	}
	if _, err := d.conn.Exec(`UPDATE users SET distance_unit_pref = 'mi' WHERE lower(distance_unit_pref) IN ('mi', 'mile', 'miles')`); err != nil {
		return fmt.Errorf("failed to migrate imperial distance unit preferences: %w", err)
	}
	if _, err := d.conn.Exec(`UPDATE users SET distance_unit_pref = CASE WHEN unit_pref = 'kg_cm' THEN 'km' ELSE 'mi' END WHERE lower(distance_unit_pref) NOT IN ('mi', 'km')`); err != nil {
		return fmt.Errorf("failed to normalize distance unit preferences: %w", err)
	}
	if _, err := d.conn.Exec(`UPDATE users SET theme_pref = 'light' WHERE theme_pref IS NULL OR theme_pref = ''`); err != nil {
		return fmt.Errorf("failed to migrate default theme preferences: %w", err)
	}
	if _, err := d.conn.Exec(`UPDATE users SET theme_pref = lower(theme_pref)`); err != nil {
		return fmt.Errorf("failed to normalize theme preference casing: %w", err)
	}
	if _, err := d.conn.Exec(`UPDATE users SET theme_pref = 'dark_hc' WHERE theme_pref IN ('dark-hc', 'darkhc', 'high-contrast', 'high_contrast')`); err != nil {
		return fmt.Errorf("failed to normalize high contrast theme preference: %w", err)
	}
	if _, err := d.conn.Exec(`UPDATE users SET theme_pref = 'peachpuff' WHERE theme_pref IN ('peach', 'vim-peachpuff')`); err != nil {
		return fmt.Errorf("failed to normalize peachpuff theme preference: %w", err)
	}
	if _, err := d.conn.Exec(`UPDATE users SET theme_pref = 'default' WHERE theme_pref IN ('vim', 'vimdefault')`); err != nil {
		return fmt.Errorf("failed to normalize default vim theme preference: %w", err)
	}
	if _, err := d.conn.Exec(`UPDATE users SET theme_pref = 'light' WHERE theme_pref NOT IN ('light', 'dark', 'forest', 'sunset', 'peachpuff', 'dark_hc', 'blue', 'darkblue', 'default', 'delek', 'desert', 'elflord', 'evening', 'habamax', 'industry', 'koehler', 'lunaperche', 'morning', 'murphy', 'pablo', 'quiet', 'retrobox', 'ron', 'shine', 'slate', 'sorbet', 'torte', 'wildcharm', 'zaibatsu', 'zellner')`); err != nil {
		return fmt.Errorf("failed to normalize theme preference values: %w", err)
	}
	if _, err := d.conn.Exec(`UPDATE users SET keep_awake = 1 WHERE keep_awake IS NULL`); err != nil {
		return fmt.Errorf("failed to migrate keep awake preferences: %w", err)
	}
	if _, err := d.conn.Exec(`UPDATE users SET keep_awake = CASE WHEN keep_awake IN (0, 1) THEN keep_awake ELSE 1 END`); err != nil {
		return fmt.Errorf("failed to normalize keep awake preferences: %w", err)
	}

	return nil
}
