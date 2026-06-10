package memory

import (
	"context"
	"sync"
	"testing"
	"log/slog"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/kgugunava/link-service/internal/domain"
)

func TestUrlMemoryRepository_Save(t *testing.T) {
	ctx := context.Background()
	repo := NewUrlMemoryRepository(slog.Default())

	t.Run("saves new URL successfully", func(t *testing.T) {
		err := repo.Save(ctx, "https://example.com", "Ab3_xK9mLp")
		require.NoError(t, err)
	})

	t.Run("is idempotent: same URL does not error on duplicate save", func(t *testing.T) {
		err1 := repo.Save(ctx, "https://ozon.ru", "Oz0n_Short")
		err2 := repo.Save(ctx, "https://ozon.ru", "Oz0n_Short") // повторный вызов
		require.NoError(t, err1)
		require.NoError(t, err2)
	})

	t.Run("allows different URLs with different codes", func(t *testing.T) {
		err1 := repo.Save(ctx, "https://a.com", "CodeAAAAA123")
		err2 := repo.Save(ctx, "https://b.com", "CodeBBBBB456")
		require.NoError(t, err1)
		require.NoError(t, err2)
	})
}

func TestUrlMemoryRepository_GetByShortCode(t *testing.T) {
	ctx := context.Background()
	repo := NewUrlMemoryRepository(slog.Default())

	// Предзаполняем данными
	_ = repo.Save(ctx, "https://example.com", "Ab3_xK9mLp")

	t.Run("returns original URL for existing code", func(t *testing.T) {
		url, err := repo.GetByShortCode(ctx, "Ab3_xK9mLp")
		require.NoError(t, err)
		assert.Equal(t, "https://example.com", url)
	})

	t.Run("returns ErrNotFound for unknown code", func(t *testing.T) {
		url, err := repo.GetByShortCode(ctx, "NotExist123")
		assert.ErrorIs(t, err, domain.ErrNotFound)
		assert.Empty(t, url)
	})

	t.Run("empty code returns ErrNotFound", func(t *testing.T) {
		_, err := repo.GetByShortCode(ctx, "")
		assert.ErrorIs(t, err, domain.ErrNotFound)
	})
}

func TestUrlMemoryRepository_Concurrency(t *testing.T) {
	ctx := context.Background()
	repo := NewUrlMemoryRepository(slog.Default())
	const goroutines = 100

	var wg sync.WaitGroup
	errs := make(chan error, goroutines*2)

	// Параллельные записи и чтения
	for i := 0; i < goroutines; i++ {
		wg.Add(2)
		
		go func(id int) {
			defer wg.Done()
			url := "https://example.com/" + string(rune('a'+id%26))
			code := "Code" + string(rune('A'+id%26)) + "123456"
			errs <- repo.Save(ctx, url, code)
		}(i)
		
		go func(id int) {
			defer wg.Done()
			code := "Code" + string(rune('A'+id%26)) + "123456"
			_, err := repo.GetByShortCode(ctx, code)
			// Ожидаем либо успех, либо ErrNotFound (если Save ещё не отработал)
			if err != nil && err != domain.ErrNotFound {
				errs <- err
			}
		}(i)
	}

	wg.Wait()
	close(errs)

	for err := range errs {
		require.NoError(t, err, "concurrent access caused error")
	}
}

func TestUrlMemoryRepository_Idempotency_GetAfterSave(t *testing.T) {
	ctx := context.Background()
	repo := NewUrlMemoryRepository(slog.Default())

	url := "https://test.com/path"
	code := "TestIdemp123"

	// Save → Get → Save again → Get again
	err := repo.Save(ctx, url, code)
	require.NoError(t, err)

	got1, err := repo.GetByShortCode(ctx, code)
	require.NoError(t, err)
	assert.Equal(t, url, got1)

	err = repo.Save(ctx, url, code) // повторный Save
	require.NoError(t, err)

	got2, err := repo.GetByShortCode(ctx, code)
	require.NoError(t, err)
	assert.Equal(t, url, got2)
}