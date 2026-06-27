package db

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	_ "github.com/mattn/go-sqlite3"
	"github.com/pressly/goose/v3"
)

type DB struct {
	conn *sql.DB
}

func New(dsn string) (*DB, error) {
	sep := "?"
	if strings.Contains(dsn, "?") {
		sep = "&"
	}
	conn, err := sql.Open("sqlite3", dsn+sep+"_foreign_keys=on&_journal_mode=WAL")
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	if err := conn.Ping(); err != nil {
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	conn.SetMaxOpenConns(1)
	conn.SetMaxIdleConns(1)

	return &DB{conn: conn}, nil
}

func (d *DB) Close() error {
	return d.conn.Close()
}

// Migrate runs all pending goose migrations from migrationsDir.
func (d *DB) Migrate(migrationsDir string) error {
	if err := goose.SetDialect("sqlite3"); err != nil {
		return fmt.Errorf("set goose dialect: %w", err)
	}
	if err := goose.Up(d.conn, migrationsDir); err != nil {
		return fmt.Errorf("run migrations: %w", err)
	}
	return nil
}

// WithTx runs fn inside a transaction, committing on success and rolling back on error.
func (d *DB) WithTx(ctx context.Context, fn func(*sql.Tx) error) error {
	tx, err := d.conn.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()
	if err := fn(tx); err != nil {
		return err
	}
	return tx.Commit()
}

func (d *DB) QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row {
	return d.conn.QueryRowContext(ctx, query, args...)
}

func (d *DB) QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	return d.conn.QueryContext(ctx, query, args...)
}

func (d *DB) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	return d.conn.ExecContext(ctx, query, args...)
}

func (d *DB) BeginTx(ctx context.Context, opts *sql.TxOptions) (*sql.Tx, error) {
	return d.conn.BeginTx(ctx, opts)
}

func (d *DB) Exec(query string, args ...any) (sql.Result, error) {
	return d.conn.Exec(query, args...)
}

func (d *DB) QueryRow(query string, args ...any) *sql.Row {
	return d.conn.QueryRow(query, args...)
}

func (d *DB) Query(query string, args ...any) (*sql.Rows, error) {
	return d.conn.Query(query, args...)
}
