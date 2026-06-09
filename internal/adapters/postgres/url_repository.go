package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

type UrlPostgresRepository struct {
	pool *pgxpool.Pool
}

func NewUrlPostgresRepository(pool *pgxpool.Pool) *UrlPostgresRepository {
	return &UrlPostgresRepository{
		pool: pool,
	}
}

func (r *UrlPostgresRepository) Save(ctx context.Context, originalUrl, shortCode string) error {
	const q = `
		INSERT INTO urls (original_url, short_code)
		VALUES ($1, $2)
		ON CONFLICT (original_url) DO NOTHING
	`
	if _, err := r.pool.Exec(ctx, q, originalUrl, shortCode); err != nil {
		return fmt.Errorf("postgres save: %w", err)
	}
	return nil
}

func (r *UrlPostgresRepository) GetByShortCode(ctx context.Context, shortCode string) (string, error) {
	const q = `SELECT original_url FROM urls WHERE short_code = $1`
	var url string
	err := r.pool.QueryRow(ctx, q, shortCode).Scan(&url)
	if err != nil {
		// pgx возвращает pgx.ErrNoRows, который нужно мапить в доменную ошибку
		return "", fmt.Errorf("postgres get: %w", err)
	}
	return url, nil
}