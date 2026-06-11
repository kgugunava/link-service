package memory

import (
	"context"
	"fmt"
	"log/slog"
	"sync"

	"github.com/kgugunava/link-service/internal/domain"
)

type URLMemoryRepository struct {
	mu              sync.RWMutex
	originalToShort map[string]string
	shortToOriginal map[string]string
	logger          *slog.Logger
}

func NewUrlMemoryRepository(logger *slog.Logger) *URLMemoryRepository {
	if logger == nil {
		logger = slog.Default()
	}
	return &URLMemoryRepository{
		originalToShort: make(map[string]string),
		shortToOriginal: make(map[string]string),
		logger:          logger,
	}
}

func (r *URLMemoryRepository) Save(ctx context.Context, originalUrl, shortCode string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, ok := r.originalToShort[originalUrl]; ok {
		r.logger.Debug("url already exists", "original_url", originalUrl)
		return nil
	}

	if _, ok := r.shortToOriginal[shortCode]; ok {
		r.logger.Debug("short code collision", "short_code", shortCode)
		return fmt.Errorf("short_code_collision: code %q already exists", shortCode)
	}

	r.originalToShort[originalUrl] = shortCode
	r.shortToOriginal[shortCode] = originalUrl
	r.logger.Info("url saved", "original_url", originalUrl, "short_code", shortCode)
	return nil
}

func (r *URLMemoryRepository) GetByShortCode(ctx context.Context, shortCode string) (string, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	url, ok := r.shortToOriginal[shortCode]
	if !ok {
		r.logger.Debug("short code not found", "short_code", shortCode)
		return "", domain.ErrNotFound
	}

	return url, nil
}
