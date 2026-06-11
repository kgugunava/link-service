CREATE TABLE IF NOT EXISTS urls (
    id BIGSERIAL PRIMARY KEY,
    original_url TEXT NOT NULL,
    short_code VARCHAR(10) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX IF NOT EXISTS urls_original_url_unique ON urls(original_url);

CREATE UNIQUE INDEX IF NOT EXISTS urls_short_code_unique ON urls(short_code);