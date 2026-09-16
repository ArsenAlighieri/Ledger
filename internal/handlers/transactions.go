package handlers

import (
	"database/sql"
	"net/http"
	"strings"

	"ledger/internal/models"
	"ledger/internal/services"

	"github.com/shopspring/decimal"
)

type TransactionsHandler struct {
	db       *sql.DB
	renderer *Renderer
	finance  *services.FinanceService
}

func NewTransactionsHandler(db *sql.DB, renderer *Renderer, f *services.FinanceService) *TransactionsHandler {
	return &TransactionsHandler{
		db:       db,
		renderer: renderer,
		finance:  f,
	}
}

func (h *TransactionsHandler) IndexView(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	income, expense, remaining, savingsRate, _, _, _ := h.finance.GetMonthlyCashflow(ctx)
	breakdown, _ := h.finance.GetCategoryBreakdown(ctx)

	// Fetch recent transactions
	query := `
	SELECT t.id, t.type, t.amount, t.category_id, COALESCE(c.name, 'Diğer'), t.date, t.description, t.created_at
	FROM transactions t
	LEFT JOIN categories c ON t.category_id = c.id
	ORDER BY t.date DESC, t.id DESC
	LIMIT 100
	`
	rows, err := h.db.Query(query)
	var transactions []models.Transaction
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var t models.Transaction
			var amtStr string
			var catID sql.NullInt64
			if err := rows.Scan(&t.ID, &t.Type, &amtStr, &catID, &t.CategoryName, &t.Date, &t.Description, &t.CreatedAt); err == nil {
				t.Amount, _ = decimal.NewFromString(amtStr)
				if catID.Valid {
					t.CategoryID = &catID.Int64
				}
				t.Date = models.FormatDateTurkish(t.Date)
				transactions = append(transactions, t)
			}
		}
	}

	h.renderer.Render(w, "base.html", map[string]any{
		"Title":               "İşlemler",
		"ActiveNav":           "transactions",
		"ShowNav":             true,
		"CSRFToken":           GetCSRFToken(r),
		"Transactions":        transactions,
		"CategoriesBreakdown": breakdown,
		"Summary": services.FinancialSummary{
			ThisMonthIncome:    income,
			ThisMonthExpense:   expense,
			ThisMonthRemaining: remaining,
			SavingsRate:        savingsRate,
		},
	})
}

func (h *TransactionsHandler) NewModal(w http.ResponseWriter, r *http.Request) {
	// Categories
	cRows, _ := h.db.Query("SELECT id, name, type FROM categories ORDER BY type, name")
	var expCats, incCats []models.Category
	if cRows != nil {
		defer cRows.Close()
		for cRows.Next() {
			var c models.Category
			if err := cRows.Scan(&c.ID, &c.Name, &c.Type); err == nil {
				if c.Type == "expense" {
					expCats = append(expCats, c)
				} else {
					incCats = append(incCats, c)
				}
			}
		}
	}

	accounts, _, _, _ := h.finance.GetUserAccounts(r.Context(), "TRY")

	h.renderer.Render(w, "modal.html", map[string]any{
		"CSRFToken":         GetCSRFToken(r),
		"TodayDate":         CurrentDate(),
		"ExpenseCategories": expCats,
		"IncomeCategories":  incCats,
		"Accounts":          accounts,
	})
}

func (h *TransactionsHandler) CreateAction(w http.ResponseWriter, r *http.Request) {
	txType := CleanString(r.FormValue("type"))
	amount := CleanString(r.FormValue("amount"))
	categoryID := CleanString(r.FormValue("category_id"))
	date := CleanString(r.FormValue("date"))
	description := CleanString(r.FormValue("description"))
	accountID := CleanString(r.FormValue("account_id"))

	if txType == "" {
		txType = "expense"
	}
	if date == "" {
		date = CurrentDate()
	}

	if amount != "" {
		var accIDVal any = nil
		if accountID != "" {
			accIDVal = accountID
		}

		var catIDVal any = nil
		if categoryID != "" {
			catIDVal = categoryID
		}

		insertSQL := `INSERT INTO transactions (type, amount, category_id, account_id, date, description) VALUES (?, ?, ?, ?, ?, ?)`
		_, _ = h.db.Exec(insertSQL, txType, amount, catIDVal, accIDVal, date, description)

		// If linked to an account, adjust account balance
		if accountID != "" {
			amt, _ := decimal.NewFromString(amount)
			if txType == "expense" {
				_, _ = h.db.Exec(`UPDATE accounts SET balance = CAST(balance AS NUMERIC) - ? WHERE id = ?`, amt.String(), accountID)
			} else {
				_, _ = h.db.Exec(`UPDATE accounts SET balance = CAST(balance AS NUMERIC) + ? WHERE id = ?`, amt.String(), accountID)
			}
		}
	}

	http.Redirect(w, r, "/transactions", http.StatusFound)
}

func (h *TransactionsHandler) DeleteAction(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(r.URL.Path, "/")
	if len(parts) >= 4 {
		id := parts[3]
		_, _ = h.db.Exec("DELETE FROM transactions WHERE id = ?", id)
	}
	http.Redirect(w, r, "/transactions", http.StatusFound)
}
