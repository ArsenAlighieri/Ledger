package database

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDatabaseMigrationAndBackup(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "ledger_db_test_*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	dbPath := filepath.Join(tempDir, "test.db")
	db, err := OpenDB(dbPath)
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer db.Close()

	// 1. Run migrations
	if err := Migrate(db); err != nil {
		t.Fatalf("migration failed: %v", err)
	}

	// 2. Verify default categories exist
	var catCount int
	err = db.QueryRow("SELECT COUNT(*) FROM categories").Scan(&catCount)
	if err != nil || catCount == 0 {
		t.Errorf("expected default categories to be inserted, count=%d err=%v", catCount, err)
	}

	// 3. Test Backup
	backupDir := filepath.Join(tempDir, "backups")
	backupFile, err := BackupDatabase(db, backupDir)
	if err != nil {
		t.Fatalf("backup failed: %v", err)
	}

	if _, err := os.Stat(backupFile); os.IsNotExist(err) {
		t.Errorf("expected backup file %s to exist", backupFile)
	}

	// 4. Test ListBackups
	backups := ListBackups(backupDir)
	if len(backups) != 1 {
		t.Errorf("expected 1 backup, got %d", len(backups))
	}
}
