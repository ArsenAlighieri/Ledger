package market

import (
	"context"
	"time"

	"github.com/shopspring/decimal"
)

type Quote struct {
	Price    decimal.Decimal `json:"price"`
	Currency string          `json:"currency"`
	QuotedAt time.Time       `json:"quoted_at"`
	Provider string          `json:"provider"`
	IsStale  bool            `json:"is_stale"`
}

type FXRate struct {
	Base     string          `json:"base"`
	Quote    string          `json:"quote"`
	Rate     decimal.Decimal `json:"rate"`
	QuotedAt time.Time       `json:"quoted_at"`
	Provider string          `json:"provider"`
}

type SearchResult struct {
	Symbol    string `json:"symbol"`
	Name      string `json:"name"`
	Market    string `json:"market"`
	AssetType string `json:"asset_type"` // 'stock', 'etf', 'fund', 'gold', 'currency'
	Currency  string `json:"currency"`
}

type QuoteProvider interface {
	GetQuote(ctx context.Context, symbol string, market string) (Quote, error)
}

type FXProvider interface {
	GetFXRate(ctx context.Context, base, quote string) (FXRate, error)
}

type SearchProvider interface {
	Search(ctx context.Context, query string) ([]SearchResult, error)
}
