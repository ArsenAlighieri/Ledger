package market

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/shopspring/decimal"
)

type Service struct {
	db    *sql.DB
	tcmb  *TCMBProvider
	tefas *TEFASProvider
	yahoo *YahooProvider

	mu       sync.RWMutex
	fxCache  map[string]FXRate
	fxExpiry map[string]time.Time
	fxTTL    time.Duration
	quoteTTL int
}

func NewService(db *sql.DB, ttlMinutes ...int) *Service {
	fxMinutes, quoteMinutes := 60, 30
	if len(ttlMinutes) > 0 && ttlMinutes[0] > 0 { fxMinutes = ttlMinutes[0] }
	if len(ttlMinutes) > 1 && ttlMinutes[1] > 0 { quoteMinutes = ttlMinutes[1] }
	return &Service{
		db:      db,
		tcmb:    NewTCMBProvider(),
		tefas:   NewTEFASProvider(),
		yahoo:   NewYahooProvider(),
		fxCache: make(map[string]FXRate),
		fxExpiry: make(map[string]time.Time),
		fxTTL: time.Duration(fxMinutes) * time.Minute,
		quoteTTL: quoteMinutes,
	}
}

// GetFXRate returns the exchange rate from base to quote currency with caching.
func (s *Service) GetFXRate(ctx context.Context, base, quote string) (FXRate, error) {
	base = strings.ToUpper(strings.TrimSpace(base))
	quote = strings.ToUpper(strings.TrimSpace(quote))

	if base == quote {
		return FXRate{
			Base:     base,
			Quote:    quote,
			Rate:     decimal.NewFromInt(1),
			QuotedAt: time.Now(),
			Provider: "IDENTITY",
		}, nil
	}

	pairKey := fmt.Sprintf("%s_%s", base, quote)

	s.mu.RLock()
	cached, ok := s.fxCache[pairKey]
	valid := ok && time.Now().Before(s.fxExpiry[pairKey])
	s.mu.RUnlock()

	if valid {
		return cached, nil
	}

	// Try fetching from TCMB
	rate, err := s.tcmb.GetFXRate(ctx, base, quote)
	if err == nil && !rate.Rate.IsZero() {
		s.mu.Lock()
		s.fxCache[pairKey] = rate
		s.fxExpiry[pairKey] = time.Now().Add(s.fxTTL)
		s.mu.Unlock()
		return rate, nil
	}

	log.Printf("[Market] TCMB FX rate failed for %s/%s: %v. Checking cache...", base, quote, err)

	// Fallback to in-memory cached rate even if expired
	if ok {
		return cached, nil
	}

	// Fallback to Yahoo Finance for FX if TCMB is unreachable
	if quote == "TRY" {
		ySymbol := fmt.Sprintf("%sTRY=X", base)
		yQuote, yErr := s.yahoo.GetStockQuote(ctx, ySymbol, "FX")
		if yErr == nil && !yQuote.Price.IsZero() {
			fx := FXRate{
				Base:     base,
				Quote:    quote,
				Rate:     yQuote.Price,
				QuotedAt: yQuote.QuotedAt,
				Provider: "YahooFX",
			}
			s.mu.Lock()
			s.fxCache[pairKey] = fx
			s.fxExpiry[pairKey] = time.Now().Add(s.fxTTL)
			s.mu.Unlock()
			return fx, nil
		}
	}

	return FXRate{}, fmt.Errorf("unable to obtain FX rate for %s/%s: %w", base, quote, err)
}

// GetQuote gets the price for a specific instrument with DB caching and graceful degradation.
func (s *Service) GetQuote(ctx context.Context, instrumentID int64, symbol, market, assetType string, ttlMinutes int) (Quote, error) {
	symbol = strings.ToUpper(strings.TrimSpace(symbol))
	market = strings.ToUpper(strings.TrimSpace(market))
	assetType = strings.ToLower(strings.TrimSpace(assetType))

	if ttlMinutes <= 0 {
		ttlMinutes = s.quoteTTL
	}

	// 1. Check DB cache first
	var cachedQuote Quote
	var priceStr string
	var quotedAt, fetchedAt time.Time
	var provider string
	var isStale bool

	query := `SELECT price, currency, quoted_at, fetched_at, provider, is_stale 
	          FROM market_quotes WHERE instrument_id = ?`
	err := s.db.QueryRow(query, instrumentID).Scan(&priceStr, &cachedQuote.Currency, &quotedAt, &fetchedAt, &provider, &isStale)
	hasCached := err == nil

	if hasCached {
		cachedQuote.Price, _ = decimal.NewFromString(priceStr)
		cachedQuote.QuotedAt = quotedAt
		cachedQuote.Provider = provider
		cachedQuote.IsStale = isStale

		// If cache is fresh, return it directly
		if time.Since(fetchedAt) < time.Duration(ttlMinutes)*time.Minute {
			return cachedQuote, nil
		}
	}

	// 2. Fetch fresh quote from external provider
	var freshQuote Quote
	var fetchErr error

	if market == "TEFAS" || assetType == "fund" {
		freshQuote, _, fetchErr = s.tefas.GetFundQuote(ctx, symbol)
	} else if assetType == "gold" || symbol == "ALTIN" || symbol == "GRAM_ALTIN" {
		freshQuote, fetchErr = s.getGoldGramPrice(ctx)
	} else {
		freshQuote, fetchErr = s.yahoo.GetStockQuote(ctx, symbol, market)
	}

	if fetchErr != nil {
		log.Printf("[Market] Failed to fetch quote for %s (%s): %v", symbol, market, fetchErr)
		if hasCached {
			// Mark stale in DB and return cached
			_, _ = s.db.Exec("UPDATE market_quotes SET is_stale = 1 WHERE instrument_id = ?", instrumentID)
			cachedQuote.IsStale = true
			return cachedQuote, nil
		}
		return Quote{}, fmt.Errorf("quote fetch failed and no cache available for %s: %w", symbol, fetchErr)
	}

	// 3. Persist fresh quote to DB
	upsertSQL := `
	INSERT INTO market_quotes (instrument_id, price, currency, quoted_at, fetched_at, provider, is_stale)
	VALUES (?, ?, ?, ?, CURRENT_TIMESTAMP, ?, 0)
	ON CONFLICT(instrument_id) DO UPDATE SET
		price = excluded.price,
		currency = excluded.currency,
		quoted_at = excluded.quoted_at,
		fetched_at = CURRENT_TIMESTAMP,
		provider = excluded.provider,
		is_stale = 0;
	`
	_, _ = s.db.Exec(upsertSQL, instrumentID, freshQuote.Price.String(), freshQuote.Currency, freshQuote.QuotedAt, freshQuote.Provider)

	return freshQuote, nil
}

// Calculate Gram Altın price from international gold spot (GC=F) and USD/TRY
func (s *Service) getGoldGramPrice(ctx context.Context) (Quote, error) {
	goldQuote, err := s.yahoo.GetStockQuote(ctx, "GC=F", "COMMODITY")
	if err != nil {
		return Quote{}, fmt.Errorf("failed to fetch gold futures GC=F: %w", err)
	}

	fxRate, err := s.GetFXRate(ctx, "USD", "TRY")
	if err != nil {
		return Quote{}, fmt.Errorf("failed to get USD/TRY for gold calculation: %w", err)
	}

	// 1 troy ounce = 31.1034768 grams
	troyOunceToGrams := decimal.NewFromFloat(31.1034768)
	usdPerGram := goldQuote.Price.Div(troyOunceToGrams)
	tryPerGram := usdPerGram.Mul(fxRate.Rate)

	return Quote{
		Price:    tryPerGram,
		Currency: "TRY",
		QuotedAt: goldQuote.QuotedAt,
		Provider: "Yahoo+TCMB",
		IsStale:  false,
	}, nil
}

// SearchInstruments searches across TEFAS and Yahoo Finance for autocomplete.
func (s *Service) SearchInstruments(ctx context.Context, query string) ([]SearchResult, error) {
	query = strings.TrimSpace(query)
	if len(query) < 2 {
		return nil, nil
	}

	var allResults []SearchResult
	var wg sync.WaitGroup
	var mu sync.Mutex

	// 1. Search TEFAS
	wg.Add(1)
	go func() {
		defer wg.Done()
		tefasResults, err := s.tefas.SearchFunds(ctx, query)
		if err == nil && len(tefasResults) > 0 {
			mu.Lock()
			allResults = append(allResults, tefasResults...)
			mu.Unlock()
		}
	}()

	// 2. Search Yahoo Finance
	wg.Add(1)
	go func() {
		defer wg.Done()
		yahooResults, err := s.yahoo.Search(ctx, query)
		if err == nil && len(yahooResults) > 0 {
			mu.Lock()
			allResults = append(allResults, yahooResults...)
			mu.Unlock()
		}
	}()

	wg.Wait()
	return allResults, nil
}
