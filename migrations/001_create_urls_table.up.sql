-- +goose Up
-- +goose StatementBegin

CREATE TABLE IF NOT EXISTS urls (
    id BIGSERIAL PRIMARY KEY,
    original_url TEXT NOT NULL,
    short_code VARCHAR(10) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Уникальность: один оригинальный URL - одна короткая ссылка (идемпотентность)
CREATE UNIQUE INDEX IF NOT EXISTS urls_original_url_unique ON urls(original_url);

-- Уникальность короткого кода + быстрый поиск при редиректе
CREATE UNIQUE INDEX IF NOT EXISTS urls_short_code_unique ON urls(short_code);

-- +goose StatementEnd