# LEDGER Market Data Documentation

## Overview
Ledger adopts a privacy-first, zero-maintenance market data architecture. It avoids brittle scraping or expensive subscription requirements by utilizing official and well-established public financial data interfaces, backed by multi-level caching and graceful offline fallback.

---

## Providers & Market Coverage

### 1. Foreign Exchange (FX)
- **Primary Provider**: Türkiye Cumhuriyet Merkez Bankası (TCMB) Daily Reference Rates.
- **Endpoint**: `https://www.tcmb.gov.tr/kurlar/today.xml`
- **Why**: Official central bank rate, zero API keys, published every business day at 15:30 TRT, highly reliable.
- **Coverage**: USD/TRY, EUR/TRY, GBP/TRY, CHF/TRY, JPY/TRY, CAD/TRY, AUD/TRY, and cross-rates.
- **Secondary Fallback**: Yahoo Finance FX ticker pairs (`USDTRY=X`, `EURTRY=X`, `GBPTRY=X`).
- **Cache TTL**: 60 minutes.

### 2. Turkish Mutual Funds & Money Market Funds (TEFAS / PPF)
- **Primary Provider**: Türkiye Elektronik Fon Alım Satım Platformu (TEFAS) official JSON API.
- **Endpoint**: `https://www.tefas.gov.tr/api/funds/fonGnlBlgSiraliGetir`
- **Why**: Official Takasbank/TEFAS data platform. Delivers exact daily NAV (Net Asset Value), official title, and date.
- **Coverage**: All mutual funds traded on TEFAS (e.g. `TP2`, `TI2`, `MAC`, `NNF`, money market funds, gold funds).
- **Search**: Supports fund lookup via `aramaMetni` query parameter.
- **Cache TTL**: 24 hours (funds publish new NAV once per business day).

### 3. US Stocks & Global ETFs
- **Primary Provider**: Yahoo Finance v8 Chart API.
- **Endpoint**: `https://query1.finance.yahoo.com/v8/finance/chart/{symbol}`
- **Why**: High coverage of US exchanges (NYSE, NASDAQ), ETFs (VOO, SPY, QQQ), real-time pricing during market hours, zero API key configuration needed.
- **Coverage**: All US equities, ADRs, global ETFs.
- **Cache TTL**: 15–30 minutes during market open.

### 4. Borsa İstanbul (BIST)
- **Primary Provider**: Yahoo Finance with `.IS` ticker suffix.
- **Endpoint**: `https://query1.finance.yahoo.com/v8/finance/chart/{symbol}.IS`
- **Why**: Native support for Borsa İstanbul tickers (e.g. `THYAO.IS`, `GARAN.IS`, `KCHOL.IS`) with prices quoted directly in TRY.
- **Cache TTL**: 15–30 minutes during market open.

### 5. Precious Metals (Gold / Altın)
- **Primary Provider**: Derived from international gold spot futures (`GC=F` in USD/troy oz) multiplied by TCMB USD/TRY divided by 31.1034768 grams to obtain the exact Gram Altın (TRY) unit price.
- **Fallback**: Manual price entry if desired.

---

## Limitations & Fallback Strategy

1. **Physical Real Estate & Vehicles**: Automated public valuation APIs do not exist in Turkey for individual private properties or second-hand cars without paid subscriptions. Ledger allows manual value tracking on the Assets page.
2. **Provider Downtime / Offline Server**:
   - If an external provider cannot be reached (e.g., home server internet drop or API rate limit), Ledger **never crashes** and **never sets the price to 0**.
   - It reads the last successfully fetched quote from SQLite `market_quotes`.
   - The UI clearly labels the price as stale: `"Son Fiyat: 15.09.2026 18:00 (Güncel fiyat alınamadı)"`.
   - The user can click `[Düzenle]` at any time to enter a manual override price.
