package handlers

import (
	"database/sql"
	"fmt"
	"net/http"
	"strings"

	"ledger/internal/models"
	"ledger/internal/services"

	"github.com/shopspring/decimal"
)

type DashboardHandler struct {
	db       *sql.DB
	renderer *Renderer
	finance  *services.FinanceService
	cfo      *services.CFOService
	snapshot *services.SnapshotService
}

func NewDashboardHandler(db *sql.DB, renderer *Renderer, f *services.FinanceService, cfo *services.CFOService, s *services.SnapshotService) *DashboardHandler {
	return &DashboardHandler{
		db:       db,
		renderer: renderer,
		finance:  f,
		cfo:      cfo,
		snapshot: s,
	}
}

type ChartCircle struct {
	X float64
	Y float64
}

type CashflowBarViewModel struct {
	MonthName     string
	Income        string
	Expense       string
	IncomeHeight  int
	ExpenseHeight int
}

func (h *DashboardHandler) DashboardView(w http.ResponseWriter, r *http.Request) {
	userID := GetUserID(r)
	if userID == 0 {
		_ = h.db.QueryRow("SELECT id FROM users ORDER BY id ASC LIMIT 1").Scan(&userID)
	}
	ctx := r.Context()

	var settings models.Settings
	var targetStr, invStr string
	_ = h.db.QueryRow(`
		SELECT id, user_id, setup_completed, setup_step, base_currency, emergency_fund_target, 
		       emergency_fund_asset_type, emergency_fund_asset_id, monthly_investment_target 
		FROM settings WHERE user_id = ?
	`, userID).Scan(
		&settings.ID, &settings.UserID, &settings.SetupCompleted, &settings.SetupStep,
		&settings.BaseCurrency, &targetStr, &settings.EmergencyFundAssetType,
		&settings.EmergencyFundAssetID, &invStr,
	)
	settings.EmergencyFundTarget, _ = decimal.NewFromString(targetStr)
	settings.MonthlyInvestmentTarget, _ = decimal.NewFromString(invStr)
	if settings.BaseCurrency == "" {
		settings.BaseCurrency = "TRY"
	}

	// 1. Gathers assets, debts, and positions
	accounts, cashBank, foreign, _ := h.finance.GetUserAccounts(ctx, settings.BaseCurrency)
	debts, currentDebt, stmtDebt, _ := h.finance.GetUserDebts(ctx)
	positions, investBase, _ := h.finance.GetUserPositions(ctx, settings.BaseCurrency)

	totalAssets := cashBank.Add(foreign).Add(investBase)
	netWorth := totalAssets.Sub(currentDebt)

	// 2. Cashflow metrics
	income, expense, remaining, savingsRate, lastExpense, expChange, _ := h.finance.GetMonthlyCashflow(ctx)

	// 3. CFO Evaluation
	cfoEval := h.cfo.Evaluate(ctx, settings, accounts, debts, positions, cashBank, foreign, stmtDebt, income, expense)

	// Build summary
	summary := services.FinancialSummary{
		TotalAssetsBase:      totalAssets,
		TotalDebtBase:        currentDebt,
		NetWorthBase:         netWorth,
		CashAndBankBase:      cashBank,
		ForeignCurrencyBase:  foreign,
		InvestmentsBase:      investBase,
		ThisMonthIncome:      income,
		ThisMonthExpense:     expense,
		ThisMonthRemaining:   remaining,
		SavingsRate:          savingsRate,
		LastMonthExpense:     lastExpense,
		ExpenseChangePct:     expChange,
		EmergencyFundCurrent: cfoEval.EmergencyFundCurrent,
		EmergencyFundTarget:  cfoEval.EmergencyFundTarget,
		EmergencyFundPct:     cfoEval.EmergencyFundPct,
		EmergencyFundName:    cfoEval.EmergencyFundName,
		SafeToSpend:          cfoEval.SafeToSpend,
		CFOAdvice:            cfoEval.Advice,
	}

	// 4. Cashflow Bars (last 6 months)
	rawBars, _ := h.snapshot.GetMonthlyCashflowBars(ctx)
	maxVal := decimal.NewFromInt(1)
	for _, b := range rawBars {
		if b.Income.GreaterThan(maxVal) {
			maxVal = b.Income
		}
		if b.Expense.GreaterThan(maxVal) {
			maxVal = b.Expense
		}
	}

	var cashflowBars []CashflowBarViewModel
	for _, b := range rawBars {
		incPct := int(b.Income.Div(maxVal).Mul(decimal.NewFromInt(100)).IntPart())
		expPct := int(b.Expense.Div(maxVal).Mul(decimal.NewFromInt(100)).IntPart())
		if incPct < 4 && !b.Income.IsZero() {
			incPct = 4
		}
		if expPct < 4 && !b.Expense.IsZero() {
			expPct = 4
		}

		cashflowBars = append(cashflowBars, CashflowBarViewModel{
			MonthName:     b.MonthName,
			Income:        b.Income.StringFixed(0),
			Expense:       b.Expense.StringFixed(0),
			IncomeHeight:  incPct,
			ExpenseHeight: expPct,
		})
	}

	// 5. Net worth history points
	points, _ := h.snapshot.GetNetWorthHistory(ctx, "TUMU")
	// If no snapshot exists yet, add current net worth as first point
	if len(points) == 0 {
		points = append(points, services.NetWorthPoint{
			Date:     models.FormatDateTurkish(CurrentDate()),
			NetWorth: netWorth,
		})
	}

	polyline, circles, firstDate, lastDate := buildChartPolyline(points)

	// Record daily snapshot in background to ensure up-to-date daily record
	go func() {
		_ = h.snapshot.RecordDailySnapshot(r.Context(), settings.BaseCurrency)
	}()

	h.renderer.Render(w, "dashboard", map[string]any{
		"Title":          "Genel Bakış",
		"ActiveNav":      "dashboard",
		"ShowNav":        true,
		"CSRFToken":      GetCSRFToken(r),
		"Summary":        summary,
		"CashflowBars":   cashflowBars,
		"NetWorthPoints": points,
		"ChartPolyline":  polyline,
		"ChartCircles":   circles,
		"FirstPointDate": firstDate,
		"LastPointDate":  lastDate,
	})
}

func (h *DashboardHandler) ChartView(w http.ResponseWriter, r *http.Request) {
	rangeCode := r.URL.Query().Get("range")
	if rangeCode == "" {
		rangeCode = "TUMU"
	}

	points, _ := h.snapshot.GetNetWorthHistory(r.Context(), rangeCode)
	polyline, circles, firstDate, lastDate := buildChartPolyline(points)

	h.renderer.Render(w, "networth_chart", map[string]any{
		"NetWorthPoints": points,
		"ChartPolyline":  polyline,
		"ChartCircles":   circles,
		"FirstPointDate": firstDate,
		"LastPointDate":  lastDate,
	})
}

func buildChartPolyline(points []services.NetWorthPoint) (string, []ChartCircle, string, string) {
	if len(points) == 0 {
		return "", nil, "", ""
	}

	firstDate := points[0].Date
	lastDate := points[len(points)-1].Date

	if len(points) == 1 {
		p := points[0]
		return "0,60 400,60", []ChartCircle{{X: 200, Y: 60}}, p.Date, p.Date
	}

	minVal := points[0].NetWorth
	maxVal := points[0].NetWorth
	for _, p := range points {
		if p.NetWorth.LessThan(minVal) {
			minVal = p.NetWorth
		}
		if p.NetWorth.GreaterThan(maxVal) {
			maxVal = p.NetWorth
		}
	}

	rangeVal := maxVal.Sub(minVal)
	if rangeVal.IsZero() {
		rangeVal = decimal.NewFromInt(1)
	}

	width := 400.0
	height := 100.0
	padding := 10.0

	var coords []string
	var circles []ChartCircle

	stepX := width / float64(len(points)-1)
	for i, p := range points {
		x := float64(i) * stepX
		ratio := p.NetWorth.Sub(minVal).Div(rangeVal).InexactFloat64()
		y := (height - (ratio * (height - 2*padding))) + padding

		coords = append(coords, fmt.Sprintf("%.1f,%.1f", x, y))
		circles = append(circles, ChartCircle{X: x, Y: y})
	}

	return strings.Join(coords, " "), circles, firstDate, lastDate
}
