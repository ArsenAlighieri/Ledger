package handlers

import (
	"database/sql"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"net/http"

	"ledger/internal/database"
	"ledger/internal/models"
	"ledger/internal/services"

	"github.com/shopspring/decimal"
)

type SettingsHandler struct {
	db        *sql.DB
	renderer  *Renderer
	finance   *services.FinanceService
	backupDir string
}

func NewSettingsHandler(db *sql.DB, renderer *Renderer, f *services.FinanceService, backupDir string) *SettingsHandler {
	return &SettingsHandler{
		db:        db,
		renderer:  renderer,
		finance:   f,
		backupDir: backupDir,
	}
}

func (h *SettingsHandler) IndexView(w http.ResponseWriter, r *http.Request) {
	userID := GetUserID(r)

	var settings models.Settings
	var targetStr, invStr string
	_ = h.db.QueryRow(`
		SELECT id, user_id, setup_completed, setup_step, base_currency, emergency_fund_target, 
		       emergency_fund_asset_type, emergency_fund_asset_id, monthly_investment_target 
		FROM settings WHERE user_id = ?
	`, userID).Scan(
		&settings.ID, &settings.UserID, &settings.SetupCompleted, &settings.SetupStep,
		&settings.BaseCurrency, &targetStr, &settings.EmergencyFundAssetType,
		&settings.EmergencyFundAssetID, &invStr,
	)
	settings.EmergencyFundTarget, _ = decimal.NewFromString(targetStr)
	settings.MonthlyInvestmentTarget, _ = decimal.NewFromString(invStr)

	backups := database.ListBackups(h.backupDir)

	msg := r.URL.Query().Get("msg")

	h.renderer.Render(w, "base.html", map[string]any{
		"Title":     "Ayarlar",
		"ActiveNav": "settings",
		"ShowNav":   true,
		"CSRFToken": GetCSRFToken(r),
		"Settings":  settings,
		"Backups":   backups,
		"Message":   msg,
	})
}

func (h *SettingsHandler) UpdatePreferencesAction(w http.ResponseWriter, r *http.Request) {
	userID := GetUserID(r)
	emergTarget := CleanString(r.FormValue("emergency_target"))
	invTarget := CleanString(r.FormValue("monthly_investment_target"))

	if emergTarget != "" {
		_, _ = h.db.Exec(`UPDATE settings SET emergency_fund_target = ?, monthly_investment_target = ? WHERE user_id = ?`,
			emergTarget, invTarget, userID)
	}

	http.Redirect(w, r, "/settings?msg=Tercihler+başarıyla+güncellendi", http.StatusFound)
}

func (h *SettingsHandler) BackupAction(w http.ResponseWriter, r *http.Request) {
	_, err := database.BackupDatabase(h.db, h.backupDir)
	if err != nil {
		http.Error(w, "Yedekleme başarısız: "+err.Error(), http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/settings?msg=Yedekleme+başarıyla+oluşturuldu", http.StatusFound)
}

func (h *SettingsHandler) ExportTransactionsCSV(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", "attachment; filename=ledger_transactions.csv")

	writer := csv.NewWriter(w)
	defer writer.Flush()

	_ = writer.Write([]string{"ID", "Tür", "Tutar", "Kategori", "Tarih", "Açıklama"})

	rows, err := h.db.Query(`
		SELECT t.id, t.type, t.amount, COALESCE(c.name, ''), t.date, t.description 
		FROM transactions t LEFT JOIN categories c ON t.category_id = c.id 
		ORDER BY t.date DESC
	`)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var id int64
			var txType, amt, cat, date, desc string
			if err := rows.Scan(&id, &txType, &amt, &cat, &date, &desc); err == nil {
				_ = writer.Write([]string{fmt.Sprintf("%d", id), txType, amt, cat, date, desc})
			}
		}
	}
}

func (h *SettingsHandler) ExportAssetsCSV(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", "attachment; filename=ledger_assets.csv")

	writer := csv.NewWriter(w)
	defer writer.Flush()

	_ = writer.Write([]string{"Grup", "Ad / Sembol", "Kurum / Piyasa", "Bakiye / Adet", "Para Birimi"})

	// Accounts
	aRows, err := h.db.Query("SELECT name, institution, balance, currency FROM accounts WHERE is_archived = 0")
	if err == nil {
		defer aRows.Close()
		for aRows.Next() {
			var name, inst, bal, curr string
			if err := aRows.Scan(&name, &inst, &bal, &curr); err == nil {
				_ = writer.Write([]string{"Nakit & Banka", name, inst, bal, curr})
			}
		}
	}

	// Positions
	pRows, err := h.db.Query(`
		SELECT i.symbol, i.market, p.quantity, i.currency 
		FROM positions p JOIN instruments i ON p.instrument_id = i.id
	`)
	if err == nil {
		defer pRows.Close()
		for pRows.Next() {
			var sym, mkt, qty, curr string
			if err := pRows.Scan(&sym, &mkt, &qty, &curr); err == nil {
				_ = writer.Write([]string{"Yatırım", sym, mkt, qty, curr})
			}
		}
	}

	// Debts
	dRows, err := h.db.Query("SELECT name, bank, current_debt FROM debts")
	if err == nil {
		defer dRows.Close()
		for dRows.Next() {
			var name, bank, debt string
			if err := dRows.Scan(&name, &bank, &debt); err == nil {
				_ = writer.Write([]string{"Borç", name, bank, debt, "TRY"})
			}
		}
	}
}

func (h *SettingsHandler) ExportFullJSON(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Content-Disposition", "attachment; filename=ledger_backup.json")

	accounts, _, _, _ := h.finance.GetUserAccounts(r.Context(), "TRY")
	debts, _, _, _ := h.finance.GetUserDebts(r.Context())
	positions, _, _ := h.finance.GetUserPositions(r.Context(), "TRY")

	payload := map[string]any{
		"app":         "LEDGER",
		"exported_at": CurrentDate(),
		"accounts":    accounts,
		"debts":       debts,
		"positions":   positions,
	}

	_ = json.NewEncoder(w).Encode(payload)
}
