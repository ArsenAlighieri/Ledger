-- 001_initial_schema.sql
CREATE TABLE IF NOT EXISTS users (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    username TEXT NOT NULL UNIQUE,
    password_hash TEXT NOT NULL,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS sessions (
    token TEXT PRIMARY KEY,
    user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    expires_at DATETIME NOT NULL,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS settings (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    setup_completed BOOLEAN NOT NULL DEFAULT 0,
    setup_step INTEGER NOT NULL DEFAULT 1,
    base_currency TEXT NOT NULL DEFAULT 'TRY',
    emergency_fund_target TEXT NOT NULL DEFAULT '50000.00',
    emergency_fund_asset_type TEXT NOT NULL DEFAULT '',
    emergency_fund_asset_id INTEGER DEFAULT 0,
    monthly_investment_target TEXT NOT NULL DEFAULT '0.00',
    fx_ttl_minutes INTEGER NOT NULL DEFAULT 60,
    stock_ttl_minutes INTEGER NOT NULL DEFAULT 30,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS accounts (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL,
    institution TEXT NOT NULL DEFAULT '',
    account_type TEXT NOT NULL DEFAULT 'bank', -- 'bank', 'cash', 'foreign_currency'
    currency TEXT NOT NULL DEFAULT 'TRY',
    balance TEXT NOT NULL DEFAULT '0.00',
    is_archived BOOLEAN NOT NULL DEFAULT 0,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS debts (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL,
    debt_type TEXT NOT NULL DEFAULT 'credit_card', -- 'credit_card', 'loan', 'other'
    bank TEXT NOT NULL DEFAULT '',
    current_debt TEXT NOT NULL DEFAULT '0.00',
    statement_debt TEXT NOT NULL DEFAULT '0.00',
    due_day INTEGER DEFAULT 0,
    note TEXT NOT NULL DEFAULT '',
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS categories (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL,
    type TEXT NOT NULL, -- 'expense', 'income'
    icon TEXT NOT NULL DEFAULT 'tag',
    is_default BOOLEAN NOT NULL DEFAULT 1
);

CREATE TABLE IF NOT EXISTS transactions (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    type TEXT NOT NULL, -- 'expense', 'income'
    amount TEXT NOT NULL,
    category_id INTEGER REFERENCES categories(id) ON DELETE SET NULL,
    account_id INTEGER REFERENCES accounts(id) ON DELETE SET NULL,
    date TEXT NOT NULL, -- YYYY-MM-DD
    description TEXT NOT NULL DEFAULT '',
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS recurring_items (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    title TEXT NOT NULL,
    type TEXT NOT NULL, -- 'income', 'expense', 'subscription'
    amount TEXT NOT NULL,
    currency TEXT NOT NULL DEFAULT 'TRY',
    day_of_month INTEGER NOT NULL DEFAULT 1,
    frequency TEXT NOT NULL DEFAULT 'monthly', -- 'monthly', 'yearly'
    auto_realize BOOLEAN NOT NULL DEFAULT 0,
    is_active BOOLEAN NOT NULL DEFAULT 1,
    last_realized_at TEXT DEFAULT NULL,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS instruments (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    symbol TEXT NOT NULL UNIQUE,
    name TEXT NOT NULL,
    asset_type TEXT NOT NULL, -- 'stock', 'etf', 'fund', 'gold', 'currency', 'other'
    market TEXT NOT NULL DEFAULT '', -- 'BIST', 'US', 'TEFAS', 'COMMODITY', 'FX', 'OTHER'
    currency TEXT NOT NULL DEFAULT 'TRY',
    provider TEXT NOT NULL DEFAULT '',
    is_manual BOOLEAN NOT NULL DEFAULT 0,
    manual_price TEXT NOT NULL DEFAULT '0.00',
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS positions (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    instrument_id INTEGER NOT NULL UNIQUE REFERENCES instruments(id) ON DELETE CASCADE,
    quantity TEXT NOT NULL DEFAULT '0',
    average_cost TEXT NOT NULL DEFAULT '0.00',
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS market_quotes (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    instrument_id INTEGER NOT NULL UNIQUE REFERENCES instruments(id) ON DELETE CASCADE,
    price TEXT NOT NULL DEFAULT '0.00',
    currency TEXT NOT NULL DEFAULT 'TRY',
    quoted_at DATETIME NOT NULL,
    fetched_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    provider TEXT NOT NULL DEFAULT '',
    is_stale BOOLEAN NOT NULL DEFAULT 0
);

CREATE TABLE IF NOT EXISTS historical_prices (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    instrument_id INTEGER NOT NULL REFERENCES instruments(id) ON DELETE CASCADE,
    price_date TEXT NOT NULL, -- YYYY-MM-DD
    close_price TEXT NOT NULL,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(instrument_id, price_date)
);

CREATE TABLE IF NOT EXISTS portfolio_snapshots (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    snapshot_date TEXT NOT NULL UNIQUE, -- YYYY-MM-DD
    total_assets_base TEXT NOT NULL DEFAULT '0.00',
    investments_base TEXT NOT NULL DEFAULT '0.00',
    cash_base TEXT NOT NULL DEFAULT '0.00',
    debt_base TEXT NOT NULL DEFAULT '0.00',
    net_worth_base TEXT NOT NULL DEFAULT '0.00',
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
