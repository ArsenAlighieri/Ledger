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

	h.renderer.Render(w, "base.html", map[string]any{
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
	parts := strings.Split(r.URL.Path, "/")
	if len(parts) >= 4 {
		id, err := strconv.ParseInt(parts[3], 10, 64)
		if err == nil {
			_ = h.recurring.MarkRealized(r.Context(), id)
		}
	}
	http.Redirect(w, r, "/recurring", http.StatusFound)
}
