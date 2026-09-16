package handlers

import (
	"database/sql"
	"net/http"
	"strings"

	"ledger/internal/auth"
)

type AuthHandler struct {
	db       *sql.DB
	renderer *Renderer
	limiter  *auth.RateLimiter
}

func NewAuthHandler(db *sql.DB, renderer *Renderer, limiter *auth.RateLimiter) *AuthHandler {
	return &AuthHandler{
		db:       db,
		renderer: renderer,
		limiter:  limiter,
	}
}

func (h *AuthHandler) LoginView(w http.ResponseWriter, r *http.Request) {
	var count int
	_ = h.db.QueryRow("SELECT COUNT(*) FROM users").Scan(&count)
	if count == 0 {
		http.Redirect(w, r, "/register", http.StatusFound)
		return
	}

	h.renderer.Render(w, "base.html", map[string]any{
		"Title":     "Giriş Yap",
		"ShowNav":   false,
		"CSRFToken": GetCSRFToken(r),
	})
}

func (h *AuthHandler) LoginAction(w http.ResponseWriter, r *http.Request) {
	ip := strings.Split(r.RemoteAddr, ":")[0]
	if !h.limiter.Allow(ip) {
		h.renderer.Render(w, "base.html", map[string]any{
			"Title":     "Giriş Yap",
			"ShowNav":   false,
			"CSRFToken": GetCSRFToken(r),
			"Error":     "Çok fazla başarısız giriş denemesi. Lütfen 5 dakika bekleyin.",
		})
		return
	}

	username := strings.TrimSpace(r.FormValue("username"))
	password := r.FormValue("password")

	var userID int64
	var hash string
	err := h.db.QueryRow("SELECT id, password_hash FROM users WHERE username = ?", username).Scan(&userID, &hash)
	if err != nil {
		h.limiter.RecordFailure(ip)
		h.renderer.Render(w, "base.html", map[string]any{
			"Title":     "Giriş Yap",
			"ShowNav":   false,
			"CSRFToken": GetCSRFToken(r),
			"Error":     "Geçersiz kullanıcı adı veya şifre.",
		})
		return
	}

	valid, _ := auth.VerifyPassword(password, hash)
	if !valid {
		h.limiter.RecordFailure(ip)
		h.renderer.Render(w, "base.html", map[string]any{
			"Title":     "Giriş Yap",
			"ShowNav":   false,
			"CSRFToken": GetCSRFToken(r),
			"Error":     "Geçersiz kullanıcı adı veya şifre.",
		})
		return
	}

	h.limiter.Reset(ip)
	session, err := auth.CreateSession(h.db, userID)
	if err != nil {
		http.Error(w, "Oturum oluşturulamadı: "+err.Error(), http.StatusInternalServerError)
		return
	}

	auth.SetSessionCookie(w, session.Token, session.ExpiresAt)
	http.Redirect(w, r, "/dashboard", http.StatusFound)
}

func (h *AuthHandler) RegisterView(w http.ResponseWriter, r *http.Request) {
	var count int
	_ = h.db.QueryRow("SELECT COUNT(*) FROM users").Scan(&count)
	if count > 0 {
		http.Redirect(w, r, "/login", http.StatusFound)
		return
	}

	h.renderer.Render(w, "base.html", map[string]any{
		"Title":     "İlk Kurulum — Yönetici Hesabı",
		"ShowNav":   false,
		"CSRFToken": GetCSRFToken(r),
	})
}

func (h *AuthHandler) RegisterAction(w http.ResponseWriter, r *http.Request) {
	var count int
	_ = h.db.QueryRow("SELECT COUNT(*) FROM users").Scan(&count)
	if count > 0 {
		http.Redirect(w, r, "/login", http.StatusFound)
		return
	}

	username := strings.TrimSpace(r.FormValue("username"))
	password := r.FormValue("password")
	confirm := r.FormValue("password_confirm")

	if username == "" || len(password) < 6 {
		h.renderer.Render(w, "base.html", map[string]any{
			"Title":     "İlk Kurulum — Yönetici Hesabı",
			"ShowNav":   false,
			"CSRFToken": GetCSRFToken(r),
			"Error":     "Şifre en az 6 karakter olmalıdır.",
		})
		return
	}

	if password != confirm {
		h.renderer.Render(w, "base.html", map[string]any{
			"Title":     "İlk Kurulum — Yönetici Hesabı",
			"ShowNav":   false,
			"CSRFToken": GetCSRFToken(r),
			"Error":     "Girdiğin şifreler birbiriyle eşleşmiyor.",
		})
		return
	}

	hash, err := auth.HashPassword(password)
	if err != nil {
		http.Error(w, "Şifre hashlenemedi: "+err.Error(), http.StatusInternalServerError)
		return
	}

	res, err := h.db.Exec("INSERT INTO users (username, password_hash) VALUES (?, ?)", username, hash)
	if err != nil {
		http.Error(w, "Kullanıcı oluşturulamadı: "+err.Error(), http.StatusInternalServerError)
		return
	}

	userID, _ := res.LastInsertId()
	// Initialize default settings for user
	_, _ = h.db.Exec("INSERT INTO settings (user_id, setup_completed, setup_step) VALUES (?, 0, 1)", userID)

	session, err := auth.CreateSession(h.db, userID)
	if err != nil {
		http.Error(w, "Oturum oluşturulamadı: "+err.Error(), http.StatusInternalServerError)
		return
	}

	auth.SetSessionCookie(w, session.Token, session.ExpiresAt)
	http.Redirect(w, r, "/setup", http.StatusFound)
}

func (h *AuthHandler) Logout(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie(auth.SessionCookieName)
	if err == nil && cookie.Value != "" {
		_ = auth.DeleteSession(h.db, cookie.Value)
	}
	auth.ClearSessionCookie(w)
	http.Redirect(w, r, "/login", http.StatusFound)
}
