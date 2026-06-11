-- +goose Up
-- +goose StatementBegin

CREATE TABLE IF NOT EXISTS urls (
    id BIGSERIAL PRIMARY KEY,
    original_url TEXT NOT NULL,
    short_code VARCHAR(10) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    expires_at TIMESTAMPTZ,  -- NULL означает "без срока действия"
    
    CONSTRAINT chk_short_code_format CHECK (short_code ~ '^[A-Za-z0-9_]{10}$'),
    CONSTRAINT chk_short_code_types CHECK (
        short_code ~ '[a-z]' AND
        short_code ~ '[A-Z]' AND
        short_code ~ '[0-9]' AND
        short_code ~ '_'
    )
);

CREATE UNIQUE INDEX IF NOT EXISTS urls_original_url_unique ON urls(original_url);
CREATE UNIQUE INDEX IF NOT EXISTS urls_short_code_unique ON urls(short_code);

CREATE INDEX IF NOT EXISTS urls_expires_at_idx ON urls(expires_at) WHERE expires_at IS NOT NULL;