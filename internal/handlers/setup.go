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

type SetupHandler struct {
	db       *sql.DB
	renderer *Renderer
	finance  *services.FinanceService
	market   *market.Service
	snapshot *services.SnapshotService
}

func NewSetupHandler(db *sql.DB, renderer *Renderer, f *services.FinanceService, m *market.Service, s *services.SnapshotService) *SetupHandler {
	return &SetupHandler{
		db:       db,
		renderer: renderer,
		finance:  f,
		market:   m,
		snapshot: s,
	}
}

func (h *SetupHandler) SetupView(w http.ResponseWriter, r *http.Request) {
	userID := GetUserID(r)
	ctx := r.Context()

	var settings models.Settings
	var targetStr, invStr string
	err := h.db.QueryRow(`
		SELECT id, user_id, setup_completed, setup_step, base_currency, emergency_fund_target, 
		       emergency_fund_asset_type, emergency_fund_asset_id, monthly_investment_target 
		FROM settings WHERE user_id = ?
	`, userID).Scan(
		&settings.ID, &settings.UserID, &settings.SetupCompleted, &settings.SetupStep,
		&settings.BaseCurrency, &targetStr, &settings.EmergencyFundAssetType,
		&settings.EmergencyFundAssetID, &invStr,
	)
	if err != nil {
		http.Error(w, "Ayarlar yüklenemedi", http.StatusInternalServerError)
		return
	}
	settings.EmergencyFundTarget, _ = decimal.NewFromString(targetStr)
	settings.MonthlyInvestmentTarget, _ = decimal.NewFromString(invStr)

	step := settings.SetupStep
	if qStep := r.URL.Query().Get("step"); qStep != "" {
		if s, err := strconv.Atoi(qStep); err == nil && s >= 1 && s <= 12 {
			step = s
		}
	}

	progressPct := int(float64(step) / 11.0 * 100)
	if progressPct > 100 {
		progressPct = 100
	}

	data := map[string]any{
		"Title":       fmt.Sprintf("Kurulum — Adım %d", step),
		"ShowNav":     false,
		"CSRFToken":   GetCSRFToken(r),
		"Step":        step,
		"ProgressPct": progressPct,
		"Settings":    settings,
	}

	// Step-specific data gathering
	switch step {
	case 1:
		h.renderer.Render(w, "setup_step1", data)
	case 2:
		h.renderer.Render(w, "setup_step2", data)
	case 3:
		accounts, _, _, _ := h.finance.GetUserAccounts(ctx, settings.BaseCurrency)
		var bankAndCash []models.Account
		for _, a := range accounts {
			if a.AccountType == "bank" || a.AccountType == "cash" {
				bankAndCash = append(bankAndCash, a)
			}
		}
		data["Accounts"] = bankAndCash
		h.renderer.Render(w, "setup_step3", data)
	case 4:
		accounts, _, _, _ := h.finance.GetUserAccounts(ctx, settings.BaseCurrency)
		var foreign []models.Account
		for _, a := range accounts {
			if a.AccountType == "foreign_currency" || a.Currency != settings.BaseCurrency {
				foreign = append(foreign, a)
			}
		}
		data["ForeignAccounts"] = foreign

		// Fetch preview FX rates
		usdRate, _ := h.market.GetFXRate(ctx, "USD", "TRY")
		eurRate, _ := h.market.GetFXRate(ctx, "EUR", "TRY")
		gbpRate, _ := h.market.GetFXRate(ctx, "GBP", "TRY")
		data["USDRate"] = usdRate.Rate.StringFixed(4)
		data["EURRate"] = eurRate.Rate.StringFixed(4)
		data["GBPRate"] = gbpRate.Rate.StringFixed(4)

		h.renderer.Render(w, "setup_step4", data)
	case 5:
		debts, _, _, _ := h.finance.GetUserDebts(ctx)
		data["Debts"] = debts
		h.renderer.Render(w, "setup_step5", data)
	case 6:
		h.renderer.Render(w, "setup_step6", data)
	case 7:
		h.renderer.Render(w, "setup_step7", data)
	case 8:
		h.renderer.Render(w, "setup_step8", data)
	case 9:
		positions, _, _ := h.finance.GetUserPositions(ctx, settings.BaseCurrency)
		data["Positions"] = positions
		h.renderer.Render(w, "setup_step9", data)
	case 10:
		accounts, _, _, _ := h.finance.GetUserAccounts(ctx, settings.BaseCurrency)
		positions, _, _ := h.finance.GetUserPositions(ctx, settings.BaseCurrency)
		data["Accounts"] = accounts
		data["Positions"] = positions
		h.renderer.Render(w, "setup_step10", data)
	case 11:
		accounts, cashBank, foreign, _ := h.finance.GetUserAccounts(ctx, settings.BaseCurrency)
		_, currentDebt, _, _ := h.finance.GetUserDebts(ctx)
		_, investBase, _ := h.finance.GetUserPositions(ctx, settings.BaseCurrency)

		totalAssets := cashBank.Add(foreign).Add(investBase)
		netWorth := totalAssets.Sub(currentDebt)

		data["Summary"] = services.FinancialSummary{
			TotalAssetsBase:     totalAssets,
			TotalDebtBase:       currentDebt,
			NetWorthBase:        netWorth,
			CashAndBankBase:     cashBank,
			ForeignCurrencyBase: foreign,
			InvestmentsBase:     investBase,
		}
		data["Accounts"] = accounts
		h.renderer.Render(w, "setup_step11", data)
	default:
		h.renderer.Render(w, "setup_step1", data)
	}
}

func (h *SetupHandler) NextStep(w http.ResponseWriter, r *http.Request) {
	userID := GetUserID(r)
	currentStep, _ := strconv.Atoi(r.FormValue("step"))
	action := r.FormValue("action")

	// Save data if not skipped
	if action != "skip" {
		switch currentStep {
		case 2:
			baseCurr := CleanString(r.FormValue("base_currency"))
			if baseCurr == "" {
				baseCurr = "TRY"
			}
			emergTarget := CleanString(r.FormValue("emergency_target"))
			invTarget := CleanString(r.FormValue("monthly_investment_target"))
			_, _ = h.db.Exec(`UPDATE settings SET base_currency = ?, emergency_fund_target = ?, monthly_investment_target = ? WHERE user_id = ?`,
				baseCurr, emergTarget, invTarget, userID)
		case 10:
			binding := CleanString(r.FormValue("emergency_fund_binding"))
			parts := strings.Split(binding, ":")
			if len(parts) == 2 {
				assetType := parts[0]
				assetID, _ := strconv.ParseInt(parts[1], 10, 64)
				_, _ = h.db.Exec(`UPDATE settings SET emergency_fund_asset_type = ?, emergency_fund_asset_id = ? WHERE user_id = ?`,
					assetType, assetID, userID)
			}
		}
	}

	nextStep := currentStep + 1
	if nextStep > 11 {
		nextStep = 11
	}

	// Update current progress in database
	_, _ = h.db.Exec(`UPDATE settings SET setup_step = ? WHERE user_id = ?`, nextStep, userID)
	http.Redirect(w, r, fmt.Sprintf("/setup?step=%d", nextStep), http.StatusFound)
}

func (h *SetupHandler) AddAccount(w http.ResponseWriter, r *http.Request) {
	name := CleanString(r.FormValue("name"))
	inst := CleanString(r.FormValue("institution"))
	accType := CleanString(r.FormValue("account_type"))
	bal := CleanString(r.FormValue("balance"))

	if name != "" && bal != "" {
		_, _ = h.db.Exec(`INSERT INTO accounts (name, institution, account_type, currency, balance) VALUES (?, ?, ?, 'TRY', ?)`,
			name, inst, accType, bal)
	}
	http.Redirect(w, r, "/setup?step=3", http.StatusFound)
}

func (h *SetupHandler) AddForeignCurrency(w http.ResponseWriter, r *http.Request) {
	curr := CleanString(r.FormValue("currency"))
	bal := CleanString(r.FormValue("balance"))

	if curr != "" && bal != "" {
		name := fmt.Sprintf("%s Varlığı", curr)
		_, _ = h.db.Exec(`INSERT INTO accounts (name, institution, account_type, currency, balance) VALUES (?, 'Nakit / Banka', 'foreign_currency', ?, ?)`,
			name, curr, bal)
	}
	http.Redirect(w, r, "/setup?step=4", http.StatusFound)
}

func (h *SetupHandler) AddDebt(w http.ResponseWriter, r *http.Request) {
	name := CleanString(r.FormValue("name"))
	bank := CleanString(r.FormValue("bank"))
	currDebt := CleanString(r.FormValue("current_debt"))
	stmtDebt := CleanString(r.FormValue("statement_debt"))
	dueDay, _ := strconv.Atoi(r.FormValue("due_day"))
	debtType := CleanString(r.FormValue("debt_type"))

	if name != "" && currDebt != "" {
		if stmtDebt == "" {
			stmtDebt = "0.00"
		}
		_, _ = h.db.Exec(`INSERT INTO debts (name, debt_type, bank, current_debt, statement_debt, due_day) VALUES (?, ?, ?, ?, ?, ?)`,
			name, debtType, bank, currDebt, stmtDebt, dueDay)
	}
	http.Redirect(w, r, "/setup?step=5", http.StatusFound)
}

func (h *SetupHandler) SaveIncome(w http.ResponseWriter, r *http.Request) {
	items := []struct {
		Enabled string
		Title   string
		Amount  string
		Day     string
	}{
		{r.FormValue("income_maas_enabled"), "Maaş", r.FormValue("income_maas_amount"), r.FormValue("income_maas_day")},
		{r.FormValue("income_burs_enabled"), "Burs", r.FormValue("income_burs_amount"), r.FormValue("income_burs_day")},
		{r.FormValue("income_freelance_enabled"), "Freelance", r.FormValue("income_freelance_amount"), r.FormValue("income_freelance_day")},
		{r.FormValue("income_aile_enabled"), "Aile Desteği", r.FormValue("income_aile_amount"), r.FormValue("income_aile_day")},
	}

	for _, item := range items {
		if item.Enabled == "1" && item.Amount != "" {
			day, _ := strconv.Atoi(item.Day)
			if day <= 0 || day > 31 {
				day = 1
			}
			_, _ = h.db.Exec(`INSERT INTO recurring_items (title, type, amount, day_of_month, frequency, auto_realize) VALUES (?, 'income', ?, ?, 'monthly', 0)`,
				item.Title, item.Amount, day)
		}
	}

	userID := GetUserID(r)
	_, _ = h.db.Exec(`UPDATE settings SET setup_step = 7 WHERE user_id = ?`, userID)
	http.Redirect(w, r, "/setup?step=7", http.StatusFound)
}

func (h *SetupHandler) SaveExpenses(w http.ResponseWriter, r *http.Request) {
	items := []struct {
		Enabled string
		Title   string
		Amount  string
		Day     string
	}{
		{r.FormValue("exp_kira_enabled"), "Ev Kirası", r.FormValue("exp_kira_amount"), r.FormValue("exp_kira_day")},
		{r.FormValue("exp_aidat_enabled"), "Aidat", r.FormValue("exp_aidat_amount"), r.FormValue("exp_aidat_day")},
		{r.FormValue("exp_faturalar_enabled"), "Faturalar", r.FormValue("exp_faturalar_amount"), r.FormValue("exp_faturalar_day")},
		{r.FormValue("exp_internet_enabled"), "Ev İnterneti", r.FormValue("exp_internet_amount"), r.FormValue("exp_internet_day")},
		{r.FormValue("exp_telefon_enabled"), "Cep Telefonu", r.FormValue("exp_telefon_amount"), r.FormValue("exp_telefon_day")},
		{r.FormValue("exp_market_enabled"), "Market / Yemek", r.FormValue("exp_market_amount"), r.FormValue("exp_market_day")},
	}

	for _, item := range items {
		if item.Enabled == "1" && item.Amount != "" {
			day, _ := strconv.Atoi(item.Day)
			if day <= 0 || day > 31 {
				day = 1
			}
			_, _ = h.db.Exec(`INSERT INTO recurring_items (title, type, amount, day_of_month, frequency, auto_realize) VALUES (?, 'expense', ?, ?, 'monthly', 0)`,
				item.Title, item.Amount, day)
		}
	}

	userID := GetUserID(r)
	_, _ = h.db.Exec(`UPDATE settings SET setup_step = 8 WHERE user_id = ?`, userID)
	http.Redirect(w, r, "/setup?step=8", http.StatusFound)
}

func (h *SetupHandler) SaveSubscriptions(w http.ResponseWriter, r *http.Request) {
	items := []struct {
		Enabled string
		Title   string
		Amount  string
	}{
		{r.FormValue("sub_netflix_enabled"), "Netflix", r.FormValue("sub_netflix_amount")},
		{r.FormValue("sub_spotify_enabled"), "Spotify", r.FormValue("sub_spotify_amount")},
		{r.FormValue("sub_youtube_enabled"), "YouTube Premium", r.FormValue("sub_youtube_amount")},
		{r.FormValue("sub_chatgpt_enabled"), "ChatGPT Plus", r.FormValue("sub_chatgpt_amount")},
		{r.FormValue("sub_prime_enabled"), "Amazon Prime", r.FormValue("sub_prime_amount")},
	}

	for _, item := range items {
		if item.Enabled == "1" && item.Amount != "" {
			_, _ = h.db.Exec(`INSERT INTO recurring_items (title, type, amount, day_of_month, frequency, auto_realize) VALUES (?, 'subscription', ?, 1, 'monthly', 0)`,
				item.Title, item.Amount)
		}
	}

	userID := GetUserID(r)
	_, _ = h.db.Exec(`UPDATE settings SET setup_step = 9 WHERE user_id = ?`, userID)
	http.Redirect(w, r, "/setup?step=9", http.StatusFound)
}

func (h *SetupHandler) AddInvestment(w http.ResponseWriter, r *http.Request) {
	symbol := CleanString(r.FormValue("symbol"))
	name := CleanString(r.FormValue("name"))
	market := CleanString(r.FormValue("market"))
	assetType := CleanString(r.FormValue("asset_type"))
	currency := CleanString(r.FormValue("currency"))
	qty := CleanString(r.FormValue("quantity"))
	avgCost := CleanString(r.FormValue("average_cost"))

	if symbol != "" && qty != "" {
		if currency == "" {
			currency = "TRY"
		}
		if avgCost == "" {
			avgCost = "0.00"
		}

		// Insert or get instrument
		var instID int64
		err := h.db.QueryRow("SELECT id FROM instruments WHERE symbol = ?", symbol).Scan(&instID)
		if err != nil {
			res, err := h.db.Exec(`INSERT INTO instruments (symbol, name, asset_type, market, currency, provider) VALUES (?, ?, ?, ?, ?, ?)`,
				symbol, name, assetType, market, currency, market)
			if err == nil {
				instID, _ = res.LastInsertId()
			}
		}

		if instID > 0 {
			upsertPos := `
			INSERT INTO positions (instrument_id, quantity, average_cost) VALUES (?, ?, ?)
			ON CONFLICT(instrument_id) DO UPDATE SET
				quantity = excluded.quantity,
				average_cost = excluded.average_cost,
				updated_at = CURRENT_TIMESTAMP;
			`
			_, _ = h.db.Exec(upsertPos, instID, qty, avgCost)

			// Fetch live price right away in background to prime cache
			go func() {
				_, _ = h.market.GetQuote(r.Context(), instID, symbol, market, assetType, 30)
			}()
		}
	}

	http.Redirect(w, r, "/setup?step=9", http.StatusFound)
}

func (h *SetupHandler) FinishSetup(w http.ResponseWriter, r *http.Request) {
	userID := GetUserID(r)
	ctx := r.Context()

	// Mark setup_completed = 1
	_, err := h.db.Exec(`UPDATE settings SET setup_completed = 1 WHERE user_id = ?`, userID)
	if err != nil {
		http.Error(w, "Kurulum tamamlanamadı", http.StatusInternalServerError)
		return
	}

	// Create initial snapshot
	var baseCurr string
	_ = h.db.QueryRow("SELECT base_currency FROM settings WHERE user_id = ?", userID).Scan(&baseCurr)
	if baseCurr == "" {
		baseCurr = "TRY"
	}
	_ = h.snapshot.RecordDailySnapshot(ctx, baseCurr)

	http.Redirect(w, r, "/dashboard", http.StatusFound)
}

func (h *SetupHandler) RestartSetup(w http.ResponseWriter, r *http.Request) {
	userID := GetUserID(r)
	// Reset setup_step to 1 without erasing user data
	_, _ = h.db.Exec(`UPDATE settings SET setup_completed = 0, setup_step = 1 WHERE user_id = ?`, userID)
	http.Redirect(w, r, "/setup?step=1", http.StatusFound)
}
