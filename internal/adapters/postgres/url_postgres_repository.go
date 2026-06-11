package postgres

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/kgugunava/link-service/internal/domain"
)

type URLPostgresRepository struct {
	pool   *pgxpool.Pool
	logger *slog.Logger
}

func NewURLPostgresRepository(pool *pgxpool.Pool, logger *slog.Logger) *URLPostgresRepository {
	if logger == nil {
		logger = slog.Default()
	}
	return &URLPostgresRepository{
		pool:   pool,
		logger: logger,
	}
}

func (r *URLPostgresRepository) Save(ctx context.Context, url *domain.URL) error {
	const q = `
		INSERT INTO urls (original_url, short_code, created_at, expires_at)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (original_url) DO NOTHING
	`

	r.logger.Debug("executing insert", "original_url", url.OriginalURL, "short_code", url.ShortCode)

	var expiresAt any
	if url.ExpiresAt.IsZero() {
		expiresAt = nil
	} else {
		expiresAt = url.ExpiresAt
	}

	result, err := r.pool.Exec(ctx, q, url.OriginalURL, url.ShortCode, url.CreatedAt, expiresAt)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			if pgErr.ConstraintName == "urls_short_code_unique" {
				r.logger.Warn("short code collision detected", "short_code", url.ShortCode, "original_url", url.OriginalURL)
				return fmt.Errorf("short_code_collision: %w", err)
			}
		}
		r.logger.Error("insert failed", "original_url", url.OriginalURL, "short_code", url.ShortCode, "error", err)
		return err
	}

	if result.RowsAffected() == 0 {
		r.logger.Debug("url already exists, skipped insert", "original_url", url.OriginalURL)
	} else {
		r.logger.Info("url saved", "original_url", url.OriginalURL, "short_code", url.ShortCode, "expires_at", expiresAt)
	}

	return nil
}

func (r *URLPostgresRepository) GetByShortCode(ctx context.Context, shortCode string) (*domain.URL, error) {
	const q = `
		SELECT original_url, short_code, created_at, expires_at
		FROM urls
		WHERE short_code = $1
	`

	r.logger.Debug("executing select", "short_code", shortCode)

	var (
		url       domain.URL
		expiresAt *time.Time
	)
	err := r.pool.QueryRow(ctx, q, shortCode).Scan(
		&url.OriginalURL,
		&url.ShortCode,
		&url.CreatedAt,
		&expiresAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			r.logger.Debug("short code not found", "short_code", shortCode)
			return nil, domain.ErrNotFound
		}
		r.logger.Error("select failed", "short_code", shortCode, "error", err)
		return nil, err
	}

	if expiresAt != nil {
		url.ExpiresAt = *expiresAt
	}

	r.logger.Debug("url retrieved", "short_code", shortCode, "original_url", url.OriginalURL)
	return &url, nil
}

func (r *URLPostgresRepository) DeleteExpired(ctx context.Context) (int64, error) {
	const q = `
		DELETE FROM urls
		WHERE expires_at IS NOT NULL AND expires_at < NOW()
	`

	r.logger.Debug("executing delete expired")

	result, err := r.pool.Exec(ctx, q)
	if err != nil {
		r.logger.Error("delete expired failed", "error", err)
		return 0, err
	}

	deleted := result.RowsAffected()
	if deleted > 0 {
		r.logger.Info("expired urls deleted", "count", deleted)
	}

	return deleted, nil
}