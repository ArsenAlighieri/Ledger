package handlers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"ledger/internal/auth"
	"ledger/internal/database"
	"ledger/internal/market"
	"ledger/internal/services"
	"ledger/web"
)

func TestSetupView_LifecycleAndSelfHealing(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "ledger_setup_test_*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	dbPath := filepath.Join(tempDir, "test.db")
	db, err := database.OpenDB(dbPath)
	if err != nil {
		t.Fatalf("failed to open db: %v", err)
	}
	defer db.Close()

	if err := database.Migrate(db); err != nil {
		t.Fatalf("failed to migrate: %v", err)
	}

	renderer, err := NewRenderer(web.TemplateFS())
	if err != nil {
		t.Fatalf("failed to create renderer: %v", err)
	}

	marketSvc := market.NewService(db)
	financeSvc := services.NewFinanceService(db, marketSvc)
	snapshotSvc := services.NewSnapshotService(db, financeSvc)

	handler := NewSetupHandler(db, renderer, financeSvc, marketSvc, snapshotSvc)

	// Create user
	res, err := db.Exec("INSERT INTO users (username, password_hash) VALUES ('testuser', 'hash')")
	if err != nil {
		t.Fatalf("failed to insert user: %v", err)
	}
	userID, _ := res.LastInsertId()

	t.Run("SetupView with auth.UserContextKey (no settings row initially - self-heals)", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/setup", nil)
		ctx := context.WithValue(req.Context(), auth.UserContextKey, userID)
		req = req.WithContext(ctx)

		w := httptest.NewRecorder()
		handler.SetupView(w, req)

		resp := w.Result()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("expected status 200, got %d. Body: %s", resp.StatusCode, w.Body.String())
		}

		body := w.Body.String()
		if !strings.Contains(body, "Ledger'a Hoş Geldin") {
			t.Errorf("expected welcome step in body, got: %s", body)
		}
		if strings.Contains(body, "Ayarlar yüklenemedi") {
			t.Errorf("got 'Ayarlar yüklenemedi' error!")
		}

		// Verify settings row was created by self-healing
		var count int
		_ = db.QueryRow("SELECT COUNT(*) FROM settings WHERE user_id = ?", userID).Scan(&count)
		if count != 1 {
			t.Errorf("expected 1 settings row created, got %d", count)
		}
	})

	t.Run("SetupView with string user_id key", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/setup?step=2", nil)
		ctx := context.WithValue(req.Context(), "user_id", userID)
		req = req.WithContext(ctx)

		w := httptest.NewRecorder()
		handler.SetupView(w, req)

		resp := w.Result()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("expected status 200, got %d. Body: %s", resp.StatusCode, w.Body.String())
		}

		body := w.Body.String()
		if !strings.Contains(body, "Temel Tercihler") {
			t.Errorf("expected step 2 in body, got: %s", body)
		}
	})

	t.Run("SetupView fallback without context when single user exists", func(t *testing.T) {
		// Request has NO context user_id at all
		req := httptest.NewRequest("GET", "/setup?step=1", nil)

		w := httptest.NewRecorder()
		handler.SetupView(w, req)

		resp := w.Result()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("expected status 200, got %d. Body: %s", resp.StatusCode, w.Body.String())
		}

		body := w.Body.String()
		if !strings.Contains(body, "Ledger'a Hoş Geldin") {
			t.Errorf("expected step 1 in body, got: %s", body)
		}
	})
}
