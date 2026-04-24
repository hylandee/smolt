package auth

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"golang.org/x/crypto/bcrypt"
)

const bcryptCost = 12

const (
	UnitPrefMetric     = "kg_cm"
	UnitPrefImperial   = "lb_in"
	DistanceUnitMiles  = "mi"
	DistanceUnitKM     = "km"
	ThemePrefLight     = "light"
	ThemePrefDark      = "dark"
	ThemePrefForest    = "forest"
	ThemePrefSunset    = "sunset"
	ThemePrefPeachpuff = "peachpuff"
	ThemePrefDarkHC    = "dark_hc"
)

var (
	ErrUserNotFound       = errors.New("user not found")
	ErrUserExists         = errors.New("user already exists")
	ErrInvalidCredentials = errors.New("invalid credentials")
	ErrInvalidUsername    = errors.New("username must be at least 3 characters")
	ErrInvalidPassword    = errors.New("password must be at least 3 characters")
	ErrPasswordMismatch   = errors.New("passwords do not match")
	ErrPasswordUnchanged  = errors.New("new password must be different from current password")
	ErrUserDeleted        = errors.New("user account is deleted")
)

type AuthService struct {
	db *sql.DB
}

type UserSettings struct {
	UnitPref         string
	DistanceUnitPref string
}

func NewAuthService(db *sql.DB) *AuthService {
	return &AuthService{db: db}
}

func normalizeUsername(username string) string {
	return strings.TrimSpace(username)
}

func normalizeUnitPref(unitPref string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(unitPref)) {
	case "kg_cm", "kg/cm", "metric", "kg":
		return UnitPrefMetric, nil
	case "lb_in", "lb/in", "imperial", "lb":
		return UnitPrefImperial, nil
	default:
		return "", fmt.Errorf("invalid unit preference")
	}
}

func normalizeDistanceUnitPref(distanceUnitPref string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(distanceUnitPref)) {
	case "mi", "mile", "miles":
		return DistanceUnitMiles, nil
	case "km", "kilometer", "kilometers", "kilometre", "kilometres":
		return DistanceUnitKM, nil
	default:
		return "", fmt.Errorf("invalid distance unit preference")
	}
}

func normalizeThemePref(themePref string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(themePref)) {
	case ThemePrefLight:
		return ThemePrefLight, nil
	case ThemePrefDark:
		return ThemePrefDark, nil
	case ThemePrefForest:
		return ThemePrefForest, nil
	case ThemePrefSunset:
		return ThemePrefSunset, nil
	case ThemePrefPeachpuff, "peach", "vim-peachpuff":
		return ThemePrefPeachpuff, nil
	case ThemePrefDarkHC, "dark-hc", "darkhc", "high-contrast", "high_contrast":
		return ThemePrefDarkHC, nil
	case "blue", "darkblue", "default", "delek", "desert", "elflord", "evening", "habamax", "industry", "koehler", "lunaperche", "morning", "murphy", "pablo", "quiet", "retrobox", "ron", "shine", "slate", "sorbet", "torte", "wildcharm", "zaibatsu", "zellner":
		return strings.ToLower(strings.TrimSpace(themePref)), nil
	default:
		return "", fmt.Errorf("invalid theme preference")
	}
}

// RegisterUser creates a new user account
func (a *AuthService) RegisterUser(ctx context.Context, username, password string) (int, error) {
	username = normalizeUsername(username)

	if len(username) < 3 {
		return 0, ErrInvalidUsername
	}
	if len(password) < 3 {
		return 0, ErrInvalidPassword
	}

	// Hash password
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcryptCost)
	if err != nil {
		return 0, fmt.Errorf("failed to hash password: %w", err)
	}

	// Insert user
	result, err := a.db.ExecContext(
		ctx,
		"INSERT INTO users (username, password_hash) VALUES (?, ?)",
		username,
		string(hash),
	)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint failed") {
			return 0, ErrUserExists
		}
		return 0, fmt.Errorf("failed to insert user: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("failed to get user id: %w", err)
	}

	return int(id), nil
}

// GetUser retrieves a user by username
func (a *AuthService) GetUser(ctx context.Context, username string) (*User, error) {
	var user User
	username = normalizeUsername(username)

	err := a.db.QueryRowContext(
		ctx,
		"SELECT id, username, password_hash FROM users WHERE username = ? AND deleted_at IS NULL",
		username,
	).Scan(&user.ID, &user.Username, &user.PasswordHash)

	if err == sql.ErrNoRows {
		return nil, ErrUserNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("failed to query user: %w", err)
	}

	return &user, nil
}

// SoftDeleteUser marks the user as deleted without removing row data.
func (a *AuthService) SoftDeleteUser(ctx context.Context, userID int) error {
	result, err := a.db.ExecContext(
		ctx,
		"UPDATE users SET deleted_at = CURRENT_TIMESTAMP WHERE id = ? AND deleted_at IS NULL",
		userID,
	)
	if err != nil {
		return fmt.Errorf("failed to soft-delete user: %w", err)
	}

	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to read soft-delete result: %w", err)
	}
	if affected == 0 {
		return ErrUserNotFound
	}

	return nil
}

// GetUnitPref returns the user's stored unit preference.
func (a *AuthService) GetUnitPref(ctx context.Context, userID int) (string, error) {
	var unitPref string
	err := a.db.QueryRowContext(
		ctx,
		"SELECT COALESCE(unit_pref, ?) FROM users WHERE id = ? AND deleted_at IS NULL",
		UnitPrefImperial,
		userID,
	).Scan(&unitPref)
	if err == sql.ErrNoRows {
		return "", ErrUserNotFound
	}
	if err != nil {
		return "", fmt.Errorf("failed to query unit preference: %w", err)
	}

	normalized, err := normalizeUnitPref(unitPref)
	if err != nil {
		return UnitPrefImperial, nil
	}
	return normalized, nil
}

// UpdateUnitPref updates the user's measurement preference.
func (a *AuthService) UpdateUnitPref(ctx context.Context, userID int, unitPref string) error {
	normalized, err := normalizeUnitPref(unitPref)
	if err != nil {
		return err
	}

	result, err := a.db.ExecContext(
		ctx,
		"UPDATE users SET unit_pref = ? WHERE id = ? AND deleted_at IS NULL",
		normalized,
		userID,
	)
	if err != nil {
		return fmt.Errorf("failed to update unit preference: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to read update result: %w", err)
	}
	if affected == 0 {
		return ErrUserNotFound
	}

	return nil
}

// GetDistanceUnitPref returns the user's stored distance unit preference.
func (a *AuthService) GetDistanceUnitPref(ctx context.Context, userID int) (string, error) {
	var distanceUnitPref string
	err := a.db.QueryRowContext(
		ctx,
		"SELECT COALESCE(distance_unit_pref, ?) FROM users WHERE id = ? AND deleted_at IS NULL",
		DistanceUnitMiles,
		userID,
	).Scan(&distanceUnitPref)
	if err == sql.ErrNoRows {
		return "", ErrUserNotFound
	}
	if err != nil {
		return "", fmt.Errorf("failed to query distance unit preference: %w", err)
	}

	normalized, err := normalizeDistanceUnitPref(distanceUnitPref)
	if err != nil {
		return DistanceUnitMiles, nil
	}
	return normalized, nil
}

// UpdateDistanceUnitPref updates the user's distance unit preference.
func (a *AuthService) UpdateDistanceUnitPref(ctx context.Context, userID int, distanceUnitPref string) error {
	normalized, err := normalizeDistanceUnitPref(distanceUnitPref)
	if err != nil {
		return err
	}

	result, err := a.db.ExecContext(
		ctx,
		"UPDATE users SET distance_unit_pref = ? WHERE id = ? AND deleted_at IS NULL",
		normalized,
		userID,
	)
	if err != nil {
		return fmt.Errorf("failed to update distance unit preference: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to read update result: %w", err)
	}
	if affected == 0 {
		return ErrUserNotFound
	}

	return nil
}

// GetThemePref returns the user's stored theme preference.
func (a *AuthService) GetThemePref(ctx context.Context, userID int) (string, error) {
	var themePref string
	err := a.db.QueryRowContext(
		ctx,
		"SELECT COALESCE(theme_pref, ?) FROM users WHERE id = ? AND deleted_at IS NULL",
		ThemePrefLight,
		userID,
	).Scan(&themePref)
	if err == sql.ErrNoRows {
		return "", ErrUserNotFound
	}
	if err != nil {
		return "", fmt.Errorf("failed to query theme preference: %w", err)
	}

	normalized, err := normalizeThemePref(themePref)
	if err != nil {
		return ThemePrefLight, nil
	}
	return normalized, nil
}

// UpdateThemePref updates the user's theme preference.
func (a *AuthService) UpdateThemePref(ctx context.Context, userID int, themePref string) error {
	normalized, err := normalizeThemePref(themePref)
	if err != nil {
		return err
	}

	result, err := a.db.ExecContext(
		ctx,
		"UPDATE users SET theme_pref = ? WHERE id = ? AND deleted_at IS NULL",
		normalized,
		userID,
	)
	if err != nil {
		return fmt.Errorf("failed to update theme preference: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to read update result: %w", err)
	}
	if affected == 0 {
		return ErrUserNotFound
	}

	return nil
}

// ChangePassword updates a user's password after verifying the current password.
func (a *AuthService) ChangePassword(ctx context.Context, userID int, currentPassword, newPassword, confirmPassword string) error {
	if len(newPassword) < 3 {
		return ErrInvalidPassword
	}
	if newPassword != confirmPassword {
		return ErrPasswordMismatch
	}

	var currentHash string
	err := a.db.QueryRowContext(
		ctx,
		"SELECT password_hash FROM users WHERE id = ? AND deleted_at IS NULL",
		userID,
	).Scan(&currentHash)
	if err == sql.ErrNoRows {
		return ErrUserNotFound
	}
	if err != nil {
		return fmt.Errorf("failed to load current password hash: %w", err)
	}
	if a.VerifyPassword(currentPassword, currentHash) != nil {
		return ErrInvalidCredentials
	}
	if a.VerifyPassword(newPassword, currentHash) == nil {
		return ErrPasswordUnchanged
	}

	newHash, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcryptCost)
	if err != nil {
		return fmt.Errorf("failed to hash password: %w", err)
	}

	result, err := a.db.ExecContext(
		ctx,
		"UPDATE users SET password_hash = ? WHERE id = ? AND deleted_at IS NULL",
		string(newHash),
		userID,
	)
	if err != nil {
		return fmt.Errorf("failed to update password: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to read password update result: %w", err)
	}
	if affected == 0 {
		return ErrUserNotFound
	}

	return nil
}

// VerifyPassword checks if the password matches the hash
func (a *AuthService) VerifyPassword(password, hash string) error {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
}

type User struct {
	ID           int
	Username     string
	PasswordHash string
}
