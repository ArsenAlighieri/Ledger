package handlers

import (
	"database/sql"
	"errors"
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
	if !requirePost(w, r) {
		return
	}
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

func (h *AssetsHandler) CreateAccountAction(w http.ResponseWriter, r *http.Request) {
	if !requirePost(w, r) {
		return
	}
	name := CleanString(r.FormValue("name"))
	institution := CleanString(r.FormValue("institution"))
	accountType := CleanString(r.FormValue("account_type"))
	currency := strings.ToUpper(CleanString(r.FormValue("currency")))
	balance := CleanString(r.FormValue("balance"))
	if accountType != "cash" && accountType != "foreign_currency" {
		accountType = "bank"
	}
	if currency == "" {
		currency = "TRY"
	}
	if name != "" && validNonNegativeDecimal(balance) {
		_, _ = h.db.Exec(`INSERT INTO accounts (name, institution, account_type, currency, balance) VALUES (?, ?, ?, ?, ?)`, name, institution, accountType, currency, balance)
	}
	h.redirectAssets(w, r)
}

func (h *AssetsHandler) DeleteAccountAction(w http.ResponseWriter, r *http.Request) {
	if !requirePost(w, r) {
		return
	}
	id := pathID(r.URL.Path)
	if id != "" {
		// Archive accounts so historical transactions keep their account relation.
		_, _ = h.db.Exec(`UPDATE accounts SET is_archived = 1, updated_at = CURRENT_TIMESTAMP WHERE id = ?`, id)
		h.clearEmergencyBinding("account", id)
	}
	h.redirectAssets(w, r)
}

func (h *AssetsHandler) CreateDebtAction(w http.ResponseWriter, r *http.Request) {
	if !requirePost(w, r) {
		return
	}
	name := CleanString(r.FormValue("name"))
	debtType := CleanString(r.FormValue("debt_type"))
	bank := CleanString(r.FormValue("bank"))
	current := CleanString(r.FormValue("current_debt"))
	statement := CleanString(r.FormValue("statement_debt"))
	dueDay, _ := strconv.Atoi(r.FormValue("due_day"))
	if debtType != "loan" && debtType != "other" {
		debtType = "credit_card"
	}
	if statement == "" {
		statement = "0"
	}
	if dueDay < 0 || dueDay > 31 {
		dueDay = 0
	}
	if name != "" && validNonNegativeDecimal(current) && validNonNegativeDecimal(statement) {
		_, _ = h.db.Exec(`INSERT INTO debts (name, debt_type, bank, current_debt, statement_debt, due_day) VALUES (?, ?, ?, ?, ?, ?)`, name, debtType, bank, current, statement, dueDay)
	}
	h.redirectAssets(w, r)
}

func (h *AssetsHandler) DeleteDebtAction(w http.ResponseWriter, r *http.Request) {
	if !requirePost(w, r) {
		return
	}
	if id := pathID(r.URL.Path); id != "" {
		_, _ = h.db.Exec(`DELETE FROM debts WHERE id = ?`, id)
	}
	h.redirectAssets(w, r)
}

func (h *AssetsHandler) CreatePositionAction(w http.ResponseWriter, r *http.Request) {
	if !requirePost(w, r) {
		return
	}
	symbol := strings.ToUpper(CleanString(r.FormValue("symbol")))
	name := CleanString(r.FormValue("name"))
	marketName := strings.ToUpper(CleanString(r.FormValue("market")))
	assetType := strings.ToLower(CleanString(r.FormValue("asset_type")))
	currency := strings.ToUpper(CleanString(r.FormValue("currency")))
	quantity := CleanString(r.FormValue("quantity"))
	averageCost := CleanString(r.FormValue("average_cost"))
	manualPrice := CleanString(r.FormValue("manual_price"))
	if currency == "" {
		currency = "TRY"
	}
	if name == "" {
		name = symbol
	}
	if assetType == "" {
		assetType = "other"
	}
	if marketName == "" {
		marketName = "OTHER"
	}
	if averageCost == "" {
		averageCost = "0"
	}
	if manualPrice == "" {
		manualPrice = "0"
	}
	if symbol != "" && validPositiveDecimal(quantity) && validNonNegativeDecimal(averageCost) && validNonNegativeDecimal(manualPrice) {
		isManual := marketName == "OTHER" || r.FormValue("is_manual") == "1"
		tx, err := h.db.Begin()
		if err != nil {
			http.Error(w, "Yatırım eklenemedi: "+err.Error(), http.StatusInternalServerError)
			return
		}
		defer tx.Rollback()
		var instrumentID int64
		err = tx.QueryRow(`SELECT id FROM instruments WHERE symbol = ?`, symbol).Scan(&instrumentID)
		if errors.Is(err, sql.ErrNoRows) {
			res, insertErr := tx.Exec(`INSERT INTO instruments (symbol, name, asset_type, market, currency, provider, is_manual, manual_price) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`, symbol, name, assetType, marketName, currency, marketName, isManual, manualPrice)
			err = insertErr
			if err == nil {
				instrumentID, _ = res.LastInsertId()
			}
		}
		if err == nil {
			_, err = tx.Exec(`INSERT INTO positions (instrument_id, quantity, average_cost) VALUES (?, ?, ?) ON CONFLICT(instrument_id) DO UPDATE SET quantity = excluded.quantity, average_cost = excluded.average_cost, updated_at = CURRENT_TIMESTAMP`, instrumentID, quantity, averageCost)
		}
		if err == nil {
			err = tx.Commit()
		}
		if err != nil {
			http.Error(w, "Yatırım eklenemedi: "+err.Error(), http.StatusInternalServerError)
			return
		}
	}
	h.redirectAssets(w, r)
}

func (h *AssetsHandler) DeletePositionAction(w http.ResponseWriter, r *http.Request) {
	if !requirePost(w, r) {
		return
	}
	id := pathID(r.URL.Path)
	if id != "" {
		_, _ = h.db.Exec(`DELETE FROM positions WHERE id = ?`, id)
		h.clearEmergencyBinding("position", id)
	}
	h.redirectAssets(w, r)
}

func validNonNegativeDecimal(value string) bool {
	d, err := decimal.NewFromString(value)
	return err == nil && !d.IsNegative()
}

func validPositiveDecimal(value string) bool {
	d, err := decimal.NewFromString(value)
	return err == nil && d.IsPositive()
}

func pathID(path string) string {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) == 0 {
		return ""
	}
	if _, err := strconv.ParseInt(parts[len(parts)-1], 10, 64); err != nil {
		return ""
	}
	return parts[len(parts)-1]
}

func (h *AssetsHandler) clearEmergencyBinding(assetType, id string) {
	_, _ = h.db.Exec(`UPDATE settings SET emergency_fund_asset_type = '', emergency_fund_asset_id = 0 WHERE emergency_fund_asset_type = ? AND emergency_fund_asset_id = ?`, assetType, id)
}

func (h *AssetsHandler) redirectAssets(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "/assets", http.StatusFound)
}

func requirePost(w http.ResponseWriter, r *http.Request) bool {
	if r.Method == http.MethodPost {
		return true
	}
	w.Header().Set("Allow", http.MethodPost)
	http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	return false
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
	if !requirePost(w, r) {
		return
	}
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
	if !requirePost(w, r) {
		return
	}
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
	if !requirePost(w, r) {
		return
	}
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
