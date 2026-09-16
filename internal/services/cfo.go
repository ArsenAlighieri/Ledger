package services

import (
	"context"
	"fmt"
	"time"

	"ledger/internal/models"

	"github.com/shopspring/decimal"
)

type CFOService struct {
	finance *FinanceService
}

func NewCFOService(f *FinanceService) *CFOService {
	return &CFOService{finance: f}
}

type CFOEvaluation struct {
	Advice               string
	SafeToSpend          decimal.Decimal
	EmergencyFundCurrent decimal.Decimal
	EmergencyFundTarget  decimal.Decimal
	EmergencyFundPct     decimal.Decimal
	EmergencyFundName    string
}

// Evaluate produces deterministic financial advice and calculates safe-to-spend liquidity.
func (s *CFOService) Evaluate(
	ctx context.Context,
	settings models.Settings,
	accounts []models.Account,
	debts []models.Debt,
	positions []models.Position,
	cashBankBase decimal.Decimal,
	foreignBase decimal.Decimal,
	totalStatementDebt decimal.Decimal,
	thisMonthIncome decimal.Decimal,
	thisMonthExpense decimal.Decimal,
) CFOEvaluation {
	// 1. Calculate Emergency Fund Status
	target := settings.EmergencyFundTarget
	if target.IsZero() {
		target = decimal.NewFromInt(50000)
	}

	currentFund := decimal.Zero
	fundName := "Genel Nakit Rezervi"

	if settings.EmergencyFundAssetType == "account" && settings.EmergencyFundAssetID > 0 {
		for _, a := range accounts {
			if a.ID == settings.EmergencyFundAssetID {
				currentFund = a.BalanceInBase
				fundName = a.Name
				break
			}
		}
	} else if settings.EmergencyFundAssetType == "position" && settings.EmergencyFundAssetID > 0 {
		for _, p := range positions {
			if p.ID == settings.EmergencyFundAssetID {
				currentFund = p.ValueInBase
				fundName = p.Instrument.Name
				break
			}
		}
	} else {
		// Default to cash & bank balance if not explicitly bound
		currentFund = cashBankBase
	}

	fundPct := decimal.Zero
	if !target.IsZero() {
		fundPct = currentFund.Div(target).Mul(decimal.NewFromInt(100))
		if fundPct.GreaterThan(decimal.NewFromInt(100)) {
			fundPct = decimal.NewFromInt(100)
		}
	}

	// 2. Safe-to-Spend Liquidity Calculation:
	// Liquid assets = Cash + Bank (not locked in non-liquid assets)
	liquidMoney := cashBankBase.Add(foreignBase)

	// Subtract protected emergency fund reserve
	protectedReserve := decimal.Zero
	// Only subtract a reserve when it is already part of liquidMoney. A position
	// is included in investments, not cash, so subtracting it here counted it twice.
	if settings.EmergencyFundAssetType != "position" {
		protectedReserve = currentFund
	}
	if protectedReserve.GreaterThan(target) {
		protectedReserve = target
	}

	// Estimate remaining recurring expenses this month from recurring_items
	remainingRecurring := s.getRemainingRecurringExpenses(ctx)

	// Safe to spend = Liquid money + expected remaining income - remaining recurring - statement debts - protected reserve
	spendable := liquidMoney.Sub(totalStatementDebt).Sub(remainingRecurring).Sub(protectedReserve)
	if spendable.IsNegative() {
		spendable = decimal.Zero
	}

	// 3. Deterministic Advice Generation (Strict Priority: Debt -> Emergency -> Investment)
	var advice string
	if totalStatementDebt.IsPositive() {
		formattedDebt := models.FormatMoney(totalStatementDebt, "TRY")
		if currentFund.LessThan(target) {
			deficit := target.Sub(currentFund)
			advice = fmt.Sprintf("%s ekstre borcun bulunuyor. Önceliğin ay sonuna kadar bunu kapatmak. Ardından acil durum fonu eksiğin olan %s tutarı tamamlamalısın.", formattedDebt, models.FormatMoney(deficit, "TRY"))
		} else {
			advice = fmt.Sprintf("%s ekstre borcun bulunuyor. Birinci önceliğin bunu kapatmak. Acil durum fonun ise tam olarak güvende.", formattedDebt)
		}
	} else if currentFund.LessThan(target) {
		deficit := target.Sub(currentFund)
		advice = fmt.Sprintf("Kredi kartı ekstre borcun bulunmuyor. Acil durum fonu hedefinin %s altındasın; bu ayki bütçe fazlanı öncelikle buraya aktarman önerilir.", models.FormatMoney(deficit, "TRY"))
	} else {
		surplus := thisMonthIncome.Sub(thisMonthExpense)
		if surplus.IsPositive() {
			advice = fmt.Sprintf("Borç ve acil durum fonu hedeflerin güvencede! Bu ayki bütçe fazlanın yatırım için ayrılabilecek kısmı: %s.", models.FormatMoney(surplus, "TRY"))
		} else {
			advice = "Borç ve acil durum fonu hedeflerin tam. Harcamaların gelirine paralel seyrediyor, finansal dengen stabil."
		}
	}

	return CFOEvaluation{
		Advice:               advice,
		SafeToSpend:          spendable,
		EmergencyFundCurrent: currentFund,
		EmergencyFundTarget:  target,
		EmergencyFundPct:     fundPct,
		EmergencyFundName:    fundName,
	}
}

func (s *CFOService) getRemainingRecurringExpenses(ctx context.Context) decimal.Decimal {
	if s.finance == nil || s.finance.db == nil {
		return decimal.Zero
	}
	now := time.Now()
	currentDay := now.Day()

	currentMonth := int(now.Month())
	currentMonthlyPeriod := now.Format("2006-01")
	currentYearPeriod := now.Format("2006")
	rows, err := s.finance.db.Query(`
		SELECT amount, currency FROM recurring_items
		WHERE is_active = 1 AND type IN ('expense', 'subscription') AND day_of_month >= ?
		  AND ((frequency = 'monthly' AND (last_realized_at IS NULL OR last_realized_at != ?))
		    OR (frequency = 'yearly' AND CAST(strftime('%m', created_at) AS INTEGER) = ? AND (last_realized_at IS NULL OR last_realized_at != ?)))
	`, currentDay, currentMonthlyPeriod, currentMonth, currentYearPeriod)
	if err != nil {
		return decimal.Zero
	}
	defer rows.Close()

	total := decimal.Zero
	for rows.Next() {
		var amtStr, currency string
		if err := rows.Scan(&amtStr, &currency); err == nil {
			amt, _ := decimal.NewFromString(amtStr)
			if currency != "" && currency != "TRY" && s.finance.market != nil {
				if fx, fxErr := s.finance.market.GetFXRate(ctx, currency, "TRY"); fxErr == nil && !fx.Rate.IsZero() {
					amt = amt.Mul(fx.Rate)
				} else {
					continue
				}
			}
			total = total.Add(amt)
		}
	}
	return total
}
