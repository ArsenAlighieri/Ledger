package handlers

import (
	"database/sql"
	"net/http"
	"strconv"
	"strings"

	"ledger/internal/models"
	"ledger/internal/services"
)

type RecurringHandler struct {
	db        *sql.DB
	renderer  *Renderer
	recurring *services.RecurringService
}

func NewRecurringHandler(db *sql.DB, renderer *Renderer, r *services.RecurringService) *RecurringHandler {
	return &RecurringHandler{
		db:        db,
		renderer:  renderer,
		recurring: r,
	}
}

func (h *RecurringHandler) IndexView(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	items, err := h.recurring.GetRecurringItems(ctx)
	if err != nil {
		http.Error(w, "Düzenli kalemler yüklenemedi: "+err.Error(), http.StatusInternalServerError)
		return
	}

	var incomes, expenses, subscriptions []models.RecurringItem
	for _, it := range items {
		if it.Type == "income" {
			incomes = append(incomes, it)
		} else if it.Type == "subscription" {
			subscriptions = append(subscriptions, it)
		} else {
			expenses = append(expenses, it)
		}
	}

	h.renderer.Render(w, "recurring", map[string]any{
		"Title":         "Düzenli Ödemeler & Gelirler",
		"ActiveNav":     "recurring",
		"ShowNav":       true,
		"CSRFToken":     GetCSRFToken(r),
		"Incomes":       incomes,
		"Expenses":      expenses,
		"Subscriptions": subscriptions,
	})
}

func (h *RecurringHandler) RealizeAction(w http.ResponseWriter, r *http.Request) {
	if !requirePost(w, r) {
		return
	}
	parts := strings.Split(r.URL.Path, "/")
	if len(parts) >= 4 {
		id, err := strconv.ParseInt(parts[3], 10, 64)
		if err == nil {
			_ = h.recurring.MarkRealized(r.Context(), id)
		}
	}
	http.Redirect(w, r, "/recurring", http.StatusFound)
}

func (h *RecurringHandler) CreateAction(w http.ResponseWriter, r *http.Request) {
	if !requirePost(w, r) {
		return
	}
	title := CleanString(r.FormValue("title"))
	itemType := CleanString(r.FormValue("type"))
	amount := CleanString(r.FormValue("amount"))
	currency := strings.ToUpper(CleanString(r.FormValue("currency")))
	frequency := CleanString(r.FormValue("frequency"))
	day, _ := strconv.Atoi(r.FormValue("day_of_month"))
	if itemType != "income" && itemType != "subscription" {
		itemType = "expense"
	}
	if currency == "" {
		currency = "TRY"
	}
	if frequency != "yearly" {
		frequency = "monthly"
	}
	if day < 1 || day > 31 {
		day = 1
	}
	if title != "" && validPositiveDecimal(amount) {
		_, _ = h.db.Exec(`INSERT INTO recurring_items (title, type, amount, currency, day_of_month, frequency, auto_realize) VALUES (?, ?, ?, ?, ?, ?, ?)`, title, itemType, amount, currency, day, frequency, r.FormValue("auto_realize") == "1")
	}
	h.redirect(w, r)
}

func (h *RecurringHandler) DeleteAction(w http.ResponseWriter, r *http.Request) {
	if !requirePost(w, r) {
		return
	}
	if id := pathID(r.URL.Path); id != "" {
		_, _ = h.db.Exec(`UPDATE recurring_items SET is_active = 0 WHERE id = ?`, id)
	}
	h.redirect(w, r)
}

func (h *RecurringHandler) redirect(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "/recurring", http.StatusFound)
}
