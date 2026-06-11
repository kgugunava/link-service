package service

import (
	"context"
	"log/slog"
	"time"
)

type Cleaner struct {
	repo     URLRepositoryInterface
	interval time.Duration
	logger   *slog.Logger
	stopCh   chan struct{}
}

func NewCleaner(repo URLRepositoryInterface, interval time.Duration, logger *slog.Logger) *Cleaner {
	if logger == nil {
		logger = slog.Default()
	}
	return &Cleaner{
		repo:     repo,
		interval: interval,
		logger:   logger,
		stopCh:   make(chan struct{}),
	}
}

// Run запускает периодическую очистку; блокирующий вызов
func (c *Cleaner) Run(ctx context.Context) {
	c.logger.Info("cleaner started", "interval", c.interval)
	ticker := time.NewTicker(c.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			c.logger.Info("cleaner stopped by context")
			return
		case <-c.stopCh:
			c.logger.Info("cleaner stopped by signal")
			return
		case <-ticker.C:
			deleted, err := c.repo.DeleteExpired(ctx)
			if err != nil {
				c.logger.Error("cleanup failed", "error", err)
				continue
			}
			if deleted > 0 {
				c.logger.Info("cleanup completed", "deleted", deleted)
			}
		}
	}
}

func (c *Cleaner) Stop() {
	close(c.stopCh)
}
