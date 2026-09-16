package models

import (
	"fmt"
	"strings"
	"time"

	"github.com/shopspring/decimal"
)

type User struct {
	ID           int64     `json:"id"`
	Username     string    `json:"username"`
	PasswordHash string    `json:"-"`
	CreatedAt    time.Time `json:"created_at"`
}

type Settings struct {
	ID                      int64           `json:"id"`
	UserID                  int64           `json:"user_id"`
	SetupCompleted          bool            `json:"setup_completed"`
	SetupStep               int             `json:"setup_step"`
	BaseCurrency            string          `json:"base_currency"`
	EmergencyFundTarget     decimal.Decimal `json:"emergency_fund_target"`
	EmergencyFundAssetType  string          `json:"emergency_fund_asset_type"` // 'account' or 'position'
	EmergencyFundAssetID    int64           `json:"emergency_fund_asset_id"`
	MonthlyInvestmentTarget decimal.Decimal `json:"monthly_investment_target"`
	FxTTLMinutes            int             `json:"fx_ttl_minutes"`
	StockTTLMinutes         int             `json:"stock_ttl_minutes"`
	UpdatedAt               time.Time       `json:"updated_at"`
}

type Account struct {
	ID          int64           `json:"id"`
	Name        string          `json:"name"`
	Institution string          `json:"institution"`
	AccountType string          `json:"account_type"` // 'bank', 'cash', 'foreign_currency'
	Currency    string          `json:"currency"`
	Balance     decimal.Decimal `json:"balance"`
	IsArchived  bool            `json:"is_archived"`
	CreatedAt   time.Time       `json:"created_at"`
	UpdatedAt   time.Time       `json:"updated_at"`

	// Derived / UI fields
	BalanceInBase decimal.Decimal `json:"balance_in_base,omitempty"`
	ExchangeRate  decimal.Decimal `json:"exchange_rate,omitempty"`
}

type Debt struct {
	ID            int64           `json:"id"`
	Name          string          `json:"name"`
	DebtType      string          `json:"debt_type"` // 'credit_card', 'loan', 'other'
	Bank          string          `json:"bank"`
	CurrentDebt   decimal.Decimal `json:"current_debt"`
	StatementDebt decimal.Decimal `json:"statement_debt"`
	DueDay        int             `json:"due_day"`
	Note          string          `json:"note"`
	CreatedAt     time.Time       `json:"created_at"`
	UpdatedAt     time.Time       `json:"updated_at"`
}

type Category struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	Type      string `json:"type"` // 'expense', 'income'
	Icon      string `json:"icon"`
	IsDefault bool   `json:"is_default"`
}

type Transaction struct {
	ID           int64           `json:"id"`
	Type         string          `json:"type"` // 'expense', 'income'
	Amount       decimal.Decimal `json:"amount"`
	CategoryID   *int64          `json:"category_id"`
	CategoryName string          `json:"category_name,omitempty"`
	CategoryIcon string          `json:"category_icon,omitempty"`
	AccountID    *int64          `json:"account_id"`
	AccountName  string          `json:"account_name,omitempty"`
	Date         string          `json:"date"` // YYYY-MM-DD
	Description  string          `json:"description"`
	CreatedAt    time.Time       `json:"created_at"`
}

type RecurringItem struct {
	ID             int64           `json:"id"`
	Title          string          `json:"title"`
	Type           string          `json:"type"` // 'income', 'expense', 'subscription'
	Amount         decimal.Decimal `json:"amount"`
	Currency       string          `json:"currency"`
	DayOfMonth     int             `json:"day_of_month"`
	Frequency      string          `json:"frequency"` // 'monthly', 'yearly'
	AutoRealize    bool            `json:"auto_realize"`
	IsActive       bool            `json:"is_active"`
	LastRealizedAt *string         `json:"last_realized_at"`
	CreatedAt      time.Time       `json:"created_at"`

	// Derived
	IsRealizedThisMonth bool `json:"is_realized_this_month"`
}

type Instrument struct {
	ID          int64           `json:"id"`
	Symbol      string          `json:"symbol"`
	Name        string          `json:"name"`
	AssetType   string          `json:"asset_type"` // 'stock', 'etf', 'fund', 'gold', 'currency', 'other'
	Market      string          `json:"market"`     // 'BIST', 'US', 'TEFAS', 'COMMODITY', 'FX', 'OTHER'
	Currency    string          `json:"currency"`
	Provider    string          `json:"provider"`
	IsManual    bool            `json:"is_manual"`
	ManualPrice decimal.Decimal `json:"manual_price"`
	UpdatedAt   time.Time       `json:"updated_at"`

	// Latest Quote
	LatestPrice decimal.Decimal `json:"latest_price"`
	QuotedAt    time.Time       `json:"quoted_at"`
	IsStale     bool            `json:"is_stale"`
}

type Position struct {
	ID           int64           `json:"id"`
	InstrumentID int64           `json:"instrument_id"`
	Instrument   Instrument      `json:"instrument"`
	Quantity     decimal.Decimal `json:"quantity"`
	AverageCost  decimal.Decimal `json:"average_cost"`
	CreatedAt    time.Time       `json:"created_at"`
	UpdatedAt    time.Time       `json:"updated_at"`

	// Derived Valuations
	CurrentPrice decimal.Decimal `json:"current_price"`
	MarketValue  decimal.Decimal `json:"market_value"`      // In instrument currency
	ValueInBase  decimal.Decimal `json:"value_in_base"`     // In base currency (TRY)
	PortfolioPct decimal.Decimal `json:"portfolio_pct"`     // Percentage of total portfolio
	ProfitLoss   decimal.Decimal `json:"profit_loss"`       // In base currency if avg cost set
	ProfitLossPct decimal.Decimal `json:"profit_loss_pct"`
	HasCostBasis bool            `json:"has_cost_basis"`
}

type MarketQuote struct {
	ID           int64           `json:"id"`
	InstrumentID int64           `json:"instrument_id"`
	Price        decimal.Decimal `json:"price"`
	Currency     string          `json:"currency"`
	QuotedAt     time.Time       `json:"quoted_at"`
	FetchedAt    time.Time       `json:"fetched_at"`
	Provider     string          `json:"provider"`
	IsStale      bool            `json:"is_stale"`
}

type PortfolioSnapshot struct {
	ID               int64           `json:"id"`
	SnapshotDate     string          `json:"snapshot_date"` // YYYY-MM-DD
	TotalAssetsBase  decimal.Decimal `json:"total_assets_base"`
	InvestmentsBase  decimal.Decimal `json:"investments_base"`
	CashBase         decimal.Decimal `json:"cash_base"`
	DebtBase         decimal.Decimal `json:"debt_base"`
	NetWorthBase     decimal.Decimal `json:"net_worth_base"`
	CreatedAt        time.Time       `json:"created_at"`
}

// FormatMoney formats a decimal into Turkish currency style: "50.000,00 TL" or "$ 1.250,50"
func FormatMoney(amount decimal.Decimal, currency string) string {
	sign := ""
	if amount.IsNegative() {
		sign = "-"
		amount = amount.Abs()
	}

	fixed := amount.StringFixed(2)
	parts := strings.Split(fixed, ".")
	intPart := parts[0]
	decPart := parts[1]

	// Add thousand separators with dots
	var formattedInt strings.Builder
	n := len(intPart)
	for i, char := range intPart {
		if i > 0 && (n-i)%3 == 0 {
			formattedInt.WriteRune('.')
		}
		formattedInt.WriteRune(char)
	}

	res := fmt.Sprintf("%s%s,%s", sign, formattedInt.String(), decPart)
	if currency == "TRY" || currency == "TL" {
		return res + " TL"
	} else if currency == "USD" {
		return "$ " + res
	} else if currency == "EUR" {
		return "€ " + res
	} else if currency == "GBP" {
		return "£ " + res
	}
	return res + " " + currency
}

// FormatQuantity formats quantity dynamically without unnecessary trailing zeroes
func FormatQuantity(qty decimal.Decimal) string {
	str := qty.StringFixed(6)
	str = strings.TrimRight(str, "0")
	str = strings.TrimRight(str, ".")
	if str == "" {
		return "0"
	}
	// Convert decimal dot to comma for Turkish locale display
	parts := strings.Split(str, ".")
	if len(parts) == 1 {
		return parts[0]
	}
	return parts[0] + "," + parts[1]
}

// FormatDateTurkish converts "YYYY-MM-DD" to "DD.MM.YYYY"
func FormatDateTurkish(dateStr string) string {
	t, err := time.Parse("2006-01-02", dateStr)
	if err != nil {
		return dateStr
	}
	return t.Format("02.01.2006")
}

// TimeAgoTurkish returns a friendly Turkish relative time string (e.g. "5 dk önce", "2 saat önce")
func TimeAgoTurkish(t time.Time) string {
	diff := time.Since(t)
	if diff < time.Minute {
		return "Az önce"
	} else if diff < time.Hour {
		return fmt.Sprintf("%d dk önce", int(diff.Minutes()))
	} else if diff < 24*time.Hour {
		return fmt.Sprintf("%d saat önce", int(diff.Hours()))
	} else {
		days := int(diff.Hours() / 24)
		if days == 1 {
			return "Dün"
		}
		return fmt.Sprintf("%d gün önce", days)
	}
}
