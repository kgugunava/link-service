package service

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/kgugunava/link-service/internal/domain"
	"github.com/kgugunava/link-service/internal/utils"
)

// Мок репозитория

type mockURLRepo struct {
	mock.Mock
}

func (m *mockURLRepo) Save(ctx context.Context, url *domain.URL) error {
	args := m.Called(ctx, url)
	return args.Error(0)
}

func (m *mockURLRepo) GetByShortCode(ctx context.Context, shortCode string) (*domain.URL, error) {
	args := m.Called(ctx, shortCode)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.URL), args.Error(1)
}

func (m *mockURLRepo) DeleteExpired(ctx context.Context) (int64, error) {
	args := m.Called(ctx)
	return args.Get(0).(int64), args.Error(1)
}

func newTestService(t *testing.T, repo URLRepositoryInterface, ttl time.Duration) *URLService {
	t.Helper()
	gen := utils.NewGenerator(slog.New(slog.NewTextHandler(io.Discard, nil)))
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	return NewUrlService(repo, gen, logger, ttl)
}

// Тесты для Shorten

func TestURLService_Shorten(t *testing.T) {
	ctx := context.Background()

	t.Run("valid URL returns 10-char code", func(t *testing.T) {
		repo := new(mockURLRepo)
		repo.On("Save", ctx, mock.MatchedBy(func(u *domain.URL) bool {
			return u.OriginalURL == "https://example.com" &&
				len(u.ShortCode) == 10 &&
				!u.CreatedAt.IsZero() &&
				!u.ExpiresAt.IsZero()
		})).Return(nil).Once()

		svc := newTestService(t, repo, time.Hour)
		code, err := svc.Shorten(ctx, "https://example.com")

		require.NoError(t, err)
		assert.Len(t, code, 10)
		repo.AssertExpectations(t)
	})

	t.Run("invalid URL returns ErrInvalidURL", func(t *testing.T) {
		repo := new(mockURLRepo)
		svc := newTestService(t, repo, time.Hour)

		code, err := svc.Shorten(ctx, "not-a-url")
		assert.ErrorIs(t, err, domain.ErrInvalidURL)
		assert.Empty(t, code)
		repo.AssertNotCalled(t, "Save")
	})

	t.Run("empty URL returns ErrInvalidURL", func(t *testing.T) {
		repo := new(mockURLRepo)
		svc := newTestService(t, repo, time.Hour)

		code, err := svc.Shorten(ctx, "")
		assert.ErrorIs(t, err, domain.ErrInvalidURL)
		assert.Empty(t, code)
	})

	t.Run("URL without scheme returns ErrInvalidURL", func(t *testing.T) {
		repo := new(mockURLRepo)
		svc := newTestService(t, repo, time.Hour)

		code, err := svc.Shorten(ctx, "example.com")
		assert.ErrorIs(t, err, domain.ErrInvalidURL)
		assert.Empty(t, code)
	})

	t.Run("repo error is returned", func(t *testing.T) {
		repo := new(mockURLRepo)
		repo.On("Save", ctx, mock.Anything).Return(errors.New("db connection failed")).Once()

		svc := newTestService(t, repo, time.Hour)
		code, err := svc.Shorten(ctx, "https://example.com")

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "db connection failed")
		assert.Empty(t, code)
		repo.AssertExpectations(t)
	})

	t.Run("sets correct TTL on URL model", func(t *testing.T) {
		repo := new(mockURLRepo)
		ttl := 7 * 24 * time.Hour

		repo.On("Save", ctx, mock.MatchedBy(func(u *domain.URL) bool {
			diff := u.ExpiresAt.Sub(u.CreatedAt)
			return diff >= ttl-time.Minute && diff <= ttl+time.Minute
		})).Return(nil).Once()

		svc := newTestService(t, repo, ttl)
		_, err := svc.Shorten(ctx, "https://example.com")
		require.NoError(t, err)
		repo.AssertExpectations(t)
	})

	t.Run("uses default TTL when zero is passed", func(t *testing.T) {
		repo := new(mockURLRepo)
		repo.On("Save", ctx, mock.MatchedBy(func(u *domain.URL) bool {
			diff := u.ExpiresAt.Sub(u.CreatedAt)
			return diff >= 29*24*time.Hour && diff <= 31*24*time.Hour
		})).Return(nil).Once()

		svc := newTestService(t, repo, 0)
		_, err := svc.Shorten(ctx, "https://example.com")
		require.NoError(t, err)
		repo.AssertExpectations(t)
	})
}

// Тесты для fallback при коллизии

func TestURLService_Shorten_CollisionFallback(t *testing.T) {
	ctx := context.Background()

	t.Run("retries on collision and succeeds on second attempt", func(t *testing.T) {
		repo := new(mockURLRepo)

		repo.On("Save", ctx, mock.MatchedBy(func(u *domain.URL) bool {
			return u.OriginalURL == "https://example.com" && u.ShortCode == utils.NewGenerator(nil).Generate("https://example.com")
		})).Return(errors.New("short_code_collision: duplicate key")).Once()

		repo.On("Save", ctx, mock.MatchedBy(func(u *domain.URL) bool {
			return u.OriginalURL == "https://example.com" && u.ShortCode != utils.NewGenerator(nil).Generate("https://example.com")
		})).Return(nil).Once()

		svc := newTestService(t, repo, time.Hour)
		code, err := svc.Shorten(ctx, "https://example.com")

		require.NoError(t, err)
		assert.Len(t, code, 10)
		repo.AssertNumberOfCalls(t, "Save", 2)
	})

	t.Run("returns error after max retries", func(t *testing.T) {
		repo := new(mockURLRepo)

		repo.On("Save", ctx, mock.Anything).
			Return(errors.New("short_code_collision: duplicate key")).
			Times(maxCollisionRetries)

		svc := newTestService(t, repo, time.Hour)
		code, err := svc.Shorten(ctx, "https://example.com")

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to generate unique short code")
		assert.Empty(t, code)
		repo.AssertNumberOfCalls(t, "Save", maxCollisionRetries)
	})

	t.Run("does not retry on non-collision error", func(t *testing.T) {
		repo := new(mockURLRepo)
		repo.On("Save", ctx, mock.Anything).Return(errors.New("connection refused")).Once()

		svc := newTestService(t, repo, time.Hour)
		code, err := svc.Shorten(ctx, "https://example.com")

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "connection refused")
		assert.Empty(t, code)
		repo.AssertNumberOfCalls(t, "Save", 1)
	})

	t.Run("detects PostgreSQL collision error format", func(t *testing.T) {
		repo := new(mockURLRepo)

		repo.On("Save", ctx, mock.Anything).
			Return(errors.New("ERROR: duplicate key value violates unique constraint \"urls_short_code_key\" (SQLSTATE 23505)")).
			Once()

		repo.On("Save", ctx, mock.Anything).Return(nil).Once()

		svc := newTestService(t, repo, time.Hour)
		code, err := svc.Shorten(ctx, "https://example.com")

		require.NoError(t, err)
		assert.Len(t, code, 10)
		repo.AssertNumberOfCalls(t, "Save", 2)
	})

	t.Run("detects SQLite collision error format", func(t *testing.T) {
		repo := new(mockURLRepo)

		repo.On("Save", ctx, mock.Anything).
			Return(errors.New("UNIQUE constraint failed: urls.short_code")).
			Once()

		repo.On("Save", ctx, mock.Anything).Return(nil).Once()

		svc := newTestService(t, repo, time.Hour)
		code, err := svc.Shorten(ctx, "https://example.com")

		require.NoError(t, err)
		assert.Len(t, code, 10)
	})
}

// Тесты для GetOriginal

func TestURLService_GetOriginal(t *testing.T) {
	ctx := context.Background()

	t.Run("returns original URL for valid code", func(t *testing.T) {
		repo := new(mockURLRepo)
		urlModel := &domain.URL{
			OriginalURL: "https://example.com/path",
			ShortCode:   "Ab3_xK9mLp",
			CreatedAt:   time.Now(),
			ExpiresAt:   time.Now().Add(time.Hour),
		}
		repo.On("GetByShortCode", ctx, "Ab3_xK9mLp").Return(urlModel, nil).Once()

		svc := newTestService(t, repo, time.Hour)
		original, err := svc.GetOriginal(ctx, "Ab3_xK9mLp")

		require.NoError(t, err)
		assert.Equal(t, "https://example.com/path", original)
		repo.AssertExpectations(t)
	})

	t.Run("returns ErrNotFound for unknown code", func(t *testing.T) {
		repo := new(mockURLRepo)
		repo.On("GetByShortCode", ctx, "NotExist12").Return(nil, domain.ErrNotFound).Once()

		svc := newTestService(t, repo, time.Hour)
		original, err := svc.GetOriginal(ctx, "NotExist12")

		assert.ErrorIs(t, err, domain.ErrNotFound)
		assert.Empty(t, original)
		repo.AssertExpectations(t)
	})

	t.Run("returns ErrInvalidCode for wrong length", func(t *testing.T) {
		repo := new(mockURLRepo)
		svc := newTestService(t, repo, time.Hour)

		original, err := svc.GetOriginal(ctx, "short")
		assert.ErrorIs(t, err, domain.ErrInvalidCode)
		assert.Empty(t, original)
		repo.AssertNotCalled(t, "GetByShortCode")
	})

	t.Run("returns ErrInvalidCode for invalid characters", func(t *testing.T) {
		repo := new(mockURLRepo)
		svc := newTestService(t, repo, time.Hour)

		original, err := svc.GetOriginal(ctx, "bad!code#1")
		assert.ErrorIs(t, err, domain.ErrInvalidCode)
		assert.Empty(t, original)
	})

	t.Run("returns ErrNotFound for expired URL", func(t *testing.T) {
		repo := new(mockURLRepo)
		urlModel := &domain.URL{
			OriginalURL: "https://expired.com",
			ShortCode:   "Expired001",
			CreatedAt:   time.Now().Add(-2 * time.Hour),
			ExpiresAt:   time.Now().Add(-1 * time.Hour), // уже истёк
		}
		repo.On("GetByShortCode", ctx, "Expired001").Return(urlModel, nil).Once()

		svc := newTestService(t, repo, time.Hour)
		original, err := svc.GetOriginal(ctx, "Expired001")

		assert.ErrorIs(t, err, domain.ErrNotFound)
		assert.Empty(t, original)
	})

	t.Run("returns URL when ExpiresAt is zero (no TTL)", func(t *testing.T) {
		repo := new(mockURLRepo)
		urlModel := &domain.URL{
			OriginalURL: "https://no-ttl.com",
			ShortCode:   "NoTTL00001",
			CreatedAt:   time.Now(),
			ExpiresAt:   time.Time{},
		}
		repo.On("GetByShortCode", ctx, "NoTTL00001").Return(urlModel, nil).Once()

		svc := newTestService(t, repo, time.Hour)
		original, err := svc.GetOriginal(ctx, "NoTTL00001")

		require.NoError(t, err)
		assert.Equal(t, "https://no-ttl.com", original)
	})

	t.Run("returns URL just before expiration", func(t *testing.T) {
		repo := new(mockURLRepo)
		urlModel := &domain.URL{
			OriginalURL: "https://almost.com",
			ShortCode:   "Almost0001",
			CreatedAt:   time.Now().Add(-time.Hour),
			ExpiresAt:   time.Now().Add(time.Second),
		}
		repo.On("GetByShortCode", ctx, "Almost0001").Return(urlModel, nil).Once()

		svc := newTestService(t, repo, time.Hour)
		original, err := svc.GetOriginal(ctx, "Almost0001")

		require.NoError(t, err)
		assert.Equal(t, "https://almost.com", original)
	})

	t.Run("returns error on repo failure", func(t *testing.T) {
		repo := new(mockURLRepo)
		repo.On("GetByShortCode", ctx, "Ab3_xK9mLp").Return(nil, errors.New("db timeout")).Once()

		svc := newTestService(t, repo, time.Hour)
		original, err := svc.GetOriginal(ctx, "Ab3_xK9mLp")

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "db timeout")
		assert.Empty(t, original)
	})
}

func TestValidateUrl(t *testing.T) {
	tests := []struct {
		name        string
		url         string
		expectError bool
	}{
		{"valid https", "https://example.com", false},
		{"valid http", "http://example.com/path?query=1", false},
		{"valid with port", "https://localhost:8080/api", false},
		{"valid with fragment", "https://example.com/page#section", false},
		{"empty", "", true},
		{"no scheme", "example.com", true},
		{"only scheme", "https://", true},
		{"invalid chars", "https://example .com", true},
		{"just text", "not a url", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateUrl(tt.url)
			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestIsValidShortCode(t *testing.T) {
	tests := []struct {
		name     string
		code     string
		expected bool
	}{
		{"valid mixed", "Ab3_xK9mLp", true},
		{"valid lowercase", "abcdefghij", true},
		{"valid uppercase", "ABCDEFGHIJ", true},
		{"valid with underscore", "A_b_1_2_3_", true},
		{"too short", "Ab3_xK9mL", false},
		{"too long", "Ab3_xK9mLpX", false},
		{"has hyphen", "Ab3-xK9mLp", false},
		{"has dot", "Ab3.xK9mLp", false},
		{"has slash", "Ab3/xK9mLp", false},
		{"has exclamation", "Ab3!xK9mLp", false},
		{"empty", "", false},
		{"cyrillic", "Абвгдежзик", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, isValidShortCode(tt.code))
		})
	}
}

func TestIsShortCodeCollision(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{"nil error", nil, false},
		{"unrelated error", errors.New("connection refused"), false},
		{"postgres collision", errors.New("ERROR: duplicate key value violates unique constraint \"urls_short_code_key\""), true},
		{"sqlite collision", errors.New("UNIQUE constraint failed: urls.short_code"), true},
		{"domain collision", domain.ErrShortCodeCollision, true},
		{"postgres other unique", errors.New("duplicate key value violates unique constraint \"urls_original_url_key\""), false},
		{"wrapped collision", errors.New("short_code_collision: duplicate"), true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, isShortCodeCollision(tt.err))
		})
	}
}
