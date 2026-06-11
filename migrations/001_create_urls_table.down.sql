DROP INDEX IF EXISTS urls_expires_at_idx;

DROP INDEX IF EXISTS urls_short_code_unique;
DROP INDEX IF EXISTS urls_original_url_unique;
DROP TABLE IF EXISTS urls;