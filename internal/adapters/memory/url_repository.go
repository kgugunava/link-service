package memory

import (
	"context"
	"sync"

	"github.com/kgugunava/link-service/internal/domain"
)

type UrlMemoryRepository struct {
	mu sync.RWMutex
	originalToShort map[string]string
	shortToOriginal map[string]string
}

func NewUrlMemoryRepository() *UrlMemoryRepository {
	return &UrlMemoryRepository{
		originalToShort: make(map[string]string),
		shortToOriginal: make(map[string]string),
	}
}

func (r *UrlMemoryRepository) Save(ctx context.Context, originalUrl, shortCode string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Идемпотентность: если URL уже есть, игнорируем вставку
	if _, ok := r.originalToShort[originalUrl]; ok {
		return nil
	}

	r.originalToShort[originalUrl] = shortCode
	r.shortToOriginal[shortCode] = originalUrl
	return nil
}

func (r *UrlMemoryRepository) GetByShortCode(ctx context.Context, shortCode string) (string, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	url, ok := r.shortToOriginal[shortCode]
	if !ok {
		return "", domain.ErrNotFound
	}
	return url, nil
}