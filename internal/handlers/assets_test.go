package handlers

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"ledger/internal/database"
)

func TestAssetsHandler_CreateAndRemoveItems(t *testing.T) {
	dir, err := os.MkdirTemp("", "ledger_assets_test_*")
	if err != nil { t.Fatal(err) }
	defer os.RemoveAll(dir)
	db, err := database.OpenDB(filepath.Join(dir, "test.db"))
	if err != nil { t.Fatal(err) }
	defer db.Close()
	if err := database.Migrate(db); err != nil { t.Fatal(err) }
	h := &AssetsHandler{db: db}

	post := func(path string, values url.Values, fn http.HandlerFunc) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(values.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		w := httptest.NewRecorder()
		fn(w, req)
		if w.Code != http.StatusFound { t.Fatalf("%s: expected redirect, got %d", path, w.Code) }
		return w
	}

	post("/assets/account/create", url.Values{"name": {"Maaş Hesabı"}, "account_type": {"bank"}, "currency": {"TRY"}, "balance": {"1250.50"}}, h.CreateAccountAction)
	var accountID int64
	if err := db.QueryRow(`SELECT id FROM accounts WHERE name = 'Maaş Hesabı'`).Scan(&accountID); err != nil { t.Fatalf("account not created: %v", err) }
	post("/assets/account/delete/"+stringID(accountID), nil, h.DeleteAccountAction)
	var archived bool
	if err := db.QueryRow(`SELECT is_archived FROM accounts WHERE id = ?`, accountID).Scan(&archived); err != nil || !archived { t.Fatalf("account not archived: %v", err) }

	post("/assets/debt/create", url.Values{"name": {"Kart"}, "debt_type": {"credit_card"}, "current_debt": {"300"}, "statement_debt": {"100"}, "due_day": {"15"}}, h.CreateDebtAction)
	var debtID int64
	if err := db.QueryRow(`SELECT id FROM debts WHERE name = 'Kart'`).Scan(&debtID); err != nil { t.Fatalf("debt not created: %v", err) }
	post("/assets/debt/delete/"+stringID(debtID), nil, h.DeleteDebtAction)
	var debtCount int
	_ = db.QueryRow(`SELECT COUNT(*) FROM debts WHERE id = ?`, debtID).Scan(&debtCount)
	if debtCount != 0 { t.Fatal("debt not deleted") }

	post("/assets/position/create", url.Values{"symbol": {"MANUAL"}, "name": {"Manuel Varlık"}, "market": {"OTHER"}, "asset_type": {"other"}, "currency": {"TRY"}, "quantity": {"2"}, "average_cost": {"10"}, "manual_price": {"12"}}, h.CreatePositionAction)
	var positionID int64
	if err := db.QueryRow(`SELECT id FROM positions LIMIT 1`).Scan(&positionID); err != nil { t.Fatalf("position not created: %v", err) }
	post("/assets/position/delete/"+stringID(positionID), nil, h.DeletePositionAction)
	var positionCount int
	_ = db.QueryRow(`SELECT COUNT(*) FROM positions WHERE id = ?`, positionID).Scan(&positionCount)
	if positionCount != 0 { t.Fatal("position not deleted") }
}

func stringID(id int64) string { return strconv.FormatInt(id, 10) }
