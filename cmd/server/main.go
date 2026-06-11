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

func main() {
	logger := setupLogger()

	cfg, err := config.Parse()
	if err != nil {
		logger.Error("failed to parse config", "error", err)
		os.Exit(1)
	}

	logger.Info("starting server", "storage", cfg.StorageType, "port", cfg.Port)

	repo, cleanup, err := initRepository(context.Background(), cfg, logger)
	if err != nil {
		logger.Error("failed to init repository", "error", err)
		os.Exit(1)
	}
	defer cleanup()

	urlGenerator := utils.NewGenerator(logger)
	urlService := service.NewUrlService(repo, urlGenerator, logger)
	urlHandler := handler.NewURLHandler(urlService, logger)
	router := api.NewRouter(urlHandler)

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
	<-quit

	logger.Info("shutting down server")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		logger.Error("server shutdown failed", "error", err)
		os.Exit(1)
	}

	logger.Info("server stopped")
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

func setupLogger() *slog.Logger {
	level := slog.LevelInfo
	if os.Getenv("DEBUG") == "1" {
		level = slog.LevelDebug
	}

	var handler slog.Handler
	if os.Getenv("LOG_FORMAT") == "json" {
		handler = slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: level})
	} else {
		handler = slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
			Level:     level,
			AddSource: true,
		})
	}

	return slog.New(handler)
}
