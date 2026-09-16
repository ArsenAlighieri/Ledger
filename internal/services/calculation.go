package services

import (
	"context"
	"database/sql"
	"time"

	"ledger/internal/market"
	"ledger/internal/models"

	"github.com/shopspring/decimal"
)

type FinancialSummary struct {
	TotalAssetsBase      decimal.Decimal `json:"total_assets_base"`
	TotalDebtBase        decimal.Decimal `json:"total_debt_base"`
	NetWorthBase         decimal.Decimal `json:"net_worth_base"`
	CashAndBankBase      decimal.Decimal `json:"cash_and_bank_base"`
	ForeignCurrencyBase  decimal.Decimal `json:"foreign_currency_base"`
	InvestmentsBase      decimal.Decimal `json:"investments_base"`
	ThisMonthIncome      decimal.Decimal `json:"this_month_income"`
	ThisMonthExpense     decimal.Decimal `json:"this_month_expense"`
	ThisMonthRemaining   decimal.Decimal `json:"this_month_remaining"`
	SavingsRate          decimal.Decimal `json:"savings_rate"`
	LastMonthExpense     decimal.Decimal `json:"last_month_expense"`
	ExpenseChangePct     decimal.Decimal `json:"expense_change_pct"`
	EmergencyFundCurrent decimal.Decimal `json:"emergency_fund_current"`
	EmergencyFundTarget  decimal.Decimal `json:"emergency_fund_target"`
	EmergencyFundPct     decimal.Decimal `json:"emergency_fund_pct"`
	EmergencyFundName    string          `json:"emergency_fund_name"`
	SafeToSpend          decimal.Decimal `json:"safe_to_spend"`
	CFOAdvice            string          `json:"cfo_advice"`
}

type FinanceService struct {
	db     *sql.DB
	market *market.Service
}

func NewFinanceService(db *sql.DB, m *market.Service) *FinanceService {
	return &FinanceService{
		db:     db,
		market: m,
	}
}

// GetUserAccounts retrieves all active accounts and converts balances to base currency (TRY).
func (s *FinanceService) GetUserAccounts(ctx context.Context, baseCurrency string) ([]models.Account, decimal.Decimal, decimal.Decimal, error) {
	rows, err := s.db.Query(`SELECT id, name, institution, account_type, currency, balance, is_archived, created_at, updated_at 
	                         FROM accounts WHERE is_archived = 0 ORDER BY account_type, name`)
	if err != nil {
		return nil, decimal.Zero, decimal.Zero, err
	}
	defer rows.Close()

	var accounts []models.Account
	cashBankTotal := decimal.Zero
	foreignTotal := decimal.Zero

	for rows.Next() {
		var a models.Account
		var balStr string
		if err := rows.Scan(&a.ID, &a.Name, &a.Institution, &a.AccountType, &a.Currency, &balStr, &a.IsArchived, &a.CreatedAt, &a.UpdatedAt); err != nil {
			return nil, decimal.Zero, decimal.Zero, err
		}
		a.Balance, _ = decimal.NewFromString(balStr)

		if a.Currency == baseCurrency {
			a.BalanceInBase = a.Balance
			a.ExchangeRate = decimal.NewFromInt(1)
		} else {
			fx, err := s.market.GetFXRate(ctx, a.Currency, baseCurrency)
			if err == nil && !fx.Rate.IsZero() {
				a.ExchangeRate = fx.Rate
				a.BalanceInBase = a.Balance.Mul(fx.Rate)
			} else {
				a.ExchangeRate = decimal.NewFromInt(1)
				a.BalanceInBase = a.Balance
			}
		}

		if a.AccountType == "foreign_currency" || a.Currency != baseCurrency {
			foreignTotal = foreignTotal.Add(a.BalanceInBase)
		} else {
			cashBankTotal = cashBankTotal.Add(a.BalanceInBase)
		}

		accounts = append(accounts, a)
	}

	return accounts, cashBankTotal, foreignTotal, nil
}

// GetUserDebts retrieves all debts and calculates total debt.
func (s *FinanceService) GetUserDebts(ctx context.Context) ([]models.Debt, decimal.Decimal, decimal.Decimal, error) {
	rows, err := s.db.Query(`SELECT id, name, debt_type, bank, current_debt, statement_debt, due_day, note, created_at, updated_at 
	                         FROM debts ORDER BY debt_type, name`)
	if err != nil {
		return nil, decimal.Zero, decimal.Zero, err
	}
	defer rows.Close()

	var debts []models.Debt
	totalCurrentDebt := decimal.Zero
	totalStatementDebt := decimal.Zero

	for rows.Next() {
		var d models.Debt
		var currStr, stmtStr string
		if err := rows.Scan(&d.ID, &d.Name, &d.DebtType, &d.Bank, &currStr, &stmtStr, &d.DueDay, &d.Note, &d.CreatedAt, &d.UpdatedAt); err != nil {
			return nil, decimal.Zero, decimal.Zero, err
		}
		d.CurrentDebt, _ = decimal.NewFromString(currStr)
		d.StatementDebt, _ = decimal.NewFromString(stmtStr)

		totalCurrentDebt = totalCurrentDebt.Add(d.CurrentDebt)
		totalStatementDebt = totalStatementDebt.Add(d.StatementDebt)
		debts = append(debts, d)
	}

	return debts, totalCurrentDebt, totalStatementDebt, nil
}

// GetUserPositions retrieves and prices all user investment positions.
func (s *FinanceService) GetUserPositions(ctx context.Context, baseCurrency string) ([]models.Position, decimal.Decimal, error) {
	query := `
	SELECT p.id, p.instrument_id, p.quantity, p.average_cost, p.created_at, p.updated_at,
	       i.id, i.symbol, i.name, i.asset_type, i.market, i.currency, i.provider, i.is_manual, i.manual_price
	FROM positions p
	JOIN instruments i ON p.instrument_id = i.id
	WHERE CAST(p.quantity AS NUMERIC) > 0
	ORDER BY p.id ASC
	`
	rows, err := s.db.Query(query)
	if err != nil {
		return nil, decimal.Zero, err
	}
	defer rows.Close()

	var rawPositions []struct {
		pos      models.Position
		manPrice string
	}

	for rows.Next() {
		var p models.Position
		var qtyStr, avgCostStr, manPriceStr string
		if err := rows.Scan(
			&p.ID, &p.InstrumentID, &qtyStr, &avgCostStr, &p.CreatedAt, &p.UpdatedAt,
			&p.Instrument.ID, &p.Instrument.Symbol, &p.Instrument.Name, &p.Instrument.AssetType,
			&p.Instrument.Market, &p.Instrument.Currency, &p.Instrument.Provider, &p.Instrument.IsManual, &manPriceStr,
		); err != nil {
			return nil, decimal.Zero, err
		}

		p.Quantity, _ = decimal.NewFromString(qtyStr)
		p.AverageCost, _ = decimal.NewFromString(avgCostStr)
		p.Instrument.ManualPrice, _ = decimal.NewFromString(manPriceStr)

		rawPositions = append(rawPositions, struct {
			pos      models.Position
			manPrice string
		}{pos: p, manPrice: manPriceStr})
	}
	rows.Close()

	var positions []models.Position
	totalInvestmentsBase := decimal.Zero

	for _, raw := range rawPositions {
		p := raw.pos

		// Get current price
		var price decimal.Decimal
		if p.Instrument.IsManual {
			price = p.Instrument.ManualPrice
			p.Instrument.LatestPrice = price
			p.Instrument.QuotedAt = p.Instrument.UpdatedAt
		} else {
			quote, qErr := s.market.GetQuote(ctx, p.Instrument.ID, p.Instrument.Symbol, p.Instrument.Market, p.Instrument.AssetType, 0)
			if qErr == nil && !quote.Price.IsZero() {
				price = quote.Price
				p.Instrument.LatestPrice = quote.Price
				p.Instrument.QuotedAt = quote.QuotedAt
				p.Instrument.IsStale = quote.IsStale
			} else if !p.Instrument.ManualPrice.IsZero() {
				price = p.Instrument.ManualPrice
				p.Instrument.LatestPrice = price
				p.Instrument.IsStale = true
			}
		}

		p.CurrentPrice = price
		p.MarketValue = p.Quantity.Mul(price)

		// Currency conversion to base currency (TRY)
		if p.Instrument.Currency == baseCurrency {
			p.ValueInBase = p.MarketValue
		} else {
			fx, fErr := s.market.GetFXRate(ctx, p.Instrument.Currency, baseCurrency)
			if fErr == nil && !fx.Rate.IsZero() {
				p.ValueInBase = p.MarketValue.Mul(fx.Rate)
			} else {
				p.ValueInBase = p.MarketValue
			}
		}

		// Cost basis & Profit/Loss if average cost was provided
		if !p.AverageCost.IsZero() {
			p.HasCostBasis = true
			totalCost := p.Quantity.Mul(p.AverageCost)
			p.ProfitLoss = p.MarketValue.Sub(totalCost)
			if !totalCost.IsZero() {
				p.ProfitLossPct = p.ProfitLoss.Div(totalCost).Mul(decimal.NewFromInt(100))
			}
		}

		totalInvestmentsBase = totalInvestmentsBase.Add(p.ValueInBase)
		positions = append(positions, p)
	}

	// Calculate portfolio percentage allocation
	if !totalInvestmentsBase.IsZero() {
		for i := range positions {
			positions[i].PortfolioPct = positions[i].ValueInBase.Div(totalInvestmentsBase).Mul(decimal.NewFromInt(100))
		}
	}

	return positions, totalInvestmentsBase, nil
}

// GetMonthlyCashflow aggregates this month's transactions and compares to previous month.
func (s *FinanceService) GetMonthlyCashflow(ctx context.Context) (income, expense, remaining, savingsRate, lastMonthExpense, expenseChangePct decimal.Decimal, err error) {
	now := time.Now()
	startOfThisMonth := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location()).Format("2006-01-02")
	endOfThisMonth := time.Date(now.Year(), now.Month()+1, 0, 23, 59, 59, 0, now.Location()).Format("2006-01-02")

	lastMonth := now.AddDate(0, -1, 0)
	startOfLastMonth := time.Date(lastMonth.Year(), lastMonth.Month(), 1, 0, 0, 0, 0, now.Location()).Format("2006-01-02")
	endOfLastMonth := time.Date(lastMonth.Year(), lastMonth.Month()+1, 0, 23, 59, 59, 0, now.Location()).Format("2006-01-02")

	// This month's transactions
	rows, err := s.db.Query(`SELECT type, amount FROM transactions WHERE date >= ? AND date <= ?`, startOfThisMonth, endOfThisMonth)
	if err != nil {
		return
	}
	defer rows.Close()

	income = decimal.Zero
	expense = decimal.Zero

	for rows.Next() {
		var txType, amtStr string
		if err = rows.Scan(&txType, &amtStr); err != nil {
			return
		}
		amt, _ := decimal.NewFromString(amtStr)
		if txType == "income" {
			income = income.Add(amt)
		} else if txType == "expense" {
			expense = expense.Add(amt)
		}
	}

	remaining = income.Sub(expense)
	if !income.IsZero() && remaining.IsPositive() {
		savingsRate = remaining.Div(income).Mul(decimal.NewFromInt(100))
	} else {
		savingsRate = decimal.Zero
	}

	// Last month's total expense for comparison
	var lastExpStr sql.NullString
	_ = s.db.QueryRow(`SELECT COALESCE(SUM(CAST(amount AS NUMERIC)), 0) FROM transactions WHERE type = 'expense' AND date >= ? AND date <= ?`, startOfLastMonth, endOfLastMonth).Scan(&lastExpStr)
	if lastExpStr.Valid && lastExpStr.String != "" {
		lastMonthExpense, _ = decimal.NewFromString(lastExpStr.String)
	}

	if !lastMonthExpense.IsZero() {
		diff := expense.Sub(lastMonthExpense)
		expenseChangePct = diff.Div(lastMonthExpense).Mul(decimal.NewFromInt(100))
	}

	return
}

// GetCategoryBreakdown returns expenses grouped by category for this month.
func (s *FinanceService) GetCategoryBreakdown(ctx context.Context) ([]struct {
	CategoryName string
	TotalAmount  decimal.Decimal
	Percentage   decimal.Decimal
}, error) {
	now := time.Now()
	startOfMonth := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location()).Format("2006-01-02")
	endOfMonth := time.Date(now.Year(), now.Month()+1, 0, 23, 59, 59, 0, now.Location()).Format("2006-01-02")

	query := `
	SELECT COALESCE(c.name, 'Diğer'), SUM(CAST(t.amount AS NUMERIC))
	FROM transactions t
	LEFT JOIN categories c ON t.category_id = c.id
	WHERE t.type = 'expense' AND t.date >= ? AND t.date <= ?
	GROUP BY c.id
	ORDER BY SUM(CAST(t.amount AS NUMERIC)) DESC
	`
	rows, err := s.db.Query(query, startOfMonth, endOfMonth)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var totalExpense decimal.Decimal
	var items []struct {
		CategoryName string
		TotalAmount  decimal.Decimal
		Percentage   decimal.Decimal
	}

	for rows.Next() {
		var name string
		var amtStr string
		if err := rows.Scan(&name, &amtStr); err != nil {
			return nil, err
		}
		amt, _ := decimal.NewFromString(amtStr)
		totalExpense = totalExpense.Add(amt)
		items = append(items, struct {
			CategoryName string
			TotalAmount  decimal.Decimal
			Percentage   decimal.Decimal
		}{
			CategoryName: name,
			TotalAmount:  amt,
		})
	}

	if !totalExpense.IsZero() {
		for i := range items {
			items[i].Percentage = items[i].TotalAmount.Div(totalExpense).Mul(decimal.NewFromInt(100))
		}
	}

	return items, nil
}
