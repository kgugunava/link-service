package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"strings"
	"time"

	"github.com/kgugunava/link-service/internal/domain"
	"github.com/kgugunava/link-service/internal/utils"
)

const (
	maxCollisionRetries = 3
	defaultTTL          = 30 * 24 * time.Hour
)

type URLRepositoryInterface interface {
	Save(ctx context.Context, url *domain.URL) error
	GetByShortCode(ctx context.Context, shortCode string) (*domain.URL, error)
	DeleteExpired(ctx context.Context) (int64, error)
}

type URLService struct {
	urlRepo      URLRepositoryInterface
	urlGenerator *utils.Generator
	logger       *slog.Logger
	ttl          time.Duration
}

func NewUrlService(urlRepo URLRepositoryInterface, urlGenerator *utils.Generator, logger *slog.Logger, ttl time.Duration) *URLService {
	if logger == nil {
		logger = slog.Default()
	}
	if ttl <= 0 {
		ttl = defaultTTL
	}
	return &URLService{
		urlRepo:      urlRepo,
		urlGenerator: urlGenerator,
		logger:       logger,
		ttl:          ttl,
	}
}

func (s *URLService) Shorten(ctx context.Context, originalURL string) (string, error) {
	s.logger.Debug("shorten started", "original_url", originalURL)

	if err := validateUrl(originalURL); err != nil {
		s.logger.Warn("invalid url", "original_url", originalURL, "error", err)
		return "", domain.ErrInvalidURL
	}

	shortCode, err := s.generateWithFallback(ctx, originalURL)
	if err != nil {
		s.logger.Error("failed to generate short code", "original_url", originalURL, "error", err)
		return "", err
	}

	s.logger.Info("url shortened", "original_url", originalURL, "short_code", shortCode)
	return shortCode, nil
}

func (s *URLService) generateWithFallback(ctx context.Context, originalURL string) (string, error) {
	for attempt := 0; attempt < maxCollisionRetries; attempt++ {
		var shortCode string

		if attempt == 0 {
			shortCode = s.urlGenerator.Generate(originalURL)
		} else {
			saltedURL := fmt.Sprintf("%s#retry_%d", originalURL, attempt)
			shortCode = s.urlGenerator.Generate(saltedURL)
			s.logger.Debug("retrying with salted url", "original_url", originalURL, "attempt", attempt, "salted_url", saltedURL)
		}

		urlModel := &domain.URL{
			OriginalURL: originalURL,
			ShortCode:   shortCode,
			CreatedAt:   time.Now(),
			ExpiresAt:   time.Now().Add(s.ttl),
		}

		err := s.urlRepo.Save(ctx, urlModel)
		if err == nil {
			return shortCode, nil
		}

		if !isShortCodeCollision(err) {
			return "", err
		}

		s.logger.Debug("short code collision detected, retrying", "short_code", shortCode, "attempt", attempt+1)
	}

	return "", fmt.Errorf("failed to generate unique short code after %d attempts", maxCollisionRetries)
}

func isShortCodeCollision(err error) bool {
	if err == nil {
		return false
	}

	errStr := err.Error()

	if strings.Contains(errStr, "short_code_collision") {
		return true
	}

	if strings.Contains(errStr, "duplicate key value violates unique constraint") &&
		strings.Contains(errStr, "short_code") {
		return true
	}

	if strings.Contains(errStr, "UNIQUE constraint failed") &&
		strings.Contains(errStr, "short_code") {
		return true
	}

	if errors.Is(err, domain.ErrShortCodeCollision) {
		return true
	}

	return false
}

func (s *URLService) GetOriginal(ctx context.Context, shortCode string) (string, error) {
	s.logger.Debug("get_original started", "short_code", shortCode)

	if !isValidShortCode(shortCode) {
		s.logger.Warn("invalid code format", "short_code", shortCode)
		return "", domain.ErrInvalidCode
	}

	urlModel, err := s.urlRepo.GetByShortCode(ctx, shortCode)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			s.logger.Debug("not found", "short_code", shortCode)
			return "", domain.ErrNotFound
		}
		s.logger.Error("get failed", "short_code", shortCode, "error", err)
		return "", err
	}

	if !urlModel.ExpiresAt.IsZero() && time.Now().After(urlModel.ExpiresAt) {
		s.logger.Debug("url expired", "short_code", shortCode, "expires_at", urlModel.ExpiresAt)
		return "", domain.ErrNotFound
	}

	s.logger.Info("original url retrieved", "short_code", shortCode, "original_url", urlModel.OriginalURL)
	return urlModel.OriginalURL, nil
}

func (s *URLService) CleanupExpired(ctx context.Context) (int64, error) {
	s.logger.Debug("cleanup expired started")

	deleted, err := s.urlRepo.DeleteExpired(ctx)
	if err != nil {
		s.logger.Error("cleanup failed", "error", err)
		return 0, err
	}

	if deleted > 0 {
		s.logger.Info("cleanup completed", "deleted", deleted)
	}
	return deleted, nil
}

func validateUrl(raw string) error {
	u, err := url.ParseRequestURI(raw)
	if err != nil {
		return err
	}
	if u.Scheme == "" || u.Host == "" {
		return fmt.Errorf("missing scheme or host")
	}
	return nil
}

func isValidShortCode(code string) bool {
	if len(code) != 10 {
		return false
	}
	for _, c := range code {
		if !((c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '_') {
			return false
		}
	}
	return true
}