package postgres

import (
	"context"
	"log/slog"
	"fmt"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/kgugunava/link-service/internal/domain"
)

type UrlPostgresRepository struct {
	pool   *pgxpool.Pool
	logger *slog.Logger
}

func NewUrlPostgresRepository(pool *pgxpool.Pool, logger *slog.Logger) *UrlPostgresRepository {
	if logger == nil {
		logger = slog.Default()
	}
	return &UrlPostgresRepository{
		pool:   pool,
		logger: logger,
	}
}

func (r *UrlPostgresRepository) Save(ctx context.Context, originalUrl, shortCode string) error {
	const q = `
		INSERT INTO urls (original_url, short_code)
		VALUES ($1, $2)
		ON CONFLICT (original_url) DO NOTHING
	`

	result, err := r.pool.Exec(ctx, q, originalUrl, shortCode)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			if pgErr.ConstraintName == "urls_short_code_key" {
				return fmt.Errorf("short_code_collision: %w", err)
			}
		}
		return err
	}

	if result.RowsAffected() == 0 {
		r.logger.Debug("url already exists, skipped insert", "original_url", originalUrl)
	} else {
		r.logger.Info("url saved", "original_url", originalUrl, "short_code", shortCode)
	}

	return nil
}

func (r *UrlPostgresRepository) GetByShortCode(ctx context.Context, shortCode string) (string, error) {
	const q = `SELECT original_url FROM urls WHERE short_code = $1`

	r.logger.Debug("executing select", "short_code", shortCode)

	var url string
	err := r.pool.QueryRow(ctx, q, shortCode).Scan(&url)
	if err != nil {
		if err == pgx.ErrNoRows {
			r.logger.Debug("short code not found", "short_code", shortCode)
			return "", domain.ErrNotFound
		}
		r.logger.Error("select failed", "short_code", shortCode, "error", err)
		return "", err
	}

	return url, nil
}