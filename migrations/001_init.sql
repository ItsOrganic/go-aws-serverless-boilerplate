-- Schema for the loan-matching service (Postgres 13+ for gen_random_uuid()).
BEGIN;

CREATE TABLE IF NOT EXISTS users (
    user_id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name              TEXT NOT NULL,
    email             TEXT NOT NULL UNIQUE,
    monthly_income    NUMERIC(14, 2) CHECK (monthly_income >= 0),
    credit_score      INTEGER CHECK (credit_score BETWEEN 300 AND 900),
    employment_status TEXT NOT NULL DEFAULT '',
    age               INTEGER CHECK (age BETWEEN 18 AND 120),
    created_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_users_created_at ON users (created_at);

CREATE TABLE IF NOT EXISTS csv_uploads (
    upload_id             UUID PRIMARY KEY,
    s3_key                TEXT NOT NULL UNIQUE,
    filename              TEXT NOT NULL,
    file_size             BIGINT NOT NULL DEFAULT 0,
    status                TEXT NOT NULL CHECK (status IN ('pending', 'processing', 'completed', 'partial', 'failed')),
    total_rows            INTEGER NOT NULL DEFAULT 0,
    processed_rows        INTEGER NOT NULL DEFAULT 0,
    failed_rows           INTEGER NOT NULL DEFAULT 0,
    uploaded_by           TEXT NOT NULL,
    upload_started_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    processing_started_at TIMESTAMPTZ,
    upload_completed_at   TIMESTAMPTZ,
    error_summary         TEXT
);

CREATE TABLE IF NOT EXISTS csv_upload_errors (
    error_id      UUID PRIMARY KEY,
    upload_id     UUID NOT NULL REFERENCES csv_uploads (upload_id) ON DELETE CASCADE,
    row_number    INTEGER NOT NULL DEFAULT 0,
    raw_data      TEXT,
    error_message TEXT NOT NULL,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_csv_upload_errors_upload_id ON csv_upload_errors (upload_id);

CREATE TABLE IF NOT EXISTS loan_products (
    product_id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    product_name       TEXT NOT NULL,
    lender_name        TEXT NOT NULL,
    interest_rate_min  NUMERIC(6, 2),
    interest_rate_max  NUMERIC(6, 2),
    loan_amount_min    NUMERIC(14, 2),
    loan_amount_max    NUMERIC(14, 2),
    min_monthly_income NUMERIC(14, 2),
    min_credit_score   INTEGER,
    max_credit_score   INTEGER,
    employment_types   TEXT[] NOT NULL DEFAULT '{}',
    min_age            INTEGER,
    max_age            INTEGER,
    source_url         TEXT,
    is_active          BOOLEAN NOT NULL DEFAULT TRUE,
    crawled_at         TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_at         TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at         TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (product_name, lender_name)
);

CREATE TABLE IF NOT EXISTS user_loan_matches (
    match_id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id            UUID NOT NULL REFERENCES users (user_id) ON DELETE CASCADE,
    product_id         UUID NOT NULL REFERENCES loan_products (product_id) ON DELETE CASCADE,
    match_score        DOUBLE PRECISION NOT NULL,
    eligibility_status TEXT NOT NULL CHECK (eligibility_status IN ('eligible', 'partially_eligible', 'not_eligible')),
    match_reasons      TEXT[] NOT NULL DEFAULT '{}',
    ai_analysis        TEXT,
    created_at         TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (user_id, product_id)
);
CREATE INDEX IF NOT EXISTS idx_user_loan_matches_created_at ON user_loan_matches (created_at);

CREATE TABLE IF NOT EXISTS user_notifications (
    notification_id        UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id                UUID NOT NULL REFERENCES users (user_id) ON DELETE CASCADE,
    notification_type      TEXT NOT NULL,
    email_subject          TEXT,
    email_body             TEXT,
    matched_products_count INTEGER NOT NULL DEFAULT 0,
    email_status           TEXT NOT NULL,
    created_at             TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

COMMIT;
