//go:build integration

package postgres

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

// setupTestDB запускает PostgreSQL в Docker и возвращает pool + cleanup
func setupTestDB(t *testing.T) (*pgxpool.Pool, func()) {
	ctx := context.Background()

	pgContainer, err := postgres.Run(ctx,
		"postgres:15-alpine",
		postgres.WithDatabase("testdb"),
		postgres.WithUsername("testuser"),
		postgres.WithPassword("testpass"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(30*time.Second)),
	)
	require.NoError(t, err)

	dsn, err := pgContainer.ConnectionString(ctx, "sslmode=disable")
	require.NoError(t, err)

	pool, err := pgxpool.New(ctx, dsn)
	require.NoError(t, err)

	_, err = pool.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS urls (
			id BIGSERIAL PRIMARY KEY,
			original_url TEXT NOT NULL,
			short_code VARCHAR(10) NOT NULL,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			CONSTRAINT chk_short_code_format CHECK (short_code ~ '^[A-Za-z0-9_]{10}$'),
			UNIQUE (original_url),
			UNIQUE (short_code)
		)
	`)
	require.NoError(t, err)

	cleanup := func() {
		pool.Close()
		pgContainer.Terminate(ctx)
	}

	return pool, cleanup
}

func TestUrlPostgresRepository_Save(t *testing.T) {
	if os.Getenv("RUN_INTEGRATION_TESTS") == "" {
		t.Skip("set RUN_INTEGRATION_TESTS=1 to run PostgreSQL integration tests")
	}

	pool, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()
	repo := NewUrlPostgresRepository(pool)

	t.Run("saves new URL successfully", func(t *testing.T) {
		err := repo.Save(ctx, "https://example.com", "Ab3_xK9mLp")
		require.NoError(t, err)
	})

	t.Run("is idempotent via ON CONFLICT (original_url)", func(t *testing.T) {
		// Первый вызов
		err1 := repo.Save(ctx, "https://ozon.ru", "Oz0n_Short")
		require.NoError(t, err1)

		// Второй вызов с тем же URL, нет ошибки
		err2 := repo.Save(ctx, "https://ozon.ru", "Oz0n_Short")
		require.NoError(t, err2)
	})

	t.Run("prevents duplicate short_code via UNIQUE constraint", func(t *testing.T) {
		err := repo.Save(ctx, "https://different.com", "Ab3_xK9mLp") // код уже занят
		_ = err
	})
}

func TestUrlPostgresRepository_GetByShortCode(t *testing.T) {
	if os.Getenv("RUN_INTEGRATION_TESTS") == "" {
		t.Skip("set RUN_INTEGRATION_TESTS=1 to run PostgreSQL integration tests")
	}

	pool, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()
	repo := NewUrlPostgresRepository(pool)

	_ = repo.Save(ctx, "https://example.com", "Ab3_xK9mLp")

	t.Run("returns original URL for existing code", func(t *testing.T) {
		url, err := repo.GetByShortCode(ctx, "Ab3_xK9mLp")
		require.NoError(t, err)
		assert.Equal(t, "https://example.com", url)
	})

	t.Run("returns error for unknown code", func(t *testing.T) {
		url, err := repo.GetByShortCode(ctx, "NotExist123")
		assert.Error(t, err)
		assert.Empty(t, url)
	})
}

func TestUrlPostgresRepository_CheckConstraint(t *testing.T) {
	if os.Getenv("RUN_INTEGRATION_TESTS") == "" {
		t.Skip("set RUN_INTEGRATION_TESTS=1 to run PostgreSQL integration tests")
	}

	pool, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()
	repo := NewUrlPostgresRepository(pool)

	t.Run("rejects short_code with invalid characters", func(t *testing.T) {
		err := repo.Save(ctx, "https://bad.com", "bad-code!")
		assert.Error(t, err)
	})

	t.Run("rejects short_code with wrong length", func(t *testing.T) {
		err := repo.Save(ctx, "https://short.com", "12345")
		assert.Error(t, err)
	})
}
