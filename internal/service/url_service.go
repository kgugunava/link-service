package service

import (
	"context"
	"errors"
	"fmt"
	"net/url"

	"github.com/kgugunava/link-service/internal/domain"
	"github.com/kgugunava/link-service/internal/utils"
)

var (
	ErrInvalidURL    = errors.New("invalid original URL")
	ErrShortNotFound = errors.New("short code not found")
)

type UrlRepositoryInterface interface {
	Save(ctx context.Context, originalUrl, shortCode string) error
	GetByShortCode(ctx context.Context, shortCode string) (string, error)
}

type UrlService struct {
	urlRepo UrlRepositoryInterface
	urlGenerator *utils.Generator
}

func NewUrlService(urlRepo UrlRepositoryInterface, urlGenerator *utils.Generator) *UrlService {
	return &UrlService{
		urlRepo: urlRepo,
		urlGenerator: urlGenerator,
	}
}

// Shorten валидирует URL, генерирует код и сохраняет.
// Возвращает короткий код (без домена) или ошибку.
func (s *UrlService) Shorten(ctx context.Context, originalUrl string) (string, error) {
	if err := validateUrl(originalUrl); err != nil {
		return "", fmt.Errorf("%w: %v", ErrInvalidURL, err)
	}

	shortCode := s.urlGenerator.Generate(originalUrl)

	// Репозиторий гарантирует идемпотентность через UNIQUE(original_url)
	if err := s.urlRepo.Save(ctx, originalUrl, shortCode); err != nil {
		return "", fmt.Errorf("save to storage: %w", err)
	}

	return shortCode, nil
}

// GetOriginal возвращает оригинальный URL по короткому коду.
func (s *UrlService) GetOriginal(ctx context.Context, shortCode string) (string, error) {
	// Опционально: валидация формата shortCode перед запросом в БД
	if !isValidShortCode(shortCode) { return "", ErrShortNotFound }

	original, err := s.urlRepo.GetByShortCode(ctx, shortCode)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return "", ErrShortNotFound
		}
		return "", fmt.Errorf("get from storage: %w", err)
	}
	return original, nil
}

// validateUrl проверяет формат оригинальной url
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

// isValidShortCode проверяет формат [A-Za-z0-9_]{10}
// Можно вынести в domain или utils
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