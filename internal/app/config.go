package app

import (
	"os"
	"strconv"
)

type Config struct {
	Port          string
	DBPath        string
	BackupDir     string
	FxTTLMinutes  int
	StockTTL      int
}

func LoadConfig() *Config {
	port := os.Getenv("APP_PORT")
	if port == "" {
		port = "8085"
	}

	dbPath := os.Getenv("DB_PATH")
	if dbPath == "" {
		dbPath = "./data/ledger.db"
	}

	backupDir := os.Getenv("BACKUP_DIR")
	if backupDir == "" {
		backupDir = "./data/backups"
	}

	fxTTL, _ := strconv.Atoi(os.Getenv("FX_TTL_MINUTES"))
	if fxTTL <= 0 {
		fxTTL = 60
	}

	stockTTL, _ := strconv.Atoi(os.Getenv("STOCK_TTL_MINUTES"))
	if stockTTL <= 0 {
		stockTTL = 30
	}

	return &Config{
		Port:         port,
		DBPath:       dbPath,
		BackupDir:    backupDir,
		FxTTLMinutes: fxTTL,
		StockTTL:     stockTTL,
	}
}
