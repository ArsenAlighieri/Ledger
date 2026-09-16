package auth

import (
	"context"
	"crypto/subtle"
	"database/sql"
	"net/http"
	"strings"
	"time"
)

type contextKey string

const UserContextKey = contextKey("user_id")
const CSRFContextKey = contextKey("csrf_token")

// UserIDFromContext extracts authenticated user ID from context supporting both typed key and string key.
func UserIDFromContext(ctx context.Context) int64 {
	if ctx == nil {
		return 0
	}
	if val := ctx.Value(UserContextKey); val != nil {
		if id, ok := val.(int64); ok && id > 0 {
			return id
		}
	}
	if val := ctx.Value("user_id"); val != nil {
		if id, ok := val.(int64); ok && id > 0 {
			return id
		}
	}
	return 0
}

// CSRFTokenFromContext extracts CSRF token from context supporting both typed key and string key.
func CSRFTokenFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	if val := ctx.Value(CSRFContextKey); val != nil {
		if s, ok := val.(string); ok && s != "" {
			return s
		}
	}
	if val := ctx.Value("csrf_token"); val != nil {
		if s, ok := val.(string); ok && s != "" {
			return s
		}
	}
	return ""
}

type Middleware struct {
	DB          *sql.DB
	RateLimiter *RateLimiter
}

func NewMiddleware(db *sql.DB) *Middleware {
	return &Middleware{
		DB:          db,
		RateLimiter: NewRateLimiter(5, 5*time.Minute), // 5 failed attempts per 5 mins
	}
}

// UserCount returns the number of registered users.
func (m *Middleware) UserCount() (int, error) {
	var count int
	err := m.DB.QueryRow("SELECT COUNT(*) FROM users").Scan(&count)
	return count, err
}

// AuthRequired ensures the user is authenticated; otherwise redirects to /login or /register.
func (m *Middleware) AuthRequired(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count, err := m.UserCount()
		if err != nil {
			http.Error(w, "Database error", http.StatusInternalServerError)
			return
		}

		if count == 0 {
			// First-run: redirect to register
			if r.URL.Path != "/register" && !strings.HasPrefix(r.URL.Path, "/static/") {
				http.Redirect(w, r, "/register", http.StatusFound)
				return
			}
			next.ServeHTTP(w, r)
			return
		}

		cookie, err := r.Cookie(SessionCookieName)
		if err != nil || cookie.Value == "" {
			if r.URL.Path != "/login" && r.URL.Path != "/register" && !strings.HasPrefix(r.URL.Path, "/static/") && r.URL.Path != "/health" {
				http.Redirect(w, r, "/login", http.StatusFound)
				return
			}
			next.ServeHTTP(w, r)
			return
		}

		session, err := GetSession(m.DB, cookie.Value)
		if err != nil || session == nil {
			ClearSessionCookie(w)
			if r.URL.Path != "/login" && !strings.HasPrefix(r.URL.Path, "/static/") && r.URL.Path != "/health" {
				http.Redirect(w, r, "/login", http.StatusFound)
				return
			}
			next.ServeHTTP(w, r)
			return
		}

		// Inject User ID into context (both typed key and string key)
		ctx := context.WithValue(r.Context(), UserContextKey, session.UserID)
		ctx = context.WithValue(ctx, "user_id", session.UserID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// SetupCheck ensures if setup is incomplete, user is redirected to /setup.
func (m *Middleware) SetupCheck(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		userID, ok := r.Context().Value(UserContextKey).(int64)
		if !ok || userID == 0 {
			next.ServeHTTP(w, r)
			return
		}

		// Skip setup check for setup routes, logout, health, static files
		path := r.URL.Path
		if strings.HasPrefix(path, "/setup") || path == "/logout" || path == "/health" || strings.HasPrefix(path, "/static/") {
			next.ServeHTTP(w, r)
			return
		}

		var setupCompleted bool
		err := m.DB.QueryRow("SELECT setup_completed FROM settings WHERE user_id = ?", userID).Scan(&setupCompleted)
		if err == sql.ErrNoRows {
			// Initialize default settings if missing
			_, _ = m.DB.Exec("INSERT INTO settings (user_id, setup_completed, setup_step) VALUES (?, 0, 1)", userID)
			http.Redirect(w, r, "/setup", http.StatusFound)
			return
		} else if err != nil {
			http.Error(w, "Database error", http.StatusInternalServerError)
			return
		}

		if !setupCompleted {
			http.Redirect(w, r, "/setup", http.StatusFound)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// CSRFMiddleware manages CSRF tokens via cookie & request headers/forms.
func (m *Middleware) CSRFMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var token string
		cookie, err := r.Cookie(CSRFCookieName)
		if err == nil && cookie.Value != "" {
			token = cookie.Value
		} else {
			token, _ = GenerateRandomToken(24)
			http.SetCookie(w, &http.Cookie{
				Name:     CSRFCookieName,
				Value:    token,
				Path:     "/",
				HttpOnly: false, // accessible to HTMX
				SameSite: http.SameSiteLaxMode,
				Secure:   false,
			})
		}

		ctx := context.WithValue(r.Context(), CSRFContextKey, token)
		ctx = context.WithValue(ctx, "csrf_token", token)

		// On modifying HTTP methods, verify CSRF token
		if r.Method == http.MethodPost || r.Method == http.MethodPut || r.Method == http.MethodDelete {
			// Exceptions for health and static
			if r.URL.Path == "/health" || strings.HasPrefix(r.URL.Path, "/static/") {
				next.ServeHTTP(w, r.WithContext(ctx))
				return
			}

			sentToken := r.Header.Get("X-CSRF-Token")
			if sentToken == "" {
				sentToken = r.FormValue("csrf_token")
			}

			if subtle.ConstantTimeCompare([]byte(token), []byte(sentToken)) != 1 {
				http.Error(w, "Geçersiz veya eksik güvenlik belirteci (CSRF hatası). Lütfen sayfayı yenileyin.", http.StatusForbidden)
				return
			}
		}

		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
