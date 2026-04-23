package auth

import (
	"database/sql"
	"encoding/hex"
	"fmt"
	"time"

	"crypto/rand"
)

const sessionDuration = 180 * 24 * time.Hour

type UserSession struct {
	UserID   int
	Username string
}

type SessionStore struct {
	db *sql.DB
}

func NewSessionStore(db *sql.DB) *SessionStore {
	return &SessionStore{db: db}
}

func SessionMaxAgeSeconds() int {
	return int(sessionDuration / time.Second)
}

// CleanupExpired removes all expired sessions from storage.
func (s *SessionStore) CleanupExpired() error {
	_, err := s.db.Exec(`DELETE FROM user_sessions WHERE expires_at <= ?`, time.Now())
	if err != nil {
		return fmt.Errorf("cleanup expired sessions: %w", err)
	}
	return nil
}

// Create generates and persists a session, returning its session ID.
func (s *SessionStore) Create(user UserSession) (string, error) {
	sessionID := generateSessionID()
	expiresAt := time.Now().Add(sessionDuration)

	_, err := s.db.Exec(
		`INSERT INTO user_sessions (session_id, user_id, expires_at) VALUES (?, ?, ?)`,
		sessionID,
		user.UserID,
		expiresAt,
	)
	if err != nil {
		return "", fmt.Errorf("create session: %w", err)
	}

	return sessionID, nil
}

// Get retrieves a session by ID if it exists, is unexpired, and belongs to a live user.
func (s *SessionStore) Get(sessionID string) (*UserSession, error) {
	var user UserSession
	var expiresAt time.Time

	err := s.db.QueryRow(
		`SELECT u.id, u.username, us.expires_at
		 FROM user_sessions us
		 JOIN users u ON u.id = us.user_id
		 WHERE us.session_id = ? AND u.deleted_at IS NULL`,
		sessionID,
	).Scan(&user.UserID, &user.Username, &expiresAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get session: %w", err)
	}

	if time.Now().After(expiresAt) {
		if err := s.Delete(sessionID); err != nil {
			return nil, err
		}
		return nil, nil
	}

	return &user, nil
}

// Delete removes a session.
func (s *SessionStore) Delete(sessionID string) error {
	_, err := s.db.Exec(`DELETE FROM user_sessions WHERE session_id = ?`, sessionID)
	if err != nil {
		return fmt.Errorf("delete session: %w", err)
	}
	return nil
}

// DeleteByUserID removes all sessions for a user.
func (s *SessionStore) DeleteByUserID(userID int) error {
	_, err := s.db.Exec(`DELETE FROM user_sessions WHERE user_id = ?`, userID)
	if err != nil {
		return fmt.Errorf("delete user sessions: %w", err)
	}
	return nil
}

func generateSessionID() string {
	b := make([]byte, 32)
	rand.Read(b)
	return hex.EncodeToString(b)
}
