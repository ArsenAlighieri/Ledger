package market

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"ledger/internal/database"

	"github.com/shopspring/decimal"
)

func TestMarketCacheAndFallback(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "ledger_market_test_*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	dbPath := filepath.Join(tempDir, "test.db")
	db, err := database.OpenDB(dbPath)
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer db.Close()

	if err := database.Migrate(db); err != nil {
		t.Fatalf("migration failed: %v", err)
	}

	svc := NewService(db)
	ctx := context.Background()

	// Insert instrument
	res, err := db.Exec(`INSERT INTO instruments (symbol, name, asset_type, market, currency) VALUES ('TEST_STOCK', 'Test Corp', 'stock', 'US', 'USD')`)
	if err != nil {
		t.Fatalf("insert instrument failed: %v", err)
	}
	instID, _ := res.LastInsertId()

	// Pre-seed a cached quote in SQLite
	seedPrice := decimal.NewFromFloat(150.75)
	_, err = db.Exec(`
		INSERT INTO market_quotes (instrument_id, price, currency, quoted_at, fetched_at, provider, is_stale)
		VALUES (?, ?, 'USD', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, 'CacheTest', 0)
	`, instID, seedPrice.String())
	if err != nil {
		t.Fatalf("seed quote failed: %v", err)
	}

	// 1. When TTL is valid (e.g. 60 mins), should return cached quote without hitting external network
	quote, err := svc.GetQuote(ctx, instID, "TEST_STOCK", "US", "stock", 60)
	if err != nil {
		t.Fatalf("expected cached quote, got error: %v", err)
	}
	if !quote.Price.Equal(seedPrice) {
		t.Errorf("expected cached price %s, got %s", seedPrice, quote.Price)
	}

	// 2. Offline fallback test: simulate older cached quote with expired TTL and invalid symbol
	// Since "INVALID_SYMBOL_12345" will fail network lookup, it should fall back to cached quote with IsStale = true
	res2, _ := db.Exec(`INSERT INTO instruments (symbol, name, asset_type, market, currency) VALUES ('INVALID_XYZ', 'Invalid Corp', 'stock', 'US', 'USD')`)
	instID2, _ := res2.LastInsertId()

	fallbackPrice := decimal.NewFromFloat(42.50)
	twoDaysAgo := time.Now().Add(-48 * time.Hour)
	_, _ = db.Exec(`
		INSERT INTO market_quotes (instrument_id, price, currency, quoted_at, fetched_at, provider, is_stale)
		VALUES (?, ?, 'USD', ?, ?, 'OldProvider', 0)
	`, instID2, fallbackPrice.String(), twoDaysAgo, twoDaysAgo)

	staleQuote, err := svc.GetQuote(ctx, instID2, "INVALID_XYZ", "US", "stock", 10)
	if err != nil {
		t.Fatalf("expected fallback to stale quote, got error: %v", err)
	}

	if !staleQuote.Price.Equal(fallbackPrice) {
		t.Errorf("expected fallback price %s, got %s", fallbackPrice, staleQuote.Price)
	}
	if !staleQuote.IsStale {
		t.Errorf("expected fallback quote to be flagged as stale")
	}
}
