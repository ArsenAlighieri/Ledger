package app

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"ledger/internal/auth"
	"ledger/internal/database"
	"ledger/internal/handlers"
	"ledger/internal/market"
	"ledger/internal/services"
	"ledger/web"
)

type App struct {
	cfg        *Config
	db         *sql.DB
	server     *http.Server
	market     *market.Service
	finance    *services.FinanceService
	cfo        *services.CFOService
	snapshot   *services.SnapshotService
	recurring  *services.RecurringService
	middleware *auth.Middleware
}

func NewApp(cfg *Config) (*App, error) {
	db, err := database.OpenDB(cfg.DBPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	if err := database.Migrate(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to run migrations: %w", err)
	}

	marketSvc := market.NewService(db, cfg.FxTTLMinutes, cfg.StockTTL)
	financeSvc := services.NewFinanceService(db, marketSvc)
	cfoSvc := services.NewCFOService(financeSvc)
	snapshotSvc := services.NewSnapshotService(db, financeSvc)
	recurringSvc := services.NewRecurringService(db, marketSvc)
	mid := auth.NewMiddleware(db)

	return &App{
		cfg:        cfg,
		db:         db,
		market:     marketSvc,
		finance:    financeSvc,
		cfo:        cfoSvc,
		snapshot:   snapshotSvc,
		recurring:  recurringSvc,
		middleware: mid,
	}, nil
}

func (a *App) Start() error {
	renderer, err := handlers.NewRenderer(web.TemplateFS())
	if err != nil {
		return fmt.Errorf("failed to initialize renderer: %w", err)
	}

	mux := http.NewServeMux()

	// 1. Static files & PWA assets
	staticFS := http.FS(web.StaticFS())
	mux.Handle("/static/", http.StripPrefix("/static/", http.FileServer(staticFS)))
	mux.HandleFunc("/manifest.json", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFileFS(w, r, web.EmbeddedFiles, "static/manifest.json")
	})
	mux.HandleFunc("/sw.js", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/javascript")
		http.ServeFileFS(w, r, web.EmbeddedFiles, "static/sw.js")
	})

	// 2. Health check
	mux.Handle("/health", handlers.NewHealthHandler())

	// 3. Auth routes
	authH := handlers.NewAuthHandler(a.db, renderer, a.middleware.RateLimiter)
	mux.HandleFunc("/login", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			authH.LoginAction(w, r)
		} else {
			authH.LoginView(w, r)
		}
	})
	mux.HandleFunc("/register", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			authH.RegisterAction(w, r)
		} else {
			authH.RegisterView(w, r)
		}
	})
	mux.HandleFunc("/logout", authH.Logout)

	// 4. Setup Wizard routes
	setupH := handlers.NewSetupHandler(a.db, renderer, a.finance, a.market, a.snapshot)
	mux.HandleFunc("/setup", setupH.SetupView)
	mux.HandleFunc("/setup/next", setupH.NextStep)
	mux.HandleFunc("/setup/account/add", setupH.AddAccount)
	mux.HandleFunc("/setup/foreign/add", setupH.AddForeignCurrency)
	mux.HandleFunc("/setup/debt/add", setupH.AddDebt)
	mux.HandleFunc("/setup/income/save", setupH.SaveIncome)
	mux.HandleFunc("/setup/expense/save", setupH.SaveExpenses)
	mux.HandleFunc("/setup/subscription/save", setupH.SaveSubscriptions)
	mux.HandleFunc("/setup/investment/add", setupH.AddInvestment)
	mux.HandleFunc("/setup/finish", setupH.FinishSetup)
	mux.HandleFunc("/setup/restart", setupH.RestartSetup)

	// Setup delete helpers
	mux.HandleFunc("/setup/account/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete || r.Method == http.MethodPost {
			id := strings.TrimPrefix(r.URL.Path, "/setup/account/")
			_, _ = a.db.Exec("DELETE FROM accounts WHERE id = ?", id)
			http.Redirect(w, r, "/setup?step=3", http.StatusFound)
		}
	})
	mux.HandleFunc("/setup/debt/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete || r.Method == http.MethodPost {
			id := strings.TrimPrefix(r.URL.Path, "/setup/debt/")
			_, _ = a.db.Exec("DELETE FROM debts WHERE id = ?", id)
			http.Redirect(w, r, "/setup?step=5", http.StatusFound)
		}
	})
	mux.HandleFunc("/setup/position/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete || r.Method == http.MethodPost {
			id := strings.TrimPrefix(r.URL.Path, "/setup/position/")
			_, _ = a.db.Exec("DELETE FROM positions WHERE id = ?", id)
			http.Redirect(w, r, "/setup?step=9", http.StatusFound)
		}
	})

	// 5. Dashboard routes
	dashH := handlers.NewDashboardHandler(a.db, renderer, a.finance, a.cfo, a.snapshot)
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		http.Redirect(w, r, "/dashboard", http.StatusFound)
	})
	mux.HandleFunc("/dashboard", dashH.DashboardView)
	mux.HandleFunc("/dashboard/chart", dashH.ChartView)

	// 6. Transactions routes
	txH := handlers.NewTransactionsHandler(a.db, renderer, a.finance)
	mux.HandleFunc("/transactions", txH.IndexView)
	mux.HandleFunc("/transactions/new", txH.NewModal)
	mux.HandleFunc("/transactions/create", txH.CreateAction)
	mux.HandleFunc("/transactions/delete/", txH.DeleteAction)

	// 7. Recurring routes
	recH := handlers.NewRecurringHandler(a.db, renderer, a.recurring)
	mux.HandleFunc("/recurring", recH.IndexView)
	mux.HandleFunc("/recurring/create", recH.CreateAction)
	mux.HandleFunc("/recurring/delete/", recH.DeleteAction)
	mux.HandleFunc("/recurring/realize/", recH.RealizeAction)

	// 8. Assets & Market routes
	assetH := handlers.NewAssetsHandler(a.db, renderer, a.finance, a.market)
	mux.HandleFunc("/assets", assetH.IndexView)
	mux.HandleFunc("/assets/refresh-quotes", assetH.RefreshQuotesAction)
	mux.HandleFunc("/market/search", assetH.SearchMarketAction)
	mux.HandleFunc("/assets/account/create", assetH.CreateAccountAction)
	mux.HandleFunc("/assets/account/delete/", assetH.DeleteAccountAction)
	mux.HandleFunc("/assets/debt/create", assetH.CreateDebtAction)
	mux.HandleFunc("/assets/debt/delete/", assetH.DeleteDebtAction)
	mux.HandleFunc("/assets/position/create", assetH.CreatePositionAction)
	mux.HandleFunc("/assets/position/delete/", assetH.DeletePositionAction)
	mux.HandleFunc("/assets/edit-account/", assetH.EditAccountModal)
	mux.HandleFunc("/assets/update-account/", assetH.UpdateAccountAction)
	mux.HandleFunc("/assets/edit-debt/", assetH.EditDebtModal)
	mux.HandleFunc("/assets/update-debt/", assetH.UpdateDebtAction)
	mux.HandleFunc("/assets/edit-position/", assetH.EditPositionModal)
	mux.HandleFunc("/assets/update-position/", assetH.UpdatePositionAction)

	// 9. Settings & Export routes
	settingsH := handlers.NewSettingsHandler(a.db, renderer, a.finance, a.cfg.BackupDir)
	mux.HandleFunc("/settings", settingsH.IndexView)
	mux.HandleFunc("/settings/update-preferences", settingsH.UpdatePreferencesAction)
	mux.HandleFunc("/settings/backup", settingsH.BackupAction)
	mux.HandleFunc("/export/transactions.csv", settingsH.ExportTransactionsCSV)
	mux.HandleFunc("/export/assets.csv", settingsH.ExportAssetsCSV)
	mux.HandleFunc("/export/backup.json", settingsH.ExportFullJSON)

	// Apply Middleware Pipeline: CSRF -> AuthRequired -> SetupCheck -> Logger
	handler := a.middleware.CSRFMiddleware(
		a.middleware.AuthRequired(
			a.middleware.SetupCheck(mux),
		),
	)

	// Start Background Tasks (Daily Snapshot & Recurring Auto-Realization)
	a.startScheduler()

	addr := fmt.Sprintf("0.0.0.0:%s", a.cfg.Port)
	log.Printf("[LEDGER] Sunucu başlatılıyor: %s", addr)

	a.server = &http.Server{
		Addr:         addr,
		Handler:      handler,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	return a.server.ListenAndServe()
}

func (a *App) startScheduler() {
	go func() {
		runRecurring := func() {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			if err := a.recurring.AutoRealizeEligible(ctx); err != nil { log.Printf("[LEDGER] Düzenli kalemler işlenemedi: %v", err) }
		}
		runRecurring()
		lastDailyRun := ""
		ticker := time.NewTicker(1 * time.Hour)
		defer ticker.Stop()

		for range ticker.C {
			runRecurring()

			// Run once after 23:00. Tracking the date avoids duplicate runs while
			// allowing an app started at 23:30 to still perform the daily work.
			now := time.Now()
			today := now.Format("2006-01-02")
			if now.Hour() >= 23 && lastDailyRun != today {
				ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
				if err := a.snapshot.RecordDailySnapshot(ctx, "TRY"); err != nil { log.Printf("[LEDGER] Günlük snapshot alınamadı: %v", err) }
				if _, err := database.BackupDatabase(a.db, a.cfg.BackupDir); err != nil { log.Printf("[LEDGER] Günlük yedek alınamadı: %v", err) }
				cancel()
				lastDailyRun = today
			}
		}
	}()
}

func (a *App) Close() {
	if a.server != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = a.server.Shutdown(ctx)
	}
	if a.db != nil {
		_ = a.db.Close()
	}
}
