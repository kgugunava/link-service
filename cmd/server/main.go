package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/kgugunava/link-service/internal/adapters/memory"
	"github.com/kgugunava/link-service/internal/adapters/postgres"
	"github.com/kgugunava/link-service/internal/api"
	"github.com/kgugunava/link-service/internal/api/handler"
	"github.com/kgugunava/link-service/internal/config"
	"github.com/kgugunava/link-service/internal/service"
	"github.com/kgugunava/link-service/internal/utils"
)

const shutdownTimeout = 10 * time.Second

func main() {
	cfg, err := config.Parse()
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to parse config: %v\n", err)
		os.Exit(1)
	}

	logger := setupLogger(cfg)

	logger.Info("starting server",
		"storage", cfg.StorageType,
		"port", cfg.Port,
		"url_ttl", cfg.URLTTL,
		"cleaner_interval", cfg.CleanerInterval,
	)

	repo, cleanup, err := initRepository(context.Background(), cfg, logger)
	if err != nil {
		logger.Error("failed to init repository", "error", err)
		os.Exit(1)
	}
	defer cleanup()

	urlGenerator := utils.NewGenerator(logger)
	urlService := service.NewUrlService(repo, urlGenerator, logger, cfg.URLTTL)
	urlHandler := handler.NewURLHandler(urlService, logger)
	router := api.NewRouter(urlHandler)

	cleanerCtx, cleanerCancel := context.WithCancel(context.Background())
	defer cleanerCancel()

	cleaner := service.NewCleaner(repo, cfg.CleanerInterval, logger)
	go cleaner.Run(cleanerCtx)
	logger.Info("cleaner started", "interval", cfg.CleanerInterval)

	server := &http.Server{
		Addr:    cfg.Port,
		Handler: router,
	}

	go func() {
		logger.Info("server listening", "addr", cfg.Port)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("server failed", "error", err)
			os.Exit(1)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	sig := <-quit
	logger.Info("shutdown signal received", "signal", sig.String())

	logger.Info("shutting down server")

	ctx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		logger.Error("server shutdown failed", "error", err)
	} else {
		logger.Info("server stopped")
	}

	cleaner.Stop()
	logger.Info("cleaner stopped")

	logger.Info("shutdown completed")
}

func initRepository(ctx context.Context, cfg *config.Config, logger *slog.Logger) (service.URLRepositoryInterface, func(), error) {
	switch cfg.StorageType {
	case "memory":
		logger.Info("using in-memory storage")
		return memory.NewUrlMemoryRepository(logger), func() {}, nil

	case "postgres":
		logger.Info("using postgresql storage", "dsn", cfg.PgDSN)
		pool, err := pgxpool.New(ctx, cfg.PgDSN)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to connect to postgres: %w", err)
		}
		return postgres.NewURLPostgresRepository(pool, logger), func() { pool.Close() }, nil

	default:
		return nil, nil, fmt.Errorf("unknown storage type: %s", cfg.StorageType)
	}
}

// setupLogger настраивает структурированный логгер на основе конфига
func setupLogger(cfg *config.Config) *slog.Logger {
	level := slog.LevelInfo
	if cfg.Debug {
		level = slog.LevelDebug
	}

	var handler slog.Handler
	if cfg.LogFormat == "json" {
		handler = slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: level})
	} else {
		handler = slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
			Level:     level,
			AddSource: true,
		})
	}

	return slog.New(handler)
}