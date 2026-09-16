#!/usr/bin/env bash
set -euo pipefail

echo "=================================================="
echo "  LEDGER — Güvenli Güncelleme Başlatılıyor"
echo "=================================================="

# 1. Otomatik Güncelleme Öncesi Yedek Al
if [ -d "./data" ] && [ -f "./data/ledger.db" ]; then
    mkdir -p ./data/backups
    BACKUP_FILE="./data/backups/ledger_pre_update_$(date +%Y%m%d_%H%M%S).db"
    echo "Mevcut veri tabanı yedekleniyor: ${BACKUP_FILE}"
    cp ./data/ledger.db "${BACKUP_FILE}"
fi

# 2. Kod Güncellemesi
echo "Son kod değişiklikleri çekiliyor (git pull)..."
git pull

# 3. Konteyner Yeniden Derleme ve Başlatma
echo "Docker konteyneri yeniden derleniyor..."
docker compose up -d --build

echo "=================================================="
echo "  LEDGER başarıyla güncellendi ve yeniden başlatıldı."
echo "=================================================="
