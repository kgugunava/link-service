package memory

import (
	"context"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kgugunava/link-service/internal/domain"
)

// newTestRepo создаёт репозиторий с тихим логгером для тестов
func newTestRepo() *UrlMemoryRepository {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	return NewUrlMemoryRepository(logger)
}

// newTestURL создаёт тестовый URL с заданными параметрами
func newTestURL(originalURL, shortCode string, ttl time.Duration) *domain.URL {
	return &domain.URL{
		OriginalURL: originalURL,
		ShortCode:   shortCode,
		CreatedAt:   time.Now(),
		ExpiresAt:   time.Now().Add(ttl),
	}
}

// Тесты для Save

func TestUrlMemoryRepository_Save(t *testing.T) {
	ctx := context.Background()

	t.Run("saves new URL successfully", func(t *testing.T) {
		repo := newTestRepo()
		url := newTestURL("https://example.com", "Ab3_xK9mLp", time.Hour)

		err := repo.Save(ctx, url)
		require.NoError(t, err)
		assert.Equal(t, 1, repo.Size())
	})

	t.Run("idempotent: same original URL does not create duplicate", func(t *testing.T) {
		repo := newTestRepo()
		url1 := newTestURL("https://example.com", "Ab3_xK9mLp", time.Hour)
		url2 := newTestURL("https://example.com", "Xy9_mN2pQr", time.Hour) // другой код, тот же URL

		err1 := repo.Save(ctx, url1)
		err2 := repo.Save(ctx, url2)

		require.NoError(t, err1)
		require.NoError(t, err2)
		assert.Equal(t, 1, repo.Size()) // только первая запись сохранилась

		// Проверяем, что сохранился первый код
		got, err := repo.GetByShortCode(ctx, "Ab3_xK9mLp")
		require.NoError(t, err)
		assert.Equal(t, "https://example.com", got.OriginalURL)
	})

	t.Run("allows different URLs with different codes", func(t *testing.T) {
		repo := newTestRepo()

		err1 := repo.Save(ctx, newTestURL("https://a.com", "CodeAAAAA123", time.Hour))
		err2 := repo.Save(ctx, newTestURL("https://b.com", "CodeBBBBB456", time.Hour))

		require.NoError(t, err1)
		require.NoError(t, err2)
		assert.Equal(t, 2, repo.Size())
	})

	t.Run("preserves all URL fields", func(t *testing.T) {
		repo := newTestRepo()
		now := time.Now().Truncate(time.Second)
		url := &domain.URL{
			OriginalURL: "https://example.com/path",
			ShortCode:   "Ab3_xK9mLp",
			CreatedAt:   now,
			ExpiresAt:   now.Add(24 * time.Hour),
		}

		err := repo.Save(ctx, url)
		require.NoError(t, err)

		got, err := repo.GetByShortCode(ctx, "Ab3_xK9mLp")
		require.NoError(t, err)
		assert.Equal(t, url.OriginalURL, got.OriginalURL)
		assert.Equal(t, url.ShortCode, got.ShortCode)
		assert.Equal(t, url.CreatedAt, got.CreatedAt)
		assert.Equal(t, url.ExpiresAt, got.ExpiresAt)
	})
}

// Тесты для GetByShortCode

func TestUrlMemoryRepository_GetByShortCode(t *testing.T) {
	ctx := context.Background()

	t.Run("returns URL for existing code", func(t *testing.T) {
		repo := newTestRepo()
		url := newTestURL("https://example.com", "Ab3_xK9mLp", time.Hour)
		require.NoError(t, repo.Save(ctx, url))

		got, err := repo.GetByShortCode(ctx, "Ab3_xK9mLp")
		require.NoError(t, err)
		assert.Equal(t, "https://example.com", got.OriginalURL)
		assert.Equal(t, "Ab3_xK9mLp", got.ShortCode)
	})

	t.Run("returns ErrNotFound for unknown code", func(t *testing.T) {
		repo := newTestRepo()

		got, err := repo.GetByShortCode(ctx, "NotExist123")
		assert.ErrorIs(t, err, domain.ErrNotFound)
		assert.Nil(t, got)
	})

	t.Run("returns ErrNotFound for empty code", func(t *testing.T) {
		repo := newTestRepo()

		got, err := repo.GetByShortCode(ctx, "")
		assert.ErrorIs(t, err, domain.ErrNotFound)
		assert.Nil(t, got)
	})

	t.Run("returns URL when ExpiresAt is zero (no TTL)", func(t *testing.T) {
		repo := newTestRepo()
		url := &domain.URL{
			OriginalURL: "https://example.com",
			ShortCode:   "Ab3_xK9mLp",
			CreatedAt:   time.Now(),
			ExpiresAt:   time.Time{}, // zero value — без TTL
		}
		require.NoError(t, repo.Save(ctx, url))

		got, err := repo.GetByShortCode(ctx, "Ab3_xK9mLp")
		require.NoError(t, err)
		assert.Equal(t, "https://example.com", got.OriginalURL)
	})

	t.Run("returns ErrNotFound for expired URL", func(t *testing.T) {
		repo := newTestRepo()
		// Создаём URL, который уже просрочен 
		url := &domain.URL{
			OriginalURL: "https://example.com",
			ShortCode:   "Ab3_xK9mLp",
			CreatedAt:   time.Now().Add(-2 * time.Hour),
			ExpiresAt:   time.Now().Add(-1 * time.Hour), // уже истёк
		}
		require.NoError(t, repo.Save(ctx, url))

		got, err := repo.GetByShortCode(ctx, "Ab3_xK9mLp")
		assert.ErrorIs(t, err, domain.ErrNotFound)
		assert.Nil(t, got)
	})

	t.Run("returns URL for non-expired URL", func(t *testing.T) {
		repo := newTestRepo()
		url := newTestURL("https://example.com", "Ab3_xK9mLp", time.Hour)
		require.NoError(t, repo.Save(ctx, url))

		got, err := repo.GetByShortCode(ctx, "Ab3_xK9mLp")
		require.NoError(t, err)
		assert.Equal(t, "https://example.com", got.OriginalURL)
	})
}

// Тесты для DeleteExpired

func TestUrlMemoryRepository_DeleteExpired(t *testing.T) {
	ctx := context.Background()

	t.Run("deletes expired URLs and keeps valid ones", func(t *testing.T) {
		repo := newTestRepo()

		// 2 просроченных, 1 валидный
		expired1 := &domain.URL{
			OriginalURL: "https://expired1.com",
			ShortCode:   "Expired0001",
			ExpiresAt:   time.Now().Add(-1 * time.Hour),
		}
		expired2 := &domain.URL{
			OriginalURL: "https://expired2.com",
			ShortCode:   "Expired0002",
			ExpiresAt:   time.Now().Add(-30 * time.Minute),
		}
		valid := newTestURL("https://valid.com", "Valid00001", time.Hour)

		require.NoError(t, repo.Save(ctx, expired1))
		require.NoError(t, repo.Save(ctx, expired2))
		require.NoError(t, repo.Save(ctx, valid))
		assert.Equal(t, 3, repo.Size())

		// Запускаем очистку
		deleted, err := repo.DeleteExpired(ctx)
		require.NoError(t, err)
		assert.Equal(t, int64(2), deleted)
		assert.Equal(t, 1, repo.Size())

		// Проверяем, что валидный URL остался
		got, err := repo.GetByShortCode(ctx, "Valid00001")
		require.NoError(t, err)
		assert.Equal(t, "https://valid.com", got.OriginalURL)

		// Проверяем, что просроченные удалены
		_, err = repo.GetByShortCode(ctx, "Expired0001")
		assert.ErrorIs(t, err, domain.ErrNotFound)
		_, err = repo.GetByShortCode(ctx, "Expired0002")
		assert.ErrorIs(t, err, domain.ErrNotFound)
	})

	t.Run("does not delete URLs without TTL (ExpiresAt is zero)", func(t *testing.T) {
		repo := newTestRepo()
		noTTL := &domain.URL{
			OriginalURL: "https://no-ttl.com",
			ShortCode:   "NoTTL00001",
			ExpiresAt:   time.Time{}, // без TTL
		}
		require.NoError(t, repo.Save(ctx, noTTL))

		deleted, err := repo.DeleteExpired(ctx)
		require.NoError(t, err)
		assert.Equal(t, int64(0), deleted)
		assert.Equal(t, 1, repo.Size())
	})

	t.Run("returns 0 when no expired URLs", func(t *testing.T) {
		repo := newTestRepo()
		require.NoError(t, repo.Save(ctx, newTestURL("https://a.com", "CodeAAAAA123", time.Hour)))
		require.NoError(t, repo.Save(ctx, newTestURL("https://b.com", "CodeBBBBB456", time.Hour)))

		deleted, err := repo.DeleteExpired(ctx)
		require.NoError(t, err)
		assert.Equal(t, int64(0), deleted)
		assert.Equal(t, 2, repo.Size())
	})

	t.Run("returns 0 on empty repository", func(t *testing.T) {
		repo := newTestRepo()

		deleted, err := repo.DeleteExpired(ctx)
		require.NoError(t, err)
		assert.Equal(t, int64(0), deleted)
		assert.Equal(t, 0, repo.Size())
	})

	t.Run("cleans both maps consistently", func(t *testing.T) {
		repo := newTestRepo()
		expired := &domain.URL{
			OriginalURL: "https://expired.com",
			ShortCode:   "Expired0001",
			ExpiresAt:   time.Now().Add(-1 * time.Hour),
		}
		require.NoError(t, repo.Save(ctx, expired))

		_, err := repo.DeleteExpired(ctx)
		require.NoError(t, err)

		// Проверяем, что запись удалена из обеих мап
		newURL := &domain.URL{
			OriginalURL: "https://expired.com",
			ShortCode:   "NewCode0001",
			ExpiresAt:   time.Now().Add(time.Hour),
		}
		require.NoError(t, repo.Save(ctx, newURL))

		got, err := repo.GetByShortCode(ctx, "NewCode0001")
		require.NoError(t, err)
		assert.Equal(t, "https://expired.com", got.OriginalURL)
	})
}

// Тесты для Size

func TestUrlMemoryRepository_Size(t *testing.T) {
	ctx := context.Background()

	t.Run("returns 0 for empty repository", func(t *testing.T) {
		repo := newTestRepo()
		assert.Equal(t, 0, repo.Size())
	})

	t.Run("returns correct count after saves", func(t *testing.T) {
		repo := newTestRepo()

		require.NoError(t, repo.Save(ctx, newTestURL("https://a.com", "CodeAAAAA123", time.Hour)))
		assert.Equal(t, 1, repo.Size())

		require.NoError(t, repo.Save(ctx, newTestURL("https://b.com", "CodeBBBBB456", time.Hour)))
		assert.Equal(t, 2, repo.Size())

		require.NoError(t, repo.Save(ctx, newTestURL("https://c.com", "CodeCCCCC789", time.Hour)))
		assert.Equal(t, 3, repo.Size())
	})

	t.Run("does not increase on idempotent save", func(t *testing.T) {
		repo := newTestRepo()
		url := newTestURL("https://a.com", "CodeAAAAA123", time.Hour)

		require.NoError(t, repo.Save(ctx, url))
		require.NoError(t, repo.Save(ctx, url))
		assert.Equal(t, 1, repo.Size())
	})

	t.Run("decreases after DeleteExpired", func(t *testing.T) {
		repo := newTestRepo()
		require.NoError(t, repo.Save(ctx, &domain.URL{
			OriginalURL: "https://expired.com",
			ShortCode:   "Expired0001",
			ExpiresAt:   time.Now().Add(-1 * time.Hour),
		}))
		require.NoError(t, repo.Save(ctx, newTestURL("https://valid.com", "Valid00001", time.Hour)))
		assert.Equal(t, 2, repo.Size())

		_, err := repo.DeleteExpired(ctx)
		require.NoError(t, err)
		assert.Equal(t, 1, repo.Size())
	})
}

// Тесты на конкурентный доступ (запускать с -race)

func TestUrlMemoryRepository_Concurrency(t *testing.T) {
	ctx := context.Background()
	repo := newTestRepo()
	const goroutines = 100

	var wg sync.WaitGroup
	errs := make(chan error, goroutines*3)

	// Параллельные записи
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			url := &domain.URL{
				OriginalURL: "https://example.com/" + string(rune('a'+id%26)),
				ShortCode:   "Code" + string(rune('A'+id%26)) + "123456",
				ExpiresAt:   time.Now().Add(time.Hour),
			}
			errs <- repo.Save(ctx, url)
		}(i)
	}

	// Параллельные чтения
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			code := "Code" + string(rune('A'+id%26)) + "123456"
			_, err := repo.GetByShortCode(ctx, code)
			if err != nil && err != domain.ErrNotFound {
				errs <- err
			}
		}(i)
	}

	// Параллельные очистки
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := repo.DeleteExpired(ctx)
			if err != nil {
				errs <- err
			}
		}()
	}

	wg.Wait()
	close(errs)

	for err := range errs {
		require.NoError(t, err, "concurrent access caused error")
	}
}