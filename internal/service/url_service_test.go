package service

import (
	"context"
	"errors"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/kgugunava/link-service/internal/domain"
	"github.com/kgugunava/link-service/internal/utils"
)

// mockRepo реализует service.UrlRepositoryInterface
type mockRepo struct {
	mock.Mock
}

func (m *mockRepo) Save(ctx context.Context, originalURL, shortCode string) error {
	args := m.Called(ctx, originalURL, shortCode)
	return args.Error(0)
}

func (m *mockRepo) GetByShortCode(ctx context.Context, shortCode string) (string, error) {
	args := m.Called(ctx, shortCode)
	return args.String(0), args.Error(1)
}

func TestUrlService_Shorten(t *testing.T) {
	gen := utils.NewGenerator(slog.Default())
	ctx := context.Background()

	t.Run("valid URL returns 10-char code", func(t *testing.T) {
		repo := new(mockRepo)
		repo.On("Save", ctx, "https://example.com", mock.Anything).Return(nil)

		svc := NewUrlService(repo, gen, slog.Default())
		code, err := svc.Shorten(ctx, "https://example.com")

		require.NoError(t, err)
		assert.Len(t, code, 10)
		repo.AssertExpectations(t)
	})

	t.Run("invalid URL returns ErrInvalidURL", func(t *testing.T) {
		repo := new(mockRepo)
		svc := NewUrlService(repo, gen, slog.Default())

		_, err := svc.Shorten(ctx, "not-a-url")
		assert.ErrorIs(t, err, domain.ErrInvalidURL)
	})

	t.Run("repo error is returned as-is", func(t *testing.T) {
		repo := new(mockRepo)
		repo.On("Save", ctx, "https://example.com", mock.Anything).Return(errors.New("db failed"))

		svc := NewUrlService(repo, gen, nil)
		_, err := svc.Shorten(ctx, "https://example.com")

		assert.Error(t, err)
		assert.Equal(t, "db failed", err.Error())
		repo.AssertExpectations(t)
	})
}

func TestUrlService_GetOriginal(t *testing.T) {
	gen := utils.NewGenerator(slog.Default())
	ctx := context.Background()

	t.Run("valid code returns original URL", func(t *testing.T) {
		repo := new(mockRepo)
		repo.On("GetByShortCode", ctx, "Ab3_xK9mLp").Return("https://example.com", nil)

		svc := NewUrlService(repo, gen, slog.Default())
		url, err := svc.GetOriginal(ctx, "Ab3_xK9mLp")

		require.NoError(t, err)
		assert.Equal(t, "https://example.com", url)
		repo.AssertExpectations(t)
	})

	t.Run("not found returns ErrNotFound", func(t *testing.T) {
		repo := new(mockRepo)
		repo.On("GetByShortCode", ctx, "NotExist12").Return("", domain.ErrNotFound)

		svc := NewUrlService(repo, gen, nil)
		_, err := svc.GetOriginal(ctx, "NotExist12")

		assert.ErrorIs(t, err, domain.ErrNotFound)
		repo.AssertExpectations(t)
	})
}
