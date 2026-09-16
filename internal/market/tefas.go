package market

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/shopspring/decimal"
)

type TEFASProvider struct {
	client *http.Client
}

func NewTEFASProvider() *TEFASProvider {
	return &TEFASProvider{
		client: &http.Client{Timeout: 12 * time.Second},
	}
}

type tefasItem struct {
	FonKodu  string   `json:"fonKodu"`
	FonUnvan string   `json:"fonUnvan"`
	Tarih    string   `json:"tarih"`
	Fiyat    *float64 `json:"fiyat"`
}

type tefasResponse struct {
	ErrorCode    *string     `json:"errorCode"`
	ErrorMessage *string     `json:"errorMessage"`
	ResultList   []tefasItem `json:"resultList"`
}

func (p *TEFASProvider) GetFundQuote(ctx context.Context, fundCode string) (Quote, string, error) {
	fundCode = strings.ToUpper(strings.TrimSpace(fundCode))
	now := time.Now()
	endDate := now.Format("20060102")
	startDate := now.AddDate(0, 0, -7).Format("20060102") // Look back 7 days for weekend/holiday

	payload := map[string]any{
		"fonTipi":        "YAT",
		"fonKodu":        fundCode,
		"aramaMetni":     nil,
		"fonTurKod":      nil,
		"fonGrubu":       nil,
		"sfonTurKod":     nil,
		"fonTurAciklama": nil,
		"kurucuKod":      nil,
		"basTarih":       startDate,
		"bitTarih":       endDate,
		"basSira":        1,
		"bitSira":        10,
		"dil":            "TR",
		"sFonTurKod":     "",
		"fonKod":         "",
		"fonGrup":        "",
		"fonUnvanTip":    "",
	}

	bodyBytes, err := json.Marshal(payload)
	if err != nil {
		return Quote{}, "", err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", "https://www.tefas.gov.tr/api/funds/fonGnlBlgSiraliGetir", bytes.NewBuffer(bodyBytes))
	if err != nil {
		return Quote{}, "", err
	}
	req.Header.Set("Accept", "*/*")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", "https://www.tefas.gov.tr")
	req.Header.Set("Referer", "https://www.tefas.gov.tr/tr/fon-verileri")
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")

	resp, err := p.client.Do(req)
	if err != nil {
		return Quote{}, "", fmt.Errorf("TEFAS request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return Quote{}, "", fmt.Errorf("TEFAS returned status %d", resp.StatusCode)
	}

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return Quote{}, "", err
	}

	var data tefasResponse
	if err := json.Unmarshal(respBytes, &data); err != nil {
		return Quote{}, "", fmt.Errorf("decoding TEFAS response failed: %w", err)
	}

	if len(data.ResultList) == 0 {
		return Quote{}, "", fmt.Errorf("no data returned for TEFAS fund %s", fundCode)
	}

	// First item is usually the most recent
	item := data.ResultList[0]
	if item.Fiyat == nil || *item.Fiyat <= 0 {
		return Quote{}, "", fmt.Errorf("invalid price for TEFAS fund %s", fundCode)
	}

	quotedAt, err := time.Parse("2006-01-02", item.Tarih)
	if err != nil {
		quotedAt = time.Now()
	}

	price := decimal.NewFromFloat(*item.Fiyat)
	return Quote{
		Price:    price,
		Currency: "TRY",
		QuotedAt: quotedAt,
		Provider: "TEFAS",
		IsStale:  false,
	}, item.FonUnvan, nil
}

func (p *TEFASProvider) SearchFunds(ctx context.Context, query string) ([]SearchResult, error) {
	query = strings.ToUpper(strings.TrimSpace(query))
	now := time.Now()
	endDate := now.Format("20060102")
	startDate := now.AddDate(0, 0, -5).Format("20060102")

	payload := map[string]any{
		"fonTipi":        "YAT",
		"fonKodu":        "",
		"aramaMetni":     query,
		"fonTurKod":      nil,
		"fonGrubu":       nil,
		"sfonTurKod":     nil,
		"fonTurAciklama": nil,
		"kurucuKod":      nil,
		"basTarih":       startDate,
		"bitTarih":       endDate,
		"basSira":        1,
		"bitSira":        15,
		"dil":            "TR",
		"sFonTurKod":     "",
		"fonKod":         "",
		"fonGrup":        "",
		"fonUnvanTip":    "",
	}

	bodyBytes, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", "https://www.tefas.gov.tr/api/funds/fonGnlBlgSiraliGetir", bytes.NewBuffer(bodyBytes))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "*/*")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", "https://www.tefas.gov.tr")
	req.Header.Set("Referer", "https://www.tefas.gov.tr/tr/fon-verileri")
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var data tefasResponse
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, err
	}

	seen := make(map[string]bool)
	var results []SearchResult
	for _, item := range data.ResultList {
		if seen[item.FonKodu] || item.FonKodu == "" {
			continue
		}
		seen[item.FonKodu] = true

		results = append(results, SearchResult{
			Symbol:    item.FonKodu,
			Name:      item.FonUnvan,
			Market:    "TEFAS",
			AssetType: "fund",
			Currency:  "TRY",
		})
	}

	return results, nil
}
