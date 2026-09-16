package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"ledger/internal/database"
	"ledger/internal/market"
	"ledger/internal/models"
	"ledger/internal/services"

	"github.com/shopspring/decimal"
)

func TestEndToEndLedgerLifecycle(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "ledger_e2e_*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	dbPath := filepath.Join(tempDir, "ledger.db")
	backupDir := filepath.Join(tempDir, "backups")

	// 1. Initialize Database
	db, err := database.OpenDB(dbPath)
	if err != nil {
		t.Fatalf("open db failed: %v", err)
	}
	defer db.Close()

	if err := database.Migrate(db); err != nil {
		t.Fatalf("migration failed: %v", err)
	}

	// 2. Setup Services
	marketSvc := market.NewService(db)
	financeSvc := services.NewFinanceService(db, marketSvc)
	cfoSvc := services.NewCFOService(financeSvc)
	snapshotSvc := services.NewSnapshotService(db, financeSvc)
	recurringSvc := services.NewRecurringService(db)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	// 3. User Registration & Setup Settings
	res, err := db.Exec(`INSERT INTO users (username, password_hash) VALUES ('admin', 'hash')`)
	if err != nil {
		t.Fatalf("create user failed: %v", err)
	}
	userID, _ := res.LastInsertId()

	_, err = db.Exec(`INSERT INTO settings (user_id, setup_completed, setup_step, base_currency, emergency_fund_target) VALUES (?, 0, 1, 'TRY', '50000.00')`, userID)
	if err != nil {
		t.Fatalf("create settings failed: %v", err)
	}

	// 4. Setup Steps Data Input:
	// A. Bank and Cash (15,000 TRY bank + 3,000 TRY cash)
	_, _ = db.Exec(`INSERT INTO accounts (name, institution, account_type, currency, balance) VALUES ('Ziraat', 'Ziraat', 'bank', 'TRY', '15000.00')`)
	_, _ = db.Exec(`INSERT INTO accounts (name, institution, account_type, currency, balance) VALUES ('Cüzdan Nakit', 'Nakit', 'cash', 'TRY', '3000.00')`)

	// B. Foreign Currency (500 USD)
	_, _ = db.Exec(`INSERT INTO accounts (name, institution, account_type, currency, balance) VALUES ('USD Varlığı', 'Döviz', 'foreign_currency', 'USD', '500.00')`)

	// C. Debts (Credit Card: current 32,022.17, statement 7,944.11)
	_, _ = db.Exec(`INSERT INTO debts (name, debt_type, bank, current_debt, statement_debt, due_day) VALUES ('Ziraat Kart', 'credit_card', 'Ziraat', '32022.17', '7944.11', 15)`)

	// D. Recurring Income & Expenses
	_, _ = db.Exec(`INSERT INTO recurring_items (title, type, amount, day_of_month) VALUES ('Maaş', 'income', '45000.00', 1)`)
	_, _ = db.Exec(`INSERT INTO recurring_items (title, type, amount, day_of_month) VALUES ('Kira', 'expense', '21000.00', 1)`)
	_, _ = db.Exec(`INSERT INTO recurring_items (title, type, amount, day_of_month) VALUES ('Netflix', 'subscription', '299.00', 1)`)

	// E. Investments:
	// 3 AAPL (US Stock), 100 THYAO (BIST), 5000 units TP2 (TEFAS)
	resAAPL, _ := db.Exec(`INSERT INTO instruments (symbol, name, asset_type, market, currency) VALUES ('AAPL', 'Apple Inc.', 'stock', 'US', 'USD')`)
	aaplID, _ := resAAPL.LastInsertId()
	_, _ = db.Exec(`INSERT INTO positions (instrument_id, quantity, average_cost) VALUES (?, '3', '220.00')`, aaplID)

	resTHY, _ := db.Exec(`INSERT INTO instruments (symbol, name, asset_type, market, currency) VALUES ('THYAO', 'Türk Hava Yolları', 'stock', 'BIST', 'TRY')`)
	thyID, _ := resTHY.LastInsertId()
	_, _ = db.Exec(`INSERT INTO positions (instrument_id, quantity, average_cost) VALUES (?, '100', '280.00')`, thyID)

	resTP2, _ := db.Exec(`INSERT INTO instruments (symbol, name, asset_type, market, currency) VALUES ('TP2', 'Tera Portföy Para Piyasası Fonu', 'fund', 'TEFAS', 'TRY')`)
	tp2ID, _ := resTP2.LastInsertId()
	_, _ = db.Exec(`INSERT INTO positions (instrument_id, quantity, average_cost) VALUES (?, '5000', '2.20')`, tp2ID)

	// Pre-seed quotes to avoid test flakiness on network
	_, _ = db.Exec(`INSERT INTO market_quotes (instrument_id, price, currency, quoted_at) VALUES (?, '250.00', 'USD', CURRENT_TIMESTAMP)`, aaplID)
	_, _ = db.Exec(`INSERT INTO market_quotes (instrument_id, price, currency, quoted_at) VALUES (?, '310.00', 'TRY', CURRENT_TIMESTAMP)`, thyID)
	_, _ = db.Exec(`INSERT INTO market_quotes (instrument_id, price, currency, quoted_at) VALUES (?, '2.24', 'TRY', CURRENT_TIMESTAMP)`, tp2ID)

	// 5. Complete Setup
	_, _ = db.Exec(`UPDATE settings SET setup_completed = 1, setup_step = 12 WHERE user_id = ?`, userID)

	// 6. Verify Financial Calculations
	accounts, cashBank, foreign, err := financeSvc.GetUserAccounts(ctx, "TRY")
	if err != nil {
		t.Fatalf("get accounts failed: %v", err)
	}
	if len(accounts) != 3 {
		t.Errorf("expected 3 accounts, got %d", len(accounts))
	}
	if cashBank.LessThan(decimal.NewFromInt(18000)) {
		t.Errorf("expected cash & bank >= 18000, got %s", cashBank)
	}
	if foreign.IsZero() {
		t.Errorf("expected foreign currency converted to TRY, got zero")
	}

	debts, totalCurrentDebt, totalStmtDebt, err := financeSvc.GetUserDebts(ctx)
	if err != nil || len(debts) != 1 {
		t.Fatalf("get debts failed: %v", err)
	}
	if !totalStmtDebt.Equal(decimal.NewFromFloat(7944.11)) {
		t.Errorf("expected stmt debt 7944.11, got %s", totalStmtDebt)
	}

	positions, totalInvestBase, err := financeSvc.GetUserPositions(ctx, "TRY")
	if err != nil || len(positions) != 3 {
		t.Fatalf("get positions failed: %v, count=%d", err, len(positions))
	}
	if totalInvestBase.IsZero() {
		t.Errorf("expected total investments base to be positive")
	}

	totalAssets := cashBank.Add(foreign).Add(totalInvestBase)
	netWorth := totalAssets.Sub(totalCurrentDebt)

	if netWorth.IsNegative() {
		t.Errorf("net worth should be positive for this test dataset, got %s", netWorth)
	}

	// 7. Verify CFO Evaluation
	var settings models.Settings
	settings.EmergencyFundTarget = decimal.NewFromInt(50000)
	cfoEval := cfoSvc.Evaluate(ctx, settings, accounts, debts, positions, cashBank, foreign, totalStmtDebt, decimal.NewFromInt(45000), decimal.NewFromInt(21000))

	if cfoEval.Advice == "" {
		t.Errorf("expected CFO advice, got empty")
	}
	if !strings.Contains(cfoEval.Advice, "ekstre borcun") {
		t.Errorf("expected CFO advice to prioritize statement debt: %s", cfoEval.Advice)
	}

	// 8. Test Daily Snapshot Generation
	err = snapshotSvc.RecordDailySnapshot(ctx, "TRY")
	if err != nil {
		t.Fatalf("record daily snapshot failed: %v", err)
	}

	history, err := snapshotSvc.GetNetWorthHistory(ctx, "TUMU")
	if err != nil || len(history) != 1 {
		t.Errorf("expected 1 snapshot in history, got %d err=%v", len(history), err)
	}

	// 9. Test Recurring Realization
	recurringItems, err := recurringSvc.GetRecurringItems(ctx)
	if err != nil || len(recurringItems) != 3 {
		t.Fatalf("get recurring items failed: %v", err)
	}

	err = recurringSvc.MarkRealized(ctx, recurringItems[0].ID)
	if err != nil {
		t.Fatalf("mark realized failed: %v", err)
	}

	// Verify a transaction was created from recurring item
	var txCount int
	_ = db.QueryRow("SELECT COUNT(*) FROM transactions").Scan(&txCount)
	if txCount != 1 {
		t.Errorf("expected 1 transaction created, got %d", txCount)
	}

	// 10. Test Safe SQLite Backup Creation
	backupFile, err := database.BackupDatabase(db, backupDir)
	if err != nil {
		t.Fatalf("database backup failed: %v", err)
	}
	if _, err := os.Stat(backupFile); os.IsNotExist(err) {
		t.Errorf("expected backup file %s to exist", backupFile)
	}
}
