package auth

import (
	"database/sql"
	"fmt"
	"net/http"
	"sync"
	"time"
)

const SessionCookieName = "ledger_session"
const CSRFCookieName = "ledger_csrf"
const SessionDuration = 30 * 24 * time.Hour // 30 days

type Session struct {
	Token     string
	UserID    int64
	ExpiresAt time.Time
}

func CreateSession(db *sql.DB, userID int64) (*Session, error) {
	token, err := GenerateRandomToken(32)
	if err != nil {
		return nil, fmt.Errorf("failed to generate session token: %w", err)
	}

	expiresAt := time.Now().Add(SessionDuration)
	query := "INSERT INTO sessions (token, user_id, expires_at) VALUES (?, ?, ?)"
	if _, err := db.Exec(query, token, userID, expiresAt); err != nil {
		return nil, fmt.Errorf("failed to store session: %w", err)
	}

	return &Session{
		Token:     token,
		UserID:    userID,
		ExpiresAt: expiresAt,
	}, nil
}

func GetSession(db *sql.DB, token string) (*Session, error) {
	var s Session
	query := "SELECT token, user_id, expires_at FROM sessions WHERE token = ? AND expires_at > CURRENT_TIMESTAMP"
	err := db.QueryRow(query, token).Scan(&s.Token, &s.UserID, &s.ExpiresAt)
	if err != nil {
		return nil, err
	}
	return &s, nil
}

func DeleteSession(db *sql.DB, token string) error {
	_, err := db.Exec("DELETE FROM sessions WHERE token = ?", token)
	return err
}

func CleanExpiredSessions(db *sql.DB) error {
	_, err := db.Exec("DELETE FROM sessions WHERE expires_at <= CURRENT_TIMESTAMP")
	return err
}

// In-memory rate limiter for login brute force prevention
type RateLimiter struct {
	mu       sync.Mutex
	attempts map[string][]time.Time
	maxLimit int
	window   time.Duration
}

func NewRateLimiter(maxLimit int, window time.Duration) *RateLimiter {
	return &RateLimiter{
		attempts: make(map[string][]time.Time),
		maxLimit: maxLimit,
		window:   window,
	}
}

func (rl *RateLimiter) Allow(key string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	threshold := now.Add(-rl.window)

	// Filter out old attempts
	var valid []time.Time
	for _, t := range rl.attempts[key] {
		if t.After(threshold) {
			valid = append(valid, t)
		}
	}

	if len(valid) >= rl.maxLimit {
		rl.attempts[key] = valid
		return false
	}

	return true
}

func (rl *RateLimiter) RecordFailure(key string) {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	rl.attempts[key] = append(rl.attempts[key], time.Now())
}

func (rl *RateLimiter) Reset(key string) {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	delete(rl.attempts, key)
}

func SetSessionCookie(w http.ResponseWriter, token string, expiresAt time.Time) {
	http.SetCookie(w, &http.Cookie{
		Name:     SessionCookieName,
		Value:    token,
		Path:     "/",
		Expires:  expiresAt,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   false, // Set to true if running strictly under HTTPS; default false allows ZeroTier HTTP
	})
}

func ClearSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     SessionCookieName,
		Value:    "",
		Path:     "/",
		Expires:  time.Unix(0, 0),
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   false,
	})
}
