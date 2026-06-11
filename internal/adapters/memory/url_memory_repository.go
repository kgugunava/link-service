package memory

import (
    "context"
    "log/slog"
    "sync"
    "time"

    "github.com/kgugunava/link-service/internal/domain"
)

type UrlMemoryRepository struct {
    mu      sync.RWMutex
    urls    map[string]*domain.URL  // short_code → URL
    byOrig  map[string]string       // original_url → short_code
    logger  *slog.Logger
}

func NewUrlMemoryRepository(logger *slog.Logger) *UrlMemoryRepository {
    if logger == nil {
        logger = slog.Default()
    }
    return &UrlMemoryRepository{
        urls:   make(map[string]*domain.URL),
        byOrig: make(map[string]string),
        logger: logger,
    }
}

func (r *UrlMemoryRepository) Save(ctx context.Context, url *domain.URL) error {
    r.mu.Lock()
    defer r.mu.Unlock()

    if existingCode, ok := r.byOrig[url.OriginalURL]; ok {
        r.logger.Debug("url already exists", "original_url", url.OriginalURL, "existing_code", existingCode)
        return nil
    }

    r.urls[url.ShortCode] = url
    r.byOrig[url.OriginalURL] = url.ShortCode

    r.logger.Info("url saved", "original_url", url.OriginalURL, "short_code", url.ShortCode, "expires_at", url.ExpiresAt)
    return nil
}

func (r *UrlMemoryRepository) GetByShortCode(ctx context.Context, shortCode string) (*domain.URL, error) {
    r.mu.RLock()
    defer r.mu.RUnlock()

    url, ok := r.urls[shortCode]
    if !ok {
        r.logger.Debug("short code not found", "short_code", shortCode)
        return nil, domain.ErrNotFound
    }

    // Проверка TTL
    if !url.ExpiresAt.IsZero() && time.Now().After(url.ExpiresAt) {
        r.logger.Debug("url expired", "short_code", shortCode, "expired_at", url.ExpiresAt)
        return nil, domain.ErrNotFound
    }

    return url, nil
}

// DeleteExpired удаляет все просроченные записи. Возвращает количество удалённых.
func (r *UrlMemoryRepository) DeleteExpired(ctx context.Context) (int64, error) {
    r.mu.Lock()
    defer r.mu.Unlock()

    now := time.Now()
    var deleted int64

    for code, url := range r.urls {
        if !url.ExpiresAt.IsZero() && now.After(url.ExpiresAt) {
            delete(r.urls, code)
            delete(r.byOrig, url.OriginalURL)
            deleted++
        }
    }

    if deleted > 0 {
        r.logger.Info("expired urls deleted", "count", deleted, "remaining", len(r.urls))
    }
    return deleted, nil
}

// Size возвращает текущее количество записей (для метрик)
func (r *UrlMemoryRepository) Size() int {
    r.mu.RLock()
    defer r.mu.RUnlock()
    return len(r.urls)
}