package market

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/shopspring/decimal"
)

type YahooProvider struct {
	client *http.Client
}

func NewYahooProvider() *YahooProvider {
	return &YahooProvider{
		client: &http.Client{Timeout: 10 * time.Second},
	}
}

type yahooChartMeta struct {
	Currency           string   `json:"currency"`
	Symbol             string   `json:"symbol"`
	ExchangeName       string   `json:"exchangeName"`
	InstrumentType     string   `json:"instrumentType"`
	RegularMarketPrice *float64 `json:"regularMarketPrice"`
	RegularMarketTime  *int64   `json:"regularMarketTime"`
	ChartPreviousClose *float64 `json:"chartPreviousClose"`
}

type yahooIndicators struct {
	Quote []struct {
		Close []*float64 `json:"close"`
	} `json:"quote"`
}

type yahooChartResult struct {
	Meta       yahooChartMeta   `json:"meta"`
	Timestamp  []int64          `json:"timestamp"`
	Indicators yahooIndicators `json:"indicators"`
}

type yahooChartResponse struct {
	Chart struct {
		Result []yahooChartResult `json:"result"`
		Error  *struct {
			Code        string `json:"code"`
			Description string `json:"description"`
		} `json:"error"`
	} `json:"chart"`
}

type yahooSearchQuote struct {
	Symbol    string `json:"symbol"`
	Shortname string `json:"shortname"`
	Longname  string `json:"longname"`
	QuoteType string `json:"quoteType"`
	Exchange  string `json:"exchange"`
}

type yahooSearchResponse struct {
	Quotes []yahooSearchQuote `json:"quotes"`
}

func (p *YahooProvider) normalizeSymbol(symbol, market string) string {
	symbol = strings.ToUpper(strings.TrimSpace(symbol))
	if market == "BIST" && !strings.HasSuffix(symbol, ".IS") {
		return symbol + ".IS"
	}
	return symbol
}

func (p *YahooProvider) GetStockQuote(ctx context.Context, symbol, market string) (Quote, error) {
	yahooSymbol := p.normalizeSymbol(symbol, market)
	endpoint := fmt.Sprintf("https://query1.finance.yahoo.com/v8/finance/chart/%s?range=1d&interval=1d", url.PathEscape(yahooSymbol))

	req, err := http.NewRequestWithContext(ctx, "GET", endpoint, nil)
	if err != nil {
		return Quote{}, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")

	resp, err := p.client.Do(req)
	if err != nil {
		return Quote{}, fmt.Errorf("Yahoo Finance request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return Quote{}, fmt.Errorf("Yahoo Finance returned status %d for %s", resp.StatusCode, yahooSymbol)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return Quote{}, err
	}

	var data yahooChartResponse
	if err := json.Unmarshal(body, &data); err != nil {
		return Quote{}, fmt.Errorf("parsing Yahoo response failed: %w", err)
	}

	if data.Chart.Error != nil {
		return Quote{}, fmt.Errorf("Yahoo Finance error: %s", data.Chart.Error.Description)
	}

	if len(data.Chart.Result) == 0 {
		return Quote{}, fmt.Errorf("no chart result for %s", yahooSymbol)
	}

	res := data.Chart.Result[0]
	var priceVal float64
	if res.Meta.RegularMarketPrice != nil && *res.Meta.RegularMarketPrice > 0 {
		priceVal = *res.Meta.RegularMarketPrice
	} else if res.Meta.ChartPreviousClose != nil && *res.Meta.ChartPreviousClose > 0 {
		priceVal = *res.Meta.ChartPreviousClose
	} else {
		return Quote{}, fmt.Errorf("no valid price found for %s", yahooSymbol)
	}

	quotedAt := time.Now()
	if res.Meta.RegularMarketTime != nil && *res.Meta.RegularMarketTime > 0 {
		quotedAt = time.Unix(*res.Meta.RegularMarketTime, 0)
	}

	currency := res.Meta.Currency
	if currency == "" {
		if strings.HasSuffix(yahooSymbol, ".IS") {
			currency = "TRY"
		} else {
			currency = "USD"
		}
	}

	return Quote{
		Price:    decimal.NewFromFloat(priceVal),
		Currency: currency,
		QuotedAt: quotedAt,
		Provider: "YahooFinance",
		IsStale:  false,
	}, nil
}

func (p *YahooProvider) Search(ctx context.Context, query string) ([]SearchResult, error) {
	query = strings.TrimSpace(query)
	endpoint := fmt.Sprintf("https://query1.finance.yahoo.com/v1/finance/search?q=%s&quotesCount=8&newsCount=0", url.QueryEscape(query))

	req, err := http.NewRequestWithContext(ctx, "GET", endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var data yahooSearchResponse
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, err
	}

	var results []SearchResult
	for _, q := range data.Quotes {
		assetType := "stock"
		if q.QuoteType == "ETF" {
			assetType = "etf"
		} else if q.QuoteType == "CURRENCY" {
			assetType = "currency"
		}

		market := "US"
		symbolClean := q.Symbol
		currency := "USD"
		if strings.HasSuffix(q.Symbol, ".IS") {
			market = "BIST"
			symbolClean = strings.TrimSuffix(q.Symbol, ".IS")
			currency = "TRY"
		}

		name := q.Longname
		if name == "" {
			name = q.Shortname
		}
		if name == "" {
			name = symbolClean
		}

		results = append(results, SearchResult{
			Symbol:    symbolClean,
			Name:      name,
			Market:    market,
			AssetType: assetType,
			Currency:  currency,
		})
	}

	return results, nil
}
