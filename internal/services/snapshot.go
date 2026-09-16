package services

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"ledger/internal/models"

	"github.com/shopspring/decimal"
)

type SnapshotService struct {
	db      *sql.DB
	finance *FinanceService
}

func NewSnapshotService(db *sql.DB, f *FinanceService) *SnapshotService {
	return &SnapshotService{db: db, finance: f}
}

// RecordDailySnapshot generates and saves today's financial snapshot.
func (s *SnapshotService) RecordDailySnapshot(ctx context.Context, baseCurrency string) error {
	today := time.Now().Format("2006-01-02")

	// Calculate totals
	_, cashBankBase, foreignBase, err := s.finance.GetUserAccounts(ctx, baseCurrency)
	if err != nil {
		return err
	}
	cashBase := cashBankBase.Add(foreignBase)

	_, currentDebt, _, err := s.finance.GetUserDebts(ctx)
	if err != nil {
		return err
	}

	_, investmentsBase, err := s.finance.GetUserPositions(ctx, baseCurrency)
	if err != nil {
		return err
	}

	totalAssets := cashBase.Add(investmentsBase)
	netWorth := totalAssets.Sub(currentDebt)

	query := `
	INSERT INTO portfolio_snapshots (snapshot_date, total_assets_base, investments_base, cash_base, debt_base, net_worth_base)
	VALUES (?, ?, ?, ?, ?, ?)
	ON CONFLICT(snapshot_date) DO UPDATE SET
		total_assets_base = excluded.total_assets_base,
		investments_base = excluded.investments_base,
		cash_base = excluded.cash_base,
		debt_base = excluded.debt_base,
		net_worth_base = excluded.net_worth_base;
	`
	_, err = s.db.Exec(query, today, totalAssets.String(), investmentsBase.String(), cashBase.String(), currentDebt.String(), netWorth.String())
	return err
}

type NetWorthPoint struct {
	Date     string          `json:"date"`
	NetWorth decimal.Decimal `json:"net_worth"`
}

// GetNetWorthHistory retrieves snapshots for historical net worth chart.
func (s *SnapshotService) GetNetWorthHistory(ctx context.Context, rangeCode string) ([]NetWorthPoint, error) {
	now := time.Now()
	var startDate string

	switch rangeCode {
	case "1H": // 1 week
		startDate = now.AddDate(0, 0, -7).Format("2006-01-02")
	case "1A": // 1 month
		startDate = now.AddDate(0, -1, 0).Format("2006-01-02")
	case "3A": // 3 months
		startDate = now.AddDate(0, -3, 0).Format("2006-01-02")
	case "6A": // 6 months
		startDate = now.AddDate(0, -6, 0).Format("2006-01-02")
	case "1Y": // 1 year
		startDate = now.AddDate(-1, 0, 0).Format("2006-01-02")
	default: // All
		startDate = "2000-01-01"
	}

	query := `
	SELECT snapshot_date, net_worth_base FROM portfolio_snapshots 
	WHERE snapshot_date >= ? 
	ORDER BY snapshot_date ASC
	`
	rows, err := s.db.Query(query, startDate)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var points []NetWorthPoint
	for rows.Next() {
		var d, nwStr string
		if err := rows.Scan(&d, &nwStr); err != nil {
			return nil, err
		}
		nw, _ := decimal.NewFromString(nwStr)
		points = append(points, NetWorthPoint{
			Date:     models.FormatDateTurkish(d),
			NetWorth: nw,
		})
	}

	return points, nil
}

type MonthlyCashflowBar struct {
	MonthName string          `json:"month_name"`
	Income    decimal.Decimal `json:"income"`
	Expense   decimal.Decimal `json:"expense"`
}

// GetMonthlyCashflowBars returns the last 6 months of income vs expense cashflow.
func (s *SnapshotService) GetMonthlyCashflowBars(ctx context.Context) ([]MonthlyCashflowBar, error) {
	now := time.Now()
	var bars []MonthlyCashflowBar

	turkishMonths := []string{"Oca", "Şub", "Mar", "Nis", "May", "Haz", "Tem", "Ağu", "Eyl", "Eki", "Kas", "Ara"}

	for i := 5; i >= 0; i-- {
		t := now.AddDate(0, -i, 0)
		startStr := time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, t.Location()).Format("2006-01-02")
		endStr := time.Date(t.Year(), t.Month()+1, 0, 23, 59, 59, 0, t.Location()).Format("2006-01-02")
		monthName := fmt.Sprintf("%s %d", turkishMonths[t.Month()-1], t.Year()%100)

		var incStr, expStr sql.NullString
		_ = s.db.QueryRow(`SELECT SUM(CAST(amount AS NUMERIC)) FROM transactions WHERE type = 'income' AND date >= ? AND date <= ?`, startStr, endStr).Scan(&incStr)
		_ = s.db.QueryRow(`SELECT SUM(CAST(amount AS NUMERIC)) FROM transactions WHERE type = 'expense' AND date >= ? AND date <= ?`, startStr, endStr).Scan(&expStr)

		inc := decimal.Zero
		exp := decimal.Zero
		if incStr.Valid && incStr.String != "" {
			inc, _ = decimal.NewFromString(incStr.String)
		}
		if expStr.Valid && expStr.String != "" {
			exp, _ = decimal.NewFromString(expStr.String)
		}

		bars = append(bars, MonthlyCashflowBar{
			MonthName: monthName,
			Income:    inc,
			Expense:   exp,
		})
	}

	return bars, nil
}
