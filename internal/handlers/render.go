package handlers

import (
	"embed"
	"html/template"
	"net/http"
	"strings"
	"time"

	"ledger/internal/models"

	"github.com/shopspring/decimal"
)

type Renderer struct {
	templates *template.Template
}

func NewRenderer(tmplFS embed.FS) (*Renderer, error) {
	funcMap := template.FuncMap{
		"formatMoney":    models.FormatMoney,
		"formatQuantity": models.FormatQuantity,
		"formatDate":     models.FormatDateTurkish,
		"timeAgo":        models.TimeAgoTurkish,
		"add": func(a, b int) int {
			return a + b
		},
		"sub": func(a, b int) int {
			return a - b
		},
		"eq": func(a, b any) bool {
			return a == b
		},
		"not": func(b bool) bool {
			return !b
		},
		"isZero": func(d decimal.Decimal) bool {
			return d.IsZero()
		},
		"isNegative": func(d decimal.Decimal) bool {
			return d.IsNegative()
		},
		"isPositive": func(d decimal.Decimal) bool {
			return d.IsPositive()
		},
	}

	tmpl, err := template.New("").Funcs(funcMap).ParseFS(tmplFS,
		"templates/layouts/*.html",
		"templates/auth/*.html",
		"templates/setup/*.html",
		"templates/dashboard/*.html",
		"templates/transactions/*.html",
		"templates/recurring/*.html",
		"templates/assets/*.html",
		"templates/settings/*.html",
	)
	if err != nil {
		return nil, err
	}

	return &Renderer{templates: tmpl}, nil
}

func (r *Renderer) Render(w http.ResponseWriter, name string, data any) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := r.templates.ExecuteTemplate(w, name, data); err != nil {
		http.Error(w, "Şablon oluşturma hatası: "+err.Error(), http.StatusInternalServerError)
	}
}

// Helper to extract CSRF token from request context
func GetCSRFToken(r *http.Request) string {
	if val := r.Context().Value("csrf_token"); val != nil {
		if s, ok := val.(string); ok {
			return s
		}
	}
	cookie, err := r.Cookie("ledger_csrf")
	if err == nil && cookie.Value != "" {
		return cookie.Value
	}
	return ""
}

// Helper to extract current User ID from request context
func GetUserID(r *http.Request) int64 {
	if val := r.Context().Value("user_id"); val != nil {
		if id, ok := val.(int64); ok {
			return id
		}
	}
	return 0
}

func CurrentDate() string {
	return time.Now().Format("2006-01-02")
}

func CleanString(s string) string {
	return strings.TrimSpace(s)
}
