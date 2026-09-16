package services

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"ledger/internal/models"
	"ledger/internal/market"

	"github.com/shopspring/decimal"
)

type RecurringService struct {
	db     *sql.DB
	market *market.Service
}

func NewRecurringService(db *sql.DB, marketService ...*market.Service) *RecurringService {
	service := &RecurringService{db: db}
	if len(marketService) > 0 { service.market = marketService[0] }
	return service
}

// GetRecurringItems retrieves all active recurring items with their realized status for the current month.
func (s *RecurringService) GetRecurringItems(ctx context.Context) ([]models.RecurringItem, error) {
	query := `
	SELECT id, title, type, amount, currency, day_of_month, frequency, auto_realize, is_active, last_realized_at, created_at
	FROM recurring_items
	WHERE is_active = 1
	ORDER BY type, day_of_month ASC
	`
	rows, err := s.db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	now := time.Now()

	var items []models.RecurringItem
	for rows.Next() {
		var it models.RecurringItem
		var amtStr string
		var lastRealized sql.NullString
		if err := rows.Scan(&it.ID, &it.Title, &it.Type, &amtStr, &it.Currency, &it.DayOfMonth, &it.Frequency, &it.AutoRealize, &it.IsActive, &lastRealized, &it.CreatedAt); err != nil {
			return nil, err
		}
		it.Amount, _ = decimal.NewFromString(amtStr)
		if lastRealized.Valid {
			str := lastRealized.String
			it.LastRealizedAt = &str
			it.IsRealizedThisMonth = str == realizationPeriod(it.Frequency, now)
		}

		items = append(items, it)
	}

	return items, nil
}

// MarkRealized records a transaction for the recurring item and marks it realized for the current month.
func (s *RecurringService) MarkRealized(ctx context.Context, itemID int64) error {
	var it models.RecurringItem
	var amtStr string
	var lastRealized sql.NullString
	err := s.db.QueryRow(`SELECT id, title, type, amount, currency, day_of_month, frequency, last_realized_at FROM recurring_items WHERE id = ? AND is_active = 1`, itemID).Scan(&it.ID, &it.Title, &it.Type, &amtStr, &it.Currency, &it.DayOfMonth, &it.Frequency, &lastRealized)
	if err != nil {
		return err
	}
	it.Amount, _ = decimal.NewFromString(amtStr)

	now := time.Now()
	period := realizationPeriod(it.Frequency, now)
	if lastRealized.Valid && lastRealized.String == period { return nil }
	today := now.Format("2006-01-02")

	txType := "expense"
	if it.Type == "income" {
		txType = "income"
	}
	transactionAmount := it.Amount
	desc := fmt.Sprintf("Düzenli: %s", it.Title)
	if it.Currency != "" && it.Currency != "TRY" {
		if s.market == nil { return fmt.Errorf("%s/TRY kuru alınamadı", it.Currency) }
		fx, fxErr := s.market.GetFXRate(ctx, it.Currency, "TRY")
		if fxErr != nil { return fmt.Errorf("%s/TRY kuru alınamadı: %w", it.Currency, fxErr) }
		if fx.Rate.IsZero() { return fmt.Errorf("%s/TRY kuru sıfır döndü", it.Currency) }
		transactionAmount = it.Amount.Mul(fx.Rate)
		desc = fmt.Sprintf("Düzenli: %s (%s %s)", it.Title, it.Amount.String(), it.Currency)
	}

	// Insert into transactions
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// Find default category ID based on title or type
	var categoryID sql.NullInt64
	_ = tx.QueryRow("SELECT id FROM categories WHERE name = ? LIMIT 1", it.Title).Scan(&categoryID)
	if !categoryID.Valid {
		_ = tx.QueryRow("SELECT id FROM categories WHERE type = ? AND name = 'Diğer' LIMIT 1", txType).Scan(&categoryID)
	}

	insertTxSQL := `INSERT INTO transactions (type, amount, category_id, date, description) VALUES (?, ?, ?, ?, ?)`
	if _, err := tx.Exec(insertTxSQL, txType, transactionAmount.String(), categoryID, today, desc); err != nil {
		return err
	}

	// Update last_realized_at on recurring_item
	updateSQL := `UPDATE recurring_items SET last_realized_at = ? WHERE id = ?`
	if _, err := tx.Exec(updateSQL, period, itemID); err != nil {
		return err
	}

	return tx.Commit()
}

// AutoRealizeEligible checks and automatically realizes items configured with auto_realize = 1 on or after their day of month.
func (s *RecurringService) AutoRealizeEligible(ctx context.Context) error {
	now := time.Now()
	currentMonthPrefix := now.Format("2006-01")
	currentYear := now.Format("2006")
	currentDay := now.Day()
	currentMonth := int(now.Month())

	rows, err := s.db.Query(`
		SELECT id FROM recurring_items 
		WHERE is_active = 1 AND auto_realize = 1 
		  AND day_of_month <= ? 
		  AND ((frequency = 'monthly' AND (last_realized_at IS NULL OR last_realized_at != ?))
		    OR (frequency = 'yearly' AND CAST(strftime('%m', created_at) AS INTEGER) = ? AND (last_realized_at IS NULL OR last_realized_at != ?)))
	`, currentDay, currentMonthPrefix, currentMonth, currentYear)
	if err != nil {
		return err
	}
	defer rows.Close()

	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err == nil {
			ids = append(ids, id)
		}
	}

	for _, id := range ids {
		_ = s.MarkRealized(ctx, id)
	}

	return nil
}

func realizationPeriod(frequency string, now time.Time) string {
	if frequency == "yearly" { return now.Format("2006") }
	return now.Format("2006-01")
}
