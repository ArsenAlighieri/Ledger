package market

import (
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/shopspring/decimal"
)

type TCMBCurrency struct {
	Kod          string `xml:"Kod,attr"`
	CurrencyCode string `xml:"CurrencyCode,attr"`
	Unit         int    `xml:"Unit"`
	Isim         string `xml:"Isim"`
	ForexBuying  string `xml:"ForexBuying"`
	ForexSelling string `xml:"ForexSelling"`
}

type TCMBDate struct {
	XMLName    xml.Name       `xml:"Tarih_Date"`
	Tarih      string         `xml:"Tarih,attr"`
	Date       string         `xml:"Date,attr"`
	Currencies []TCMBCurrency `xml:"Currency"`
}

type TCMBProvider struct {
	client *http.Client
}

func NewTCMBProvider() *TCMBProvider {
	return &TCMBProvider{
		client: &http.Client{Timeout: 10 * time.Second},
	}
}

func (p *TCMBProvider) GetFXRates(ctx context.Context) (map[string]decimal.Decimal, time.Time, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", "https://www.tcmb.gov.tr/kurlar/today.xml", nil)
	if err != nil {
		return nil, time.Time{}, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, time.Time{}, fmt.Errorf("TCMB request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, time.Time{}, fmt.Errorf("TCMB returned status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, time.Time{}, fmt.Errorf("reading TCMB response failed: %w", err)
	}

	var data TCMBDate
	if err := xml.Unmarshal(body, &data); err != nil {
		return nil, time.Time{}, fmt.Errorf("unmarshaling TCMB XML failed: %w", err)
	}

	quotedAt, err := time.Parse("02.01.2006", data.Tarih)
	if err != nil {
		quotedAt = time.Now()
	}

	rates := make(map[string]decimal.Decimal)
	rates["TRY"] = decimal.NewFromInt(1)

	for _, c := range data.Currencies {
		code := strings.TrimSpace(c.Kod)
		sellingStr := strings.TrimSpace(c.ForexSelling)
		if sellingStr == "" {
			sellingStr = strings.TrimSpace(c.ForexBuying)
		}
		if sellingStr == "" {
			continue
		}

		rate, err := decimal.NewFromString(sellingStr)
		if err == nil && !rate.IsZero() {
			if c.Unit > 1 {
				rate = rate.Div(decimal.NewFromInt(int64(c.Unit)))
			}
			rates[code] = rate
		}
	}

	return rates, quotedAt, nil
}

func (p *TCMBProvider) GetFXRate(ctx context.Context, base, quote string) (FXRate, error) {
	base = strings.ToUpper(strings.TrimSpace(base))
	quote = strings.ToUpper(strings.TrimSpace(quote))

	if base == quote {
		return FXRate{
			Base:     base,
			Quote:    quote,
			Rate:     decimal.NewFromInt(1),
			QuotedAt: time.Now(),
			Provider: "TCMB",
		}, nil
	}

	rates, quotedAt, err := p.GetFXRates(ctx)
	if err != nil {
		return FXRate{}, err
	}

	baseRate, baseOk := rates[base]
	quoteRate, quoteOk := rates[quote]

	if !baseOk || !quoteOk {
		return FXRate{}, fmt.Errorf("currency pair %s/%s not found in TCMB", base, quote)
	}

	if quote == "TRY" {
		return FXRate{
			Base:     base,
			Quote:    "TRY",
			Rate:     baseRate,
			QuotedAt: quotedAt,
			Provider: "TCMB",
		}, nil
	}

	if baseRate.IsZero() || quoteRate.IsZero() {
		return FXRate{}, errors.New("zero rate received from TCMB")
	}

	// Cross rate: (Base to TRY) / (Quote to TRY)
	crossRate := baseRate.Div(quoteRate)
	return FXRate{
		Base:     base,
		Quote:    quote,
		Rate:     crossRate,
		QuotedAt: quotedAt,
		Provider: "TCMB",
	}, nil
}
