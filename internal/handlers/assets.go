package handlers

import (
	"database/sql"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"ledger/internal/market"
	"ledger/internal/models"
	"ledger/internal/services"

	"github.com/shopspring/decimal"
)

type AssetsHandler struct {
	db       *sql.DB
	renderer *Renderer
	finance  *services.FinanceService
	market   *market.Service
}

func NewAssetsHandler(db *sql.DB, renderer *Renderer, f *services.FinanceService, m *market.Service) *AssetsHandler {
	return &AssetsHandler{
		db:       db,
		renderer: renderer,
		finance:  f,
		market:   m,
	}
}

func (h *AssetsHandler) IndexView(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	baseCurrency := "TRY"

	accounts, cashBank, foreign, _ := h.finance.GetUserAccounts(ctx, baseCurrency)
	debts, currentDebt, _, _ := h.finance.GetUserDebts(ctx)
	positions, investBase, _ := h.finance.GetUserPositions(ctx, baseCurrency)

	totalAssets := cashBank.Add(foreign).Add(investBase)
	netWorth := totalAssets.Sub(currentDebt)

	var cashBankList, foreignList []models.Account
	for _, a := range accounts {
		if a.AccountType == "foreign_currency" || a.Currency != baseCurrency {
			foreignList = append(foreignList, a)
		} else {
			cashBankList = append(cashBankList, a)
		}
	}

	summary := services.FinancialSummary{
		TotalAssetsBase:     totalAssets,
		TotalDebtBase:       currentDebt,
		NetWorthBase:        netWorth,
		CashAndBankBase:     cashBank,
		ForeignCurrencyBase: foreign,
		InvestmentsBase:     investBase,
	}

	h.renderer.Render(w, "assets", map[string]any{
		"Title":            "Varlıklar & Portföy",
		"ActiveNav":        "assets",
		"ShowNav":          true,
		"CSRFToken":        GetCSRFToken(r),
		"Summary":          summary,
		"CashBankAccounts": cashBankList,
		"ForeignAccounts":  foreignList,
		"Positions":        positions,
		"Debts":            debts,
	})
}

func (h *AssetsHandler) RefreshQuotesAction(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Invalidate DB quotes and re-fetch for all active positions
	positions, _, _ := h.finance.GetUserPositions(ctx, "TRY")
	for _, p := range positions {
		_, _ = h.market.GetQuote(ctx, p.Instrument.ID, p.Instrument.Symbol, p.Instrument.Market, p.Instrument.AssetType, 0)
	}

	http.Redirect(w, r, "/assets", http.StatusFound)
}

func (h *AssetsHandler) SearchMarketAction(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")
	results, _ := h.market.SearchInstruments(r.Context(), q)

	h.renderer.Render(w, "search_results.html", map[string]any{
		"Results": results,
	})
}

func (h *AssetsHandler) EditAccountModal(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(r.URL.Path, "/")
	if len(parts) < 4 {
		http.NotFound(w, r)
		return
	}
	id := parts[3]

	var a models.Account
	var balStr string
	err := h.db.QueryRow("SELECT id, name, currency, balance FROM accounts WHERE id = ?", id).Scan(&a.ID, &a.Name, &a.Currency, &balStr)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	a.Balance, _ = decimal.NewFromString(balStr)

	h.renderer.Render(w, "edit_modal.html", map[string]any{
		"Title":     fmt.Sprintf("%s Bakiyesini Düzenle", a.Name),
		"Type":      "account",
		"Account":   a,
		"ActionURL": fmt.Sprintf("/assets/update-account/%d", a.ID),
		"CSRFToken": GetCSRFToken(r),
	})
}

func (h *AssetsHandler) UpdateAccountAction(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(r.URL.Path, "/")
	if len(parts) < 4 {
		http.NotFound(w, r)
		return
	}
	id := parts[3]
	bal := CleanString(r.FormValue("balance"))

	if bal != "" {
		_, _ = h.db.Exec("UPDATE accounts SET balance = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?", bal, id)
	}
	http.Redirect(w, r, "/assets", http.StatusFound)
}

func (h *AssetsHandler) EditDebtModal(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(r.URL.Path, "/")
	if len(parts) < 4 {
		http.NotFound(w, r)
		return
	}
	id := parts[3]

	var d models.Debt
	var currStr, stmtStr string
	err := h.db.QueryRow("SELECT id, name, current_debt, statement_debt FROM debts WHERE id = ?", id).Scan(&d.ID, &d.Name, &currStr, &stmtStr)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	d.CurrentDebt, _ = decimal.NewFromString(currStr)
	d.StatementDebt, _ = decimal.NewFromString(stmtStr)

	h.renderer.Render(w, "edit_modal.html", map[string]any{
		"Title":     fmt.Sprintf("%s Borcunu Güncelle", d.Name),
		"Type":      "debt",
		"Debt":      d,
		"ActionURL": fmt.Sprintf("/assets/update-debt/%d", d.ID),
		"CSRFToken": GetCSRFToken(r),
	})
}

func (h *AssetsHandler) UpdateDebtAction(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(r.URL.Path, "/")
	if len(parts) < 4 {
		http.NotFound(w, r)
		return
	}
	id := parts[3]
	currDebt := CleanString(r.FormValue("current_debt"))
	stmtDebt := CleanString(r.FormValue("statement_debt"))

	if currDebt != "" {
		if stmtDebt == "" {
			stmtDebt = "0.00"
		}
		_, _ = h.db.Exec("UPDATE debts SET current_debt = ?, statement_debt = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?", currDebt, stmtDebt, id)
	}
	http.Redirect(w, r, "/assets", http.StatusFound)
}

func (h *AssetsHandler) EditPositionModal(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(r.URL.Path, "/")
	if len(parts) < 4 {
		http.NotFound(w, r)
		return
	}
	id, _ := strconv.ParseInt(parts[3], 10, 64)

	positions, _, err := h.finance.GetUserPositions(r.Context(), "TRY")
	if err != nil {
		http.NotFound(w, r)
		return
	}

	var found *models.Position
	for _, p := range positions {
		if p.ID == id {
			found = &p
			break
		}
	}

	if found == nil {
		http.NotFound(w, r)
		return
	}

	h.renderer.Render(w, "edit_modal.html", map[string]any{
		"Title":     fmt.Sprintf("%s Varlığını Düzenle", found.Instrument.Symbol),
		"Type":      "position",
		"Position":  *found,
		"ActionURL": fmt.Sprintf("/assets/update-position/%d", found.ID),
		"CSRFToken": GetCSRFToken(r),
	})
}

func (h *AssetsHandler) UpdatePositionAction(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(r.URL.Path, "/")
	if len(parts) < 4 {
		http.NotFound(w, r)
		return
	}
	id := parts[3]
	qty := CleanString(r.FormValue("quantity"))
	avgCost := CleanString(r.FormValue("average_cost"))

	if qty != "" {
		if avgCost == "" {
			avgCost = "0.00"
		}
		_, _ = h.db.Exec("UPDATE positions SET quantity = ?, average_cost = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?", qty, avgCost, id)
	}
	http.Redirect(w, r, "/assets", http.StatusFound)
}
