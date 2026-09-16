# LEDGER — Kişisel Finans ve Varlık Yönetimi

> *"Ledger'a neye sahip olduğumu, ne borcum olduğunu, düzenli ne girip ne çıktığını ve yatırımlarımdan kaç adet olduğunu söylerim. Ledger gerisini otomatik hesaplar."*

---

## Genel Bakış

**Ledger**, tek kullanıcılı (single-user), kendi sunucunda barındırılabilir (self-hosted), gizlilik odaklı modern bir kişisel finans kokpitidir.

Geleneksel muhasebe yazılımlarının veya SaaS ürünlerinin karmaşıklığından arındırılmıştır:
- Çift taraflı kayıt tutma (double-entry) zorunluluğu yoktur.
- Banka veya aracı kurum şifresi/kazıması (scraping) gerektirmez.
- Yalnızca tek bir Docker konteyneri olarak çalışır.
- Boşta CPU kullanımı önemsiz derecede düşüktür (<%0.1).
- Özel **ZeroTier** ağı üzerinden güvenle erişilir.

---

## Temel Özellikler

1. **Rehberli İlk Kurulum Sihirbazı (Setup Wizard)**:
   - İlk açılışta asla boş bir gösterge paneli gösterilmez.
   - 11 adımlık sade sihirbaz; hesap bakiyeleri, döviz, borçlar, düzenli gelir/giderler ve yatırımları toplar.
   - Her adımda *"Şimdilik Geç"* seçeneği bulunur.
   - Tarayıcı kapansa bile son kalınan adımdan devam eder.
   - İstenildiğinde `Ayarlar → Kurulum Sihirbazını Tekrar Aç` ile verileri silmeden yeniden çalıştırılabilir.
2. **Otomatik Piyasa ve Fiyat Entegrasyonu**:
   - **Döviz (FX)**: TCMB (Türkiye Cumhuriyet Merkez Bankası) resmi XML servisinden günlük canlı kurlar (USD, EUR, GBP).
   - **Hisse Senetleri & ETF**: Borsa İstanbul (BIST) hisseleri ve ABD piyasaları (AAPL, THYAO vb.).
   - **Yatırım Fonları (TEFAS / PPF)**: Takasbank TEFAS resmi API'si üzerinden günlük fon birim fiyatları (TP2 vb.).
   - **Altın (Gram Altın)**: Uluslararası spot fiyattan ve TCMB kurundan otomatik hesaplama.
   - **Kesinti Koruması**: Ağ veya API kesintilerinde son bilinen fiyat kullanılır, sistem asla kilitlenmez veya 0 göstermez.
3. **5 Saniyede Hızlı İşlem Girişi (`+ İŞLEM`)**:
   - Tür (Gider/Gelir), Tutar ve Kategori seçilip tek dokunuşla kaydedilir.
4. **Deterministik CFO Motoru & Güvenle Harcanabilir**:
   - LLM veya yapay zeka gerektirmeden kesin finansal kurallarla tavsiye üretir (Öncelik: Ekstre Borcu → Acil Durum Fonu → Yatırım).
   - Likit varlıklar, beklenen harcamalar ve acil durum rezervi korunarak *"Güvenle Harcanabilir"* nakit tutarı anlık hesaplanır.
5. **Düzenli Kalemler & Otomatik Takip**:
   - Maaş, kira, fatura ve dijital abonelikler (Netflix, Spotify vb.) her ay `BEKLENİYOR` durumunda listelenir.
   - Tek dokunuşla `GERÇEKLEŞTİ` işaretlenebilir veya otomatik gerçekleşme tanımlanabilir.
6. **Günlük Portföy Kapanışları & Grafikler**:
   - Günde bir kez otomatik finansal durum özeti (`portfolio_snapshots`) kaydedilir.
   - Aylık nakit akışı ve net varlık geçmişi hafif SVG grafikleriyle gösterilir.
7. **PWA (Progressive Web App)**:
   - Telefona uygulama gibi yüklenebilir, tam ekran çalışır.
8. **Güvenli Yedekleme & Dışa Aktarma**:
   - SQLite `VACUUM INTO` ile canlı ve tutarlı günlük yedekleme.
   - CSV ve tam JSON dışa aktarma desteği.

---

## Mimari ve Teknoloji Yığını

- **Backend**: Go (Go 1.25+)
- **Veritabanı**: SQLite (WAL modunda, CGO'suz `modernc.org/sqlite`)
- **Frontend**: Sunucu taraflı HTML şablonları (`html/template`), HTMX, Tailwind CSS
- **Hassas Aritmetik**: `github.com/shopspring/decimal` (Kayan nokta / IEEE-754 yuvarlama hatası yoktur)
- **Güvenlik**: Argon2id parola şifreleme, HTTP-only oturum çerezi, CSRF koruması, IP bazlı brute-force hız sınırlayıcı.
- **Dağıtım**: Tek bir Alpine tabanlı Docker konteyneri.

---

## ZeroTier Özel Ağında Dağıtım

Ledger, genel internete açık bir web sitesi değildir. Ev sunucunda çalışır ve sadece ZeroTier ağına bağlı cihazlarından erişilir.

```
TELEFON (ZeroTier İstemcisi: 10.147.17.x)
   │
[ZeroTier Sanal Ağı]
   │
EV SUNUCUSU (ZeroTier IP: 10.147.17.1)
   │
Docker Konteyneri (10.147.17.1:8085) -> Ledger
```

### Kurulum Adımları:

1. Depoyu sunucuna klonla:
   ```bash
   git clone <repository_url> ledger
   cd ledger
   ```

2. Ortam değişkenlerini yapılandır:
   ```bash
   cp .env.example .env
   nano .env
   ```
   `.env` dosyasında `ZEROTIER_IP` değerine sunucunun ZeroTier IP adresini gir:
   ```ini
   ZEROTIER_IP=10.147.17.1
   APP_PORT=8085
   ```

3. Konteyneri başlat:
   ```bash
   docker compose up -d --build
   ```

4. Tarayıcından eriş:
   ```
   http://ZEROTIER_IP:8085
   ```
   İlk ziyarette otomatik olarak **Yönetici Hesabı Oluşturma** ve **Kurulum Sihirbazı** açılacaktır!

---

## Güncelleme Deneyimi

Sistemi güncellemek için hazır script'i çalıştırman yeterlidir:
```bash
./scripts/update.sh
```
Bu komut sırasıyla:
1. Veritabanının güncelleme öncesi güvenli bir yedeğini alır (`./data/backups/`).
2. `git pull` ile son kodları çeker.
3. Docker konteynerini sıfır veri kaybı ile yeniden derler ve başlatır.

---

## Yedekleme ve Geri Yükleme

### Otomatik Yedekleme:
Ledger her gece saat 23:00'te ve ayarlar menüsünden manuel olarak tetiklendiğinde `./data/backups/` altına tarihli SQLite yedeği alır. Son 30 gün otomatik olarak korunur.

### Manuel Yedek Alma:
```bash
./scripts/backup.sh
```

### Yedekten Geri Yükleme:
```bash
./scripts/restore.sh ./data/backups/ledger_YYYYMMDD_HHMMSS.db
```

---

## Yerel Geliştirme (Local Development)

Yerel ortamda Docker olmadan doğrudan Go ile çalıştırmak için:

```bash
# Bağımlılıkları kontrol et
go mod tidy

# Testleri çalıştır
go test -v ./...

# Uygulamayı başlat
go run ./cmd/ledger
```
Tarayıcında `http://localhost:8085` adresini aç.

---

## Piyasa Verisi Sağlayıcıları (Özet)

Detaylı teknik bilgi için [docs/market-data.md](docs/market-data.md) belgesini inceleyin.

- **Döviz**: TCMB (`tcmb.gov.tr/kurlar/today.xml`)
- **TEFAS Fonları & PPF**: TEFAS JSON API (`tefas.gov.tr/api/funds/fonGnlBlgSiraliGetir`)
- **ABD Hisseleri & ETF**: Yahoo Finance v8 Chart API
- **Borsa İstanbul (BIST)**: Yahoo Finance (`.IS` uzantısı ile)
- **Gram Altın**: Ons Altın Spot (`GC=F`) × (USD/TRY) / 31.1034768
