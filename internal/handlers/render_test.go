package handlers

import (
	"bytes"
	"fmt"
	"strings"
	"testing"

	"ledger/internal/models"
	"ledger/internal/services"
	"ledger/web"

	"github.com/shopspring/decimal"
)

func TestRenderer_IsolationAndPageRendering(t *testing.T) {
	renderer, err := NewRenderer(web.TemplateFS())
	if err != nil {
		t.Fatalf("NewRenderer failed: %v", err)
	}

	// 1. Test Dashboard View
	t.Run("Dashboard Page", func(t *testing.T) {
		var buf bytes.Buffer
		data := map[string]any{
			"Title":     "Genel Bakış",
			"ActiveNav": "dashboard",
			"ShowNav":   true,
			"CSRFToken": "test-csrf",
			"Summary": services.FinancialSummary{
				NetWorthBase:       decimal.NewFromInt(150000),
				ThisMonthIncome:    decimal.NewFromInt(50000),
				ThisMonthExpense:   decimal.NewFromInt(20000),
				ThisMonthRemaining: decimal.NewFromInt(30000),
				InvestmentsBase:    decimal.NewFromInt(80000),
				TotalDebtBase:      decimal.NewFromInt(5000),
				SafeToSpend:        decimal.NewFromInt(15000),
			},
		}

		if err := renderer.Execute(&buf, "dashboard", data); err != nil {
			t.Fatalf("Failed to execute dashboard: %v", err)
		}

		out := buf.String()
		if !strings.Contains(out, "Net Varlık") {
			t.Errorf("Expected 'Net Varlık' in dashboard output")
		}
		if !strings.Contains(out, "Güvenle Harcanabilir") {
			t.Errorf("Expected 'Güvenle Harcanabilir' in dashboard output")
		}
		// Crucial test: Settings page content MUST NOT leak into dashboard!
		if strings.Contains(out, "Acil Durum Fonu Hedefi (TL)") {
			t.Errorf("Settings page leaked into dashboard!")
		}
	})

	// 2. Test Transactions View
	t.Run("Transactions Page", func(t *testing.T) {
		var buf bytes.Buffer
		data := map[string]any{
			"Title":     "İşlemler",
			"ActiveNav": "transactions",
			"ShowNav":   true,
			"CSRFToken": "test-csrf",
			"Summary": services.FinancialSummary{
				ThisMonthIncome:  decimal.NewFromInt(50000),
				ThisMonthExpense: decimal.NewFromInt(20000),
			},
		}

		if err := renderer.Execute(&buf, "transactions", data); err != nil {
			t.Fatalf("Failed to execute transactions: %v", err)
		}

		out := buf.String()
		if !strings.Contains(out, "İşlemler") {
			t.Errorf("Expected 'İşlemler' in transactions output")
		}
		if strings.Contains(out, "Acil Durum Fonu Hedefi (TL)") {
			t.Errorf("Settings page leaked into transactions!")
		}
	})

	// 3. Test Recurring View
	t.Run("Recurring Page", func(t *testing.T) {
		var buf bytes.Buffer
		data := map[string]any{
			"Title":     "Düzenli Ödemeler & Gelirler",
			"ActiveNav": "recurring",
			"ShowNav":   true,
			"CSRFToken": "test-csrf",
		}

		if err := renderer.Execute(&buf, "recurring", data); err != nil {
			t.Fatalf("Failed to execute recurring: %v", err)
		}

		out := buf.String()
		if !strings.Contains(out, "Düzenli Ödemeler") {
			t.Errorf("Expected 'Düzenli Ödemeler' in recurring output")
		}
		if strings.Contains(out, "Acil Durum Fonu Hedefi (TL)") {
			t.Errorf("Settings page leaked into recurring!")
		}
	})

	// 4. Test Assets View
	t.Run("Assets Page", func(t *testing.T) {
		var buf bytes.Buffer
		data := map[string]any{
			"Title":     "Varlıklar",
			"ActiveNav": "assets",
			"ShowNav":   true,
			"CSRFToken": "test-csrf",
		}

		if err := renderer.Execute(&buf, "assets", data); err != nil {
			t.Fatalf("Failed to execute assets: %v", err)
		}

		out := buf.String()
		if !strings.Contains(out, "Varlık Dökümü") {
			t.Errorf("Expected 'Varlık Dökümü' in assets output")
		}
		if strings.Contains(out, "Acil Durum Fonu Hedefi (TL)") {
			t.Errorf("Settings page leaked into assets!")
		}
	})

	// 5. Test Settings View
	t.Run("Settings Page", func(t *testing.T) {
		var buf bytes.Buffer
		data := map[string]any{
			"Title":     "Ayarlar",
			"ActiveNav": "settings",
			"ShowNav":   true,
			"CSRFToken": "test-csrf",
			"Settings": models.Settings{
				BaseCurrency:            "TRY",
				EmergencyFundTarget:     decimal.NewFromInt(100000),
				MonthlyInvestmentTarget: decimal.NewFromInt(15000),
			},
		}

		if err := renderer.Execute(&buf, "settings", data); err != nil {
			t.Fatalf("Failed to execute settings: %v", err)
		}

		out := buf.String()
		if !strings.Contains(out, "Ayarlar") {
			t.Errorf("Expected 'Ayarlar' in settings output")
		}
		if !strings.Contains(out, "Acil Durum Fonu Hedefi (TL)") {
			t.Errorf("Expected emergency fund target in settings output")
		}
	})

	// 6. Test Auth Views
	t.Run("Login Page", func(t *testing.T) {
		var buf bytes.Buffer
		data := map[string]any{
			"Title":     "Giriş Yap",
			"ShowNav":   false,
			"CSRFToken": "test-csrf",
		}

		if err := renderer.Execute(&buf, "login", data); err != nil {
			t.Fatalf("Failed to execute login: %v", err)
		}

		out := buf.String()
		if !strings.Contains(out, "Ledger Girişi") {
			t.Errorf("Expected 'Ledger Girişi' in login output")
		}
	})

	t.Run("Register Page", func(t *testing.T) {
		var buf bytes.Buffer
		data := map[string]any{
			"Title":     "İlk Kurulum — Yönetici Hesabı",
			"ShowNav":   false,
			"CSRFToken": "test-csrf",
		}

		if err := renderer.Execute(&buf, "register", data); err != nil {
			t.Fatalf("Failed to execute register: %v", err)
		}

		out := buf.String()
		if !strings.Contains(out, "İlk Kurulum — Yönetici Hesabı") {
			t.Errorf("Expected 'İlk Kurulum' in register output")
		}
	})

	// 7. Test Setup Steps
	t.Run("Setup Wizard Steps", func(t *testing.T) {
		for step := 1; step <= 11; step++ {
			var buf bytes.Buffer
			data := map[string]any{
				"Title":       fmt.Sprintf("Kurulum — Adım %d", step),
				"ShowNav":     false,
				"CSRFToken":   "test-csrf",
				"Step":        step,
				"ProgressPct": (step - 1) * 10,
				"Settings": models.Settings{
					BaseCurrency: "TRY",
				},
				"Summary": services.FinancialSummary{
					NetWorthBase: decimal.NewFromInt(100000),
				},
			}

			viewName := fmt.Sprintf("setup_step%d", step)
			if err := renderer.Execute(&buf, viewName, data); err != nil {
				t.Fatalf("Failed to execute %s: %v", viewName, err)
			}

			out := buf.String()
			if !strings.Contains(out, fmt.Sprintf("Adım %d / 11", step)) {
				t.Errorf("Step %d missing wizard header", step)
			}
			if step == 1 && !strings.Contains(out, "Ledger'a Hoş Geldin") {
				t.Errorf("Step 1 missing welcome message")
			}
			if step == 2 && !strings.Contains(out, "Temel Tercihler") {
				t.Errorf("Step 2 missing preferences title")
			}
		}
	})

	// 8. Test Base.html Auto-Resolution Fallback
	t.Run("Base.html Fallback", func(t *testing.T) {
		var buf bytes.Buffer
		data := map[string]any{
			"Title":     "Genel Bakış",
			"ActiveNav": "dashboard",
			"ShowNav":   true,
			"CSRFToken": "test-csrf",
			"Summary": services.FinancialSummary{
				NetWorthBase: decimal.NewFromInt(150000),
			},
		}

		if err := renderer.Execute(&buf, "base.html", data); err != nil {
			t.Fatalf("Failed fallback execute: %v", err)
		}

		out := buf.String()
		if !strings.Contains(out, "Net Varlık") {
			t.Errorf("Expected dashboard content through base.html fallback")
		}
		if strings.Contains(out, "Acil Durum Fonu Hedefi (TL)") {
			t.Errorf("Settings page leaked into base.html fallback!")
		}
	})

	// 9. Test Partials
	t.Run("Partials", func(t *testing.T) {
		// Modal
		var modalBuf bytes.Buffer
		if err := renderer.Execute(&modalBuf, "modal.html", map[string]any{
			"CSRFToken": "test-csrf",
			"TodayDate": "2026-09-17",
		}); err != nil {
			t.Fatalf("Failed to execute modal.html: %v", err)
		}
		if !strings.Contains(modalBuf.String(), "Yeni İşlem Kaydı") {
			t.Errorf("Expected 'Yeni İşlem Kaydı' in modal output")
		}

		// Edit Modal
		var editBuf bytes.Buffer
		if err := renderer.Execute(&editBuf, "edit_modal.html", map[string]any{
			"Title":     "Hesap Düzenle",
			"Type":      "account",
			"CSRFToken": "test-csrf",
		}); err != nil {
			t.Fatalf("Failed to execute edit_modal.html: %v", err)
		}
		if !strings.Contains(editBuf.String(), "Hesap Düzenle") {
			t.Errorf("Expected 'Hesap Düzenle' in edit modal output")
		}

		// Search Results
		var searchBuf bytes.Buffer
		if err := renderer.Execute(&searchBuf, "search_results.html", map[string]any{
			"Results": []models.Instrument{
				{Symbol: "THYAO.IS", Name: "Türk Hava Yolları", Market: "BIST", Currency: "TRY"},
			},
		}); err != nil {
			t.Fatalf("Failed to execute search_results.html: %v", err)
		}
		if !strings.Contains(searchBuf.String(), "THYAO.IS") {
			t.Errorf("Expected 'THYAO.IS' in search results output")
		}

		// Net Worth Chart
		var chartBuf bytes.Buffer
		if err := renderer.Execute(&chartBuf, "networth_chart", map[string]any{}); err != nil {
			t.Fatalf("Failed to execute networth_chart: %v", err)
		}
		if !strings.Contains(chartBuf.String(), "İlk günlük kapanışta grafik verisi oluşacak") {
			t.Errorf("Expected empty chart fallback in networth_chart")
		}
	})
}
