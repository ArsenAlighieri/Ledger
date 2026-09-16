package database

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// BackupDatabase executes a safe, atomic SQLite backup using VACUUM INTO.
func BackupDatabase(db *sql.DB, backupDir string) (string, error) {
	if err := os.MkdirAll(backupDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create backup directory: %w", err)
	}

	timestamp := time.Now().Format("2006-01-02_150405")
	backupFile := filepath.Join(backupDir, fmt.Sprintf("ledger_%s.db", timestamp))

	// In SQLite, VACUUM INTO creates an exact, transactionally consistent snapshot
	// We sanitize single quotes in the filepath
	escapedPath := strings.ReplaceAll(backupFile, "'", "''")
	query := fmt.Sprintf("VACUUM INTO '%s';", escapedPath)

	if _, err := db.Exec(query); err != nil {
		return "", fmt.Errorf("backup failed: %w", err)
	}

	// Prune old backups (keep last 30)
	PruneBackups(backupDir, 30)

	return backupFile, nil
}

// PruneBackups removes backups older than the maxCount most recent.
func PruneBackups(backupDir string, maxCount int) {
	entries, err := os.ReadDir(backupDir)
	if err != nil {
		return
	}

	var backups []string
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasPrefix(entry.Name(), "ledger_") && strings.HasSuffix(entry.Name(), ".db") {
			backups = append(backups, filepath.Join(backupDir, entry.Name()))
		}
	}

	if len(backups) <= maxCount {
		return
	}

	sort.Strings(backups)
	for i := 0; i < len(backups)-maxCount; i++ {
		_ = os.Remove(backups[i])
	}
}

// ListBackups returns all existing backup filenames sorted newest first.
func ListBackups(backupDir string) []string {
	entries, err := os.ReadDir(backupDir)
	if err != nil {
		return nil
	}

	var backups []string
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasPrefix(entry.Name(), "ledger_") && strings.HasSuffix(entry.Name(), ".db") {
			backups = append(backups, entry.Name())
		}
	}

	sort.Sort(sort.Reverse(sort.StringSlice(backups)))
	return backups
}
