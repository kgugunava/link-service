package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/kgugunava/link-service/internal/adapters/memory"
	"github.com/kgugunava/link-service/internal/adapters/postgres"
	"github.com/kgugunava/link-service/internal/api"
	"github.com/kgugunava/link-service/internal/api/handler"
	"github.com/kgugunava/link-service/internal/config"
	"github.com/kgugunava/link-service/internal/utils"
	"github.com/kgugunava/link-service/internal/service"
)

func main() {
	// 1. Загрузка конфигурации
	cfg, err := config.Parse()
	if err != nil {
		log.Fatal("failed to parse config:", err)
	}

	// 2. Инициализация репозитория (переключение хранилища)
	repo, cleanup, err := initRepository(context.Background(), cfg)
	if err != nil {
		log.Fatal("failed to init repository:", err)
	}
	defer cleanup()

	// 3. Инициализация зависимостей
	urlGenerator := utils.NewGenerator()
	urlService := service.NewUrlService(repo, urlGenerator)
	urlHandler := handler.NewURLHandler(urlService)
	router := api.NewRouter(urlHandler)

	// 4. Запуск HTTP-сервера
	server := &http.Server{
		Addr:    cfg.Port,
		Handler: router,
	}

	// Graceful shutdown: обработка SIGINT/SIGTERM
	go func() {
		log.Printf("✓ Server starting on %s (storage: %s)", cfg.Port, cfg.StorageType)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal("server failed:", err)
		}
	}()

	// Ожидание сигнала завершения
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Println("⚠ Shutting down server...")

	// Корректное завершение с таймаутом
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := server.Shutdown(ctx); err != nil {
		log.Fatal("server shutdown failed:", err)
	}
	log.Println("✓ Server stopped")
}

// initRepository создаёт репозиторий в зависимости от типа хранилища
// Возвращает функцию cleanup для закрытия ресурсов (например, pgxpool)
func initRepository(ctx context.Context, cfg *config.Config) (service.UrlRepositoryInterface, func(), error) {
	switch cfg.StorageType {
	case "memory":
		log.Println("✓ Using in-memory storage")
		return memory.NewUrlMemoryRepository(), func() {}, nil

	case "postgres":
		log.Println("✓ Using PostgreSQL storage")
		pool, err := pgxpool.New(ctx, cfg.PgDSN)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to connect to postgres: %w", err)
		}

		// Опционально: запуск миграций при старте
		// if err := migrations.Run(ctx, cfg.PgDSN); err != nil {
		// 	pool.Close()
		// 	return nil, nil, fmt.Errorf("failed to run migrations: %w", err)
		// }

		return postgres.NewUrlPostgresRepository(pool), func() { pool.Close() }, nil

	default:
		return nil, nil, fmt.Errorf("unknown storage type: %s", cfg.StorageType)
	}
}