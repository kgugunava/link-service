package postgres

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/kgugunava/link-service/internal/domain"
)

func setupTestDB(t *testing.T) (*pgxpool.Pool, func()) {
	t.Helper()
	ctx := context.Background()

	pgContainer, err := postgres.Run(ctx,
		"postgres:15-alpine",
		postgres.WithDatabase("testdb"),
		postgres.WithUsername("test"),
		postgres.WithPassword("test"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(30*time.Second),
		),
	)
	require.NoError(t, err, "failed to start postgres container")

	dsn, err := pgContainer.ConnectionString(ctx, "sslmode=disable")
	require.NoError(t, err, "failed to get connection string")

	pool, err := pgxpool.New(ctx, dsn)
	require.NoError(t, err, "failed to create connection pool")

	_, err = pool.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS urls (
			id BIGSERIAL PRIMARY KEY,
			original_url TEXT NOT NULL,
			short_code VARCHAR(10) NOT NULL,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			expires_at TIMESTAMPTZ,
			
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
	`)
	require.NoError(t, err, "failed to create table")

	cleanup := func() {
		pool.Close()
		if err := pgContainer.Terminate(ctx); err != nil {
			t.Logf("failed to terminate container: %v", err)
		}
	}

	return pool, cleanup
}

func newTestRepo(t *testing.T) (*URLPostgresRepository, func()) {
	t.Helper()
	pool, cleanup := setupTestDB(t)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	repo := NewURLPostgresRepository(pool, logger)
	return repo, cleanup
}

func newTestURL(originalURL, shortCode string, ttl time.Duration) *domain.URL {
	now := time.Now().Truncate(time.Microsecond)
	url := &domain.URL{
		OriginalURL: originalURL,
		ShortCode:   shortCode,
		CreatedAt:   now,
	}
	if ttl > 0 {
		url.ExpiresAt = now.Add(ttl)
	}
	return url
}

// Тесты для Save

func TestURLPostgresRepository_Save(t *testing.T) {
	ctx := context.Background()

	t.Run("saves new URL successfully", func(t *testing.T) {
		repo, cleanup := newTestRepo(t)
		defer cleanup()

		url := newTestURL("https://example.com", "Ab3_xK9mLp", time.Hour)
		err := repo.Save(ctx, url)
		require.NoError(t, err)

		got, err := repo.GetByShortCode(ctx, "Ab3_xK9mLp")
		require.NoError(t, err)
		assert.Equal(t, url.OriginalURL, got.OriginalURL)
		assert.Equal(t, url.ShortCode, got.ShortCode)
	})

	t.Run("idempotent: same original URL does not create duplicate", func(t *testing.T) {
		repo, cleanup := newTestRepo(t)
		defer cleanup()

		url1 := newTestURL("https://example.com", "Ab3_xK9mLp", time.Hour)
		url2 := newTestURL("https://example.com", "Xy9_mN2pQr", time.Hour)

		err1 := repo.Save(ctx, url1)
		require.NoError(t, err1)

		err2 := repo.Save(ctx, url2)
		require.NoError(t, err2)

		got, err := repo.GetByShortCode(ctx, "Ab3_xK9mLp")
		require.NoError(t, err)
		assert.Equal(t, "https://example.com", got.OriginalURL)

		_, err = repo.GetByShortCode(ctx, "Xy9_mN2pQr")
		assert.ErrorIs(t, err, domain.ErrNotFound)
	})

	t.Run("returns collision error when short_code is taken", func(t *testing.T) {
		repo, cleanup := newTestRepo(t)
		defer cleanup()

		url1 := newTestURL("https://a.com", "Ab3_xK9mLp", time.Hour)
		require.NoError(t, repo.Save(ctx, url1))

		url2 := newTestURL("https://b.com", "Ab3_xK9mLp", time.Hour)
		err := repo.Save(ctx, url2)

		require.Error(t, err)
		assert.Contains(t, err.Error(), "short_code_collision")
	})

	t.Run("preserves all URL fields including TTL", func(t *testing.T) {
		repo, cleanup := newTestRepo(t)
		defer cleanup()

		now := time.Now().Truncate(time.Microsecond)
		url := &domain.URL{
			OriginalURL: "https://example.com/path?query=1",
			ShortCode:   "Cd5_yL8nRq",
			CreatedAt:   now,
			ExpiresAt:   now.Add(24 * time.Hour),
		}

		err := repo.Save(ctx, url)
		require.NoError(t, err)

		got, err := repo.GetByShortCode(ctx, "Cd5_yL8nRq")
		require.NoError(t, err)
		assert.Equal(t, url.OriginalURL, got.OriginalURL)
		assert.Equal(t, url.ShortCode, got.ShortCode)
		assert.WithinDuration(t, url.CreatedAt, got.CreatedAt, time.Second)
		assert.WithinDuration(t, url.ExpiresAt, got.ExpiresAt, time.Second)
	})

	t.Run("allows NULL expires_at (no TTL)", func(t *testing.T) {
		repo, cleanup := newTestRepo(t)
		defer cleanup()

		url := &domain.URL{
			OriginalURL: "https://example.com",
			ShortCode:   "Ef7_zM0pSt",
			CreatedAt:   time.Now(),
			ExpiresAt:   time.Time{},
		}

		err := repo.Save(ctx, url)
		require.NoError(t, err)

		got, err := repo.GetByShortCode(ctx, "Ef7_zM0pSt")
		require.NoError(t, err)
		assert.True(t, got.ExpiresAt.IsZero(), "expires_at should be zero")
	})

	t.Run("rejects invalid short_code format (CHECK constraint)", func(t *testing.T) {
		repo, cleanup := newTestRepo(t)
		defer cleanup()

		url := &domain.URL{
			OriginalURL: "https://example.com",
			ShortCode:   "bad-code!",
			CreatedAt:   time.Now(),
			ExpiresAt:   time.Now().Add(time.Hour),
		}

		err := repo.Save(ctx, url)
		require.Error(t, err, "should fail CHECK constraint")
	})

	t.Run("rejects short_code without all character types", func(t *testing.T) {
		repo, cleanup := newTestRepo(t)
		defer cleanup()

		url := &domain.URL{
			OriginalURL: "https://example.com",
			ShortCode:   "abcdefghij",
			CreatedAt:   time.Now(),
			ExpiresAt:   time.Now().Add(time.Hour),
		}

		err := repo.Save(ctx, url)
		require.Error(t, err, "should fail CHECK constraint for missing character types")
	})
}

// Тесты для GetByShortCode

func TestURLPostgresRepository_GetByShortCode(t *testing.T) {
	ctx := context.Background()

	t.Run("returns full URL model for existing code", func(t *testing.T) {
		repo, cleanup := newTestRepo(t)
		defer cleanup()

		url := newTestURL("https://example.com/path", "Ab3_xK9mLp", time.Hour)
		require.NoError(t, repo.Save(ctx, url))

		got, err := repo.GetByShortCode(ctx, "Ab3_xK9mLp")
		require.NoError(t, err)
		require.NotNil(t, got)
		assert.Equal(t, "https://example.com/path", got.OriginalURL)
		assert.Equal(t, "Ab3_xK9mLp", got.ShortCode)
		assert.False(t, got.CreatedAt.IsZero())
		assert.False(t, got.ExpiresAt.IsZero())
	})

	t.Run("returns ErrNotFound for unknown code", func(t *testing.T) {
		repo, cleanup := newTestRepo(t)
		defer cleanup()

		got, err := repo.GetByShortCode(ctx, "Zz9_wX8vUt")
		assert.ErrorIs(t, err, domain.ErrNotFound)
		assert.Nil(t, got)
	})

	t.Run("returns ErrNotFound for empty code", func(t *testing.T) {
		repo, cleanup := newTestRepo(t)
		defer cleanup()

		got, err := repo.GetByShortCode(ctx, "")
		assert.ErrorIs(t, err, domain.ErrNotFound)
		assert.Nil(t, got)
	})

	t.Run("returns URL even if expired (TTL check is in service layer)", func(t *testing.T) {
		repo, cleanup := newTestRepo(t)
		defer cleanup()

		url := &domain.URL{
			OriginalURL: "https://expired.com",
			ShortCode:   "Gh1_aB2cDe",
			CreatedAt:   time.Now().Add(-2 * time.Hour),
			ExpiresAt:   time.Now().Add(-1 * time.Hour),
		}
		require.NoError(t, repo.Save(ctx, url))

		got, err := repo.GetByShortCode(ctx, "Gh1_aB2cDe")
		require.NoError(t, err)
		require.NotNil(t, got)
		assert.Equal(t, "https://expired.com", got.OriginalURL)
		assert.True(t, got.ExpiresAt.Before(time.Now()), "should be expired")
	})
}

// Тесты для DeleteExpired

func TestURLPostgresRepository_DeleteExpired(t *testing.T) {
	ctx := context.Background()

	t.Run("deletes expired URLs and keeps valid ones", func(t *testing.T) {
		repo, cleanup := newTestRepo(t)
		defer cleanup()

		expired1 := &domain.URL{
			OriginalURL: "https://expired1.com",
			ShortCode:   "Ij3_kL4mNo",
			CreatedAt:   time.Now().Add(-2 * time.Hour),
			ExpiresAt:   time.Now().Add(-1 * time.Hour),
		}
		expired2 := &domain.URL{
			OriginalURL: "https://expired2.com",
			ShortCode:   "Pq5_rS6tUv",
			CreatedAt:   time.Now().Add(-90 * time.Minute),
			ExpiresAt:   time.Now().Add(-30 * time.Minute),
		}
		valid := newTestURL("https://valid.com", "Wx7_yZ8aBc", time.Hour)

		require.NoError(t, repo.Save(ctx, expired1))
		require.NoError(t, repo.Save(ctx, expired2))
		require.NoError(t, repo.Save(ctx, valid))

		deleted, err := repo.DeleteExpired(ctx)
		require.NoError(t, err)
		assert.Equal(t, int64(2), deleted)

		got, err := repo.GetByShortCode(ctx, "Wx7_yZ8aBc")
		require.NoError(t, err)
		assert.Equal(t, "https://valid.com", got.OriginalURL)

		_, err = repo.GetByShortCode(ctx, "Ij3_kL4mNo")
		assert.ErrorIs(t, err, domain.ErrNotFound)
		_, err = repo.GetByShortCode(ctx, "Pq5_rS6tUv")
		assert.ErrorIs(t, err, domain.ErrNotFound)
	})

	t.Run("does not delete URLs without TTL (expires_at IS NULL)", func(t *testing.T) {
		repo, cleanup := newTestRepo(t)
		defer cleanup()

		noTTL := &domain.URL{
			OriginalURL: "https://no-ttl.com",
			ShortCode:   "De9_fG0hIj",
			CreatedAt:   time.Now(),
			ExpiresAt:   time.Time{},
		}
		require.NoError(t, repo.Save(ctx, noTTL))

		deleted, err := repo.DeleteExpired(ctx)
		require.NoError(t, err)
		assert.Equal(t, int64(0), deleted)

		got, err := repo.GetByShortCode(ctx, "De9_fG0hIj")
		require.NoError(t, err)
		assert.Equal(t, "https://no-ttl.com", got.OriginalURL)
	})

	t.Run("returns 0 when no expired URLs", func(t *testing.T) {
		repo, cleanup := newTestRepo(t)
		defer cleanup()

		require.NoError(t, repo.Save(ctx, newTestURL("https://a.com", "Kl1_mN2oPq", time.Hour)))
		require.NoError(t, repo.Save(ctx, newTestURL("https://b.com", "Rs3_tU4vWx", time.Hour)))

		deleted, err := repo.DeleteExpired(ctx)
		require.NoError(t, err)
		assert.Equal(t, int64(0), deleted)
	})

	t.Run("returns 0 on empty table", func(t *testing.T) {
		repo, cleanup := newTestRepo(t)
		defer cleanup()

		deleted, err := repo.DeleteExpired(ctx)
		require.NoError(t, err)
		assert.Equal(t, int64(0), deleted)
	})

	t.Run("idempotent: running twice does not error", func(t *testing.T) {
		repo, cleanup := newTestRepo(t)
		defer cleanup()

		expired := &domain.URL{
			OriginalURL: "https://expired.com",
			ShortCode:   "Yz5_aB6cDe",
			CreatedAt:   time.Now().Add(-2 * time.Hour),
			ExpiresAt:   time.Now().Add(-1 * time.Hour),
		}
		require.NoError(t, repo.Save(ctx, expired))

		deleted1, err := repo.DeleteExpired(ctx)
		require.NoError(t, err)
		assert.Equal(t, int64(1), deleted1)

		deleted2, err := repo.DeleteExpired(ctx)
		require.NoError(t, err)
		assert.Equal(t, int64(0), deleted2)
	})
}

// Интеграционный тест: полный жизненный цикл URL

func TestURLPostgresRepository_FullLifecycle(t *testing.T) {
	ctx := context.Background()
	repo, cleanup := newTestRepo(t)
	defer cleanup()

	url := &domain.URL{
		OriginalURL: "https://example.com/short-lived",
		ShortCode:   "Fg7_hI8jKl",
		CreatedAt:   time.Now(),
		ExpiresAt:   time.Now().Add(50 * time.Millisecond),
	}
	require.NoError(t, repo.Save(ctx, url))

	got, err := repo.GetByShortCode(ctx, "Fg7_hI8jKl")
	require.NoError(t, err)
	assert.Equal(t, "https://example.com/short-lived", got.OriginalURL)

	time.Sleep(100 * time.Millisecond)

	got, err = repo.GetByShortCode(ctx, "Fg7_hI8jKl")
	require.NoError(t, err)
	assert.True(t, got.ExpiresAt.Before(time.Now()))

	deleted, err := repo.DeleteExpired(ctx)
	require.NoError(t, err)
	assert.Equal(t, int64(1), deleted)

	_, err = repo.GetByShortCode(ctx, "Fg7_hI8jKl")
	assert.ErrorIs(t, err, domain.ErrNotFound)

	newURL := &domain.URL{
		OriginalURL: "https://example.com/reused",
		ShortCode:   "Fg7_hI8jKl",
		CreatedAt:   time.Now(),
		ExpiresAt:   time.Now().Add(time.Hour),
	}
	require.NoError(t, repo.Save(ctx, newURL))

	got, err = repo.GetByShortCode(ctx, "Fg7_hI8jKl")
	require.NoError(t, err)
	assert.Equal(t, "https://example.com/reused", got.OriginalURL)
}

// Тест коллизии short_code с fallback

func TestURLPostgresRepository_CollisionHandling(t *testing.T) {
	ctx := context.Background()
	repo, cleanup := newTestRepo(t)
	defer cleanup()

	url1 := newTestURL("https://first.com", "Ab3_xK9mLp", time.Hour)
	require.NoError(t, repo.Save(ctx, url1))

	url2 := newTestURL("https://second.com", "Ab3_xK9mLp", time.Hour)
	err := repo.Save(ctx, url2)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "short_code_collision")

	got, err := repo.GetByShortCode(ctx, "Ab3_xK9mLp")
	require.NoError(t, err)
	assert.Equal(t, "https://first.com", got.OriginalURL)

	url3 := newTestURL("https://second.com", "Xy9_mN2pQr", time.Hour)
	require.NoError(t, repo.Save(ctx, url3))

	got, err = repo.GetByShortCode(ctx, "Xy9_mN2pQr")
	require.NoError(t, err)
	assert.Equal(t, "https://second.com", got.OriginalURL)
}

// Тест проверки ошибок

func TestURLPostgresRepository_ErrorWrapping(t *testing.T) {
	ctx := context.Background()
	repo, cleanup := newTestRepo(t)
	defer cleanup()

	t.Run("GetByShortCode returns domain.ErrNotFound (not pgx.ErrNoRows)", func(t *testing.T) {
		_, err := repo.GetByShortCode(ctx, "Zz9_wX8vUt")
		require.Error(t, err)
		assert.ErrorIs(t, err, domain.ErrNotFound)
	})

	t.Run("Save collision error can be detected via string match", func(t *testing.T) {
		url1 := newTestURL("https://a.com", "Ab3_xK9mLp", time.Hour)
		require.NoError(t, repo.Save(ctx, url1))

		url2 := newTestURL("https://b.com", "Ab3_xK9mLp", time.Hour)
		err := repo.Save(ctx, url2)

		require.Error(t, err)
		assert.Contains(t, err.Error(), "short_code_collision")
	})
}
