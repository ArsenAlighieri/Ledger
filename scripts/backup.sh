#!/usr/bin/env bash
set -euo pipefail

BACKUP_DIR="./data/backups"
mkdir -p "${BACKUP_DIR}"

TIMESTAMP=$(date +%Y%m%d_%H%M%S)
BACKUP_PATH="${BACKUP_DIR}/ledger_${TIMESTAMP}.db"

echo "Ledger veritabanı yedekleniyor..."
if [ -f "./data/ledger.db" ]; then
    # SQLite online backup or copy
    sqlite3 ./data/ledger.db ".backup '${BACKUP_PATH}'" || cp ./data/ledger.db "${BACKUP_PATH}"
    echo "Yedek başarıyla oluşturuldu: ${BACKUP_PATH}"
else
    echo "Hata: ./data/ledger.db dosyası bulunamadı."
    exit 1
fi
