package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"strings"

	"github.com/kgugunava/link-service/internal/domain"
	"github.com/kgugunava/link-service/internal/utils"
)

const maxCollisionRetries = 3

type URLRepositoryInterface interface {
	Save(ctx context.Context, originalURL, shortCode string) error
	GetByShortCode(ctx context.Context, shortCode string) (string, error)
}

type URLService struct {
	urlRepo      URLRepositoryInterface
	urlGenerator *utils.Generator
	logger       *slog.Logger
}

func NewUrlService(urlRepo URLRepositoryInterface, urlGenerator *utils.Generator, logger *slog.Logger) *URLService {
	if logger == nil {
		logger = slog.Default()
	}
	return &URLService{
		urlRepo:      urlRepo,
		urlGenerator: urlGenerator,
		logger:       logger,
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

// generateWithFallback generates a short code with collision retry logic
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

		err := s.urlRepo.Save(ctx, originalURL, shortCode)
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

// isShortCodeCollision checks if the error is due to a short_code uniqueness violation
func isShortCodeCollision(err error) bool {
	if err == nil {
		return false
	}

	errStr := err.Error()

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

	original, err := s.urlRepo.GetByShortCode(ctx, shortCode)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			s.logger.Debug("not found", "short_code", shortCode)
			return "", domain.ErrNotFound
		}
		s.logger.Error("get failed", "short_code", shortCode, "error", err)
		return "", err
	}

	s.logger.Info("original url retrieved", "short_code", shortCode, "original_url", original)
	return original, nil
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
