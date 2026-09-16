package handlers

import (
	"embed"
	"fmt"
	"html/template"
	"io"
	"net/http"
	"path/filepath"
	"reflect"
	"strings"
	"time"

	"ledger/internal/auth"
	"ledger/internal/models"

	"github.com/shopspring/decimal"
)

type viewTemplate struct {
	tmpl     *template.Template
	execName string
}

type Renderer struct {
	views map[string]*viewTemplate
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
		"len": func(v any) int {
			if v == nil {
				return 0
			}
			val := reflect.ValueOf(v)
			switch val.Kind() {
			case reflect.Array, reflect.Chan, reflect.Map, reflect.Slice, reflect.String:
				return val.Len()
			default:
				return 0
			}
		},
	}

	// 1. Base layout templates (base.html, nav.html)
	baseTmpl, err := template.New("base").Funcs(funcMap).ParseFS(tmplFS,
		"templates/layouts/base.html",
		"templates/layouts/nav.html",
	)
	if err != nil {
		return nil, fmt.Errorf("failed to parse layout templates: %w", err)
	}

	views := make(map[string]*viewTemplate)

	// Helper to add a full-page view extending base layout
	addPageView := func(name string, files ...string) error {
		cloned, err := baseTmpl.Clone()
		if err != nil {
			return fmt.Errorf("failed to clone base template for %s: %w", name, err)
		}
		t, err := cloned.ParseFS(tmplFS, files...)
		if err != nil {
			return fmt.Errorf("failed to parse %s (%v): %w", name, files, err)
		}
		views[name] = &viewTemplate{
			tmpl:     t,
			execName: "base.html",
		}
		return nil
	}

	// Helper to add a standalone partial template
	addPartialView := func(name, file string) error {
		t, err := template.New(name).Funcs(funcMap).ParseFS(tmplFS, file)
		if err != nil {
			return fmt.Errorf("failed to parse partial %s (%s): %w", name, file, err)
		}
		views[name] = &viewTemplate{
			tmpl:     t,
			execName: name,
		}
		return nil
	}

	// 2. Main Page Views
	if err := addPageView("dashboard", "templates/dashboard/index.html"); err != nil {
		return nil, err
	}
	if err := addPageView("transactions", "templates/transactions/index.html"); err != nil {
		return nil, err
	}
	if err := addPageView("recurring", "templates/recurring/index.html"); err != nil {
		return nil, err
	}
	if err := addPageView("assets", "templates/assets/index.html"); err != nil {
		return nil, err
	}
	if err := addPageView("settings", "templates/settings/index.html"); err != nil {
		return nil, err
	}
	if err := addPageView("login", "templates/auth/login.html"); err != nil {
		return nil, err
	}
	if err := addPageView("register", "templates/auth/register.html"); err != nil {
		return nil, err
	}

	// 3. Setup Wizard steps (1 to 11)
	wizardBase, err := baseTmpl.Clone()
	if err != nil {
		return nil, fmt.Errorf("failed to clone base for wizard: %w", err)
	}
	wizardBase, err = wizardBase.ParseFS(tmplFS, "templates/setup/wizard.html")
	if err != nil {
		return nil, fmt.Errorf("failed to parse wizard base: %w", err)
	}

	stepFiles := map[int]string{
		1:  "templates/setup/step1_welcome.html",
		2:  "templates/setup/step2_preferences.html",
		3:  "templates/setup/step3_accounts.html",
		4:  "templates/setup/step4_currencies.html",
		5:  "templates/setup/step5_debts.html",
		6:  "templates/setup/step6_income.html",
		7:  "templates/setup/step7_expenses.html",
		8:  "templates/setup/step8_subscriptions.html",
		9:  "templates/setup/step9_investments.html",
		10: "templates/setup/step10_emergency.html",
		11: "templates/setup/step11_review.html",
	}

	for step, file := range stepFiles {
		cloned, err := wizardBase.Clone()
		if err != nil {
			return nil, fmt.Errorf("failed to clone wizard for step %d: %w", step, err)
		}
		t, err := cloned.ParseFS(tmplFS, file)
		if err != nil {
			return nil, fmt.Errorf("failed to parse setup step %d (%s): %w", step, file, err)
		}
		vt := &viewTemplate{
			tmpl:     t,
			execName: "base.html",
		}
		views[fmt.Sprintf("setup_step%d", step)] = vt
		views[filepath.Base(file)] = vt
	}

	// 4. Partials & Components
	// Net worth chart (defined inside dashboard/index.html)
	views["networth_chart"] = &viewTemplate{
		tmpl:     views["dashboard"].tmpl,
		execName: "networth_chart",
	}

	// Modals and dynamic dropdowns
	if err := addPartialView("modal.html", "templates/transactions/modal.html"); err != nil {
		return nil, err
	}
	views["transaction_modal"] = views["modal.html"]

	if err := addPartialView("edit_modal.html", "templates/assets/edit_modal.html"); err != nil {
		return nil, err
	}
	views["edit_modal"] = views["edit_modal.html"]

	if err := addPartialView("search_results.html", "templates/assets/search_results.html"); err != nil {
		return nil, err
	}
	views["search_results"] = views["search_results.html"]

	return &Renderer{views: views}, nil
}

func (r *Renderer) Execute(w io.Writer, name string, data any) error {
	targetName := name
	// If caller passed generic "base.html", resolve to specific page from context data
	if targetName == "base.html" {
		if m, ok := data.(map[string]any); ok {
			if nav, ok := m["ActiveNav"].(string); ok && nav != "" {
				targetName = nav
			} else if step, ok := m["Step"]; ok {
				targetName = fmt.Sprintf("setup_step%v", step)
			} else if title, ok := m["Title"].(string); ok {
				if strings.Contains(title, "Giriş") {
					targetName = "login"
				} else if strings.Contains(title, "Kayıt") || strings.Contains(title, "İlk Kurulum") {
					targetName = "register"
				}
			}
		}
	}

	vt, exists := r.views[targetName]
	if !exists {
		// Fallback to name as-is
		vt, exists = r.views[name]
	}
	if !exists {
		return fmt.Errorf("şablon bulunamadı: %s", name)
	}

	return vt.tmpl.ExecuteTemplate(w, vt.execName, data)
}

func (r *Renderer) Render(w http.ResponseWriter, name string, data any) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := r.Execute(w, name, data); err != nil {
		http.Error(w, "Şablon oluşturma hatası: "+err.Error(), http.StatusInternalServerError)
	}
}

// Helper to extract CSRF token from request context
func GetCSRFToken(r *http.Request) string {
	if s := auth.CSRFTokenFromContext(r.Context()); s != "" {
		return s
	}
	cookie, err := r.Cookie(auth.CSRFCookieName)
	if err == nil && cookie.Value != "" {
		return cookie.Value
	}
	return ""
}

// Helper to extract current User ID from request context
func GetUserID(r *http.Request) int64 {
	return auth.UserIDFromContext(r.Context())
}

func CurrentDate() string {
	return time.Now().Format("2006-01-02")
}

func CleanString(s string) string {
	return strings.TrimSpace(s)
}
