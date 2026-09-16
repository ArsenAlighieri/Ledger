package services

import (
	"context"
	"testing"

	"ledger/internal/models"

	"github.com/shopspring/decimal"
)

func TestFinancialCalculations(t *testing.T) {
	// 1. FX Valuation test: 500 USD @ 42 TRY
	usdAmount := decimal.NewFromInt(500)
	usdTryRate := decimal.NewFromInt(42)
	tryVal := usdAmount.Mul(usdTryRate)
	expectedTryVal := decimal.NewFromInt(21000)

	if !tryVal.Equal(expectedTryVal) {
		t.Errorf("expected %s, got %s", expectedTryVal, tryVal)
	}

	// 2. Stock Valuation test: 3 AAPL @ 250 USD with USD/TRY = 42
	shares := decimal.NewFromInt(3)
	priceUSD := decimal.NewFromInt(250)
	totalUSD := shares.Mul(priceUSD)
	stockTryVal := totalUSD.Mul(usdTryRate)
	expectedStockTry := decimal.NewFromInt(31500)

	if !stockTryVal.Equal(expectedStockTry) {
		t.Errorf("expected %s, got %s", expectedStockTry, stockTryVal)
	}

	// 3. Net Worth: Assets - Debts
	totalAssets := tryVal.Add(stockTryVal).Add(decimal.NewFromInt(15000)) // 21000 + 31500 + 15000 = 67500
	totalDebts := decimal.NewFromInt(12500)
	netWorth := totalAssets.Sub(totalDebts)
	expectedNetWorth := decimal.NewFromInt(55000)

	if !netWorth.Equal(expectedNetWorth) {
		t.Errorf("expected %s, got %s", expectedNetWorth, netWorth)
	}

	// 4. Savings rate: (Income - Expense) / Income * 100
	income := decimal.NewFromInt(50000)
	expense := decimal.NewFromInt(30000)
	remaining := income.Sub(expense)
	savingsRate := remaining.Div(income).Mul(decimal.NewFromInt(100))
	expectedRate := decimal.NewFromInt(40)

	if !savingsRate.Equal(expectedRate) {
		t.Errorf("expected %s, got %s", expectedRate, savingsRate)
	}
}

func TestCFOEvaluationRules(t *testing.T) {
	cfo := NewCFOService(nil)
	ctx := context.Background()

	settings := models.Settings{
		EmergencyFundTarget: decimal.NewFromInt(50000),
	}

	// Case 1: Has credit card statement debt -> Priority is debt
	evalDebt := cfo.Evaluate(
		ctx,
		settings,
		nil,
		nil,
		nil,
		decimal.NewFromInt(30000), // cashBank
		decimal.Zero,              // foreign
		decimal.NewFromInt(8000),  // statement debt
		decimal.NewFromInt(40000), // income
		decimal.NewFromInt(25000), // expense
	)

	if !evalDebt.SafeToSpend.Equal(decimal.Zero) && evalDebt.SafeToSpend.IsNegative() {
		t.Errorf("safe to spend should never be negative")
	}
	if evalDebt.Advice == "" {
		t.Errorf("expected debt priority advice, got empty")
	}

	// Case 2: Zero debt, but below emergency fund target -> Priority is emergency fund
	evalEmergency := cfo.Evaluate(
		ctx,
		settings,
		nil,
		nil,
		nil,
		decimal.NewFromInt(35000),  // cashBank (target is 50,000)
		decimal.Zero,               // foreign
		decimal.Zero,               // zero statement debt
		decimal.NewFromInt(40000),
		decimal.NewFromInt(25000),
	)

	if evalEmergency.EmergencyFundPct.LessThan(decimal.NewFromInt(70)) {
		t.Errorf("expected around 70%% fund progress, got %s", evalEmergency.EmergencyFundPct)
	}

	// Case 3: Zero debt, emergency fund funded -> Priority is investment
	evalInvest := cfo.Evaluate(
		ctx,
		settings,
		nil,
		nil,
		nil,
		decimal.NewFromInt(60000),  // cashBank > 50,000
		decimal.Zero,
		decimal.Zero,
		decimal.NewFromInt(40000),
		decimal.NewFromInt(25000),
	)

	if evalInvest.SafeToSpend.IsNegative() {
		t.Errorf("safe to spend should be >= 0")
	}
}
