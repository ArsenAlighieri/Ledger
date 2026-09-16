#!/usr/bin/env bash
set -euo pipefail

if [ "$#" -ne 1 ]; then
    echo "Kullanım: $0 <yedek_dosyasi_yolu.db>"
    echo "Örnek: $0 ./data/backups/ledger_20260916_120000.db"
    exit 1
fi

BACKUP_SOURCE="$1"

if [ ! -f "${BACKUP_SOURCE}" ]; then
    echo "Hata: Belirtilen yedek dosyası bulunamadı: ${BACKUP_SOURCE}"
    exit 1
fi

echo "=================================================="
echo "  DİKKAT: Aktif veritabanı seçilen yedek ile değiştirilecek!"
echo "  Hedef: ./data/ledger.db"
echo "  Kaynak: ${BACKUP_SOURCE}"
echo "=================================================="
read -p "Devam etmek istiyor musun? (e/h): " -r CONFIRM
if [[ ! $CONFIRM =~ ^[Ee]$ ]]; then
    echo "İşlem iptal edildi."
    exit 0
fi

# 1. Konteyneri durdur
echo "Ledger servisi durduruluyor..."
docker compose stop

# 2. Güvenlik için mevcut dosyanın son kopyasını al
if [ -f "./data/ledger.db" ]; then
    cp ./data/ledger.db "./data/ledger_before_restore_$(date +%s).db"
fi

# 3. Yedeği geri yükle
echo "Veritabanı geri yükleniyor..."
cp "${BACKUP_SOURCE}" ./data/ledger.db

# 4. Konteyneri başlat
echo "Ledger servisi yeniden başlatılıyor..."
docker compose start

echo "Geri yükleme işlemi başarıyla tamamlandı."
