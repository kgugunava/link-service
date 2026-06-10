package config

import (
	"flag"
	"fmt"
	"os"
)

// Config хранит параметры запуска сервиса
type Config struct {
	StorageType string // memory | postgres
	PgDSN       string // PostgreSQL connection string
	Port        string // порт HTTP-сервера, например ":8080"
	// BaseURL     string // домен коротких ссылок, например "https://short.link"
}

// Parse считывает конфигурацию из флагов командной строки и env-переменных.
// Приоритет: флаги > env > дефолты.
func Parse() (*Config, error) {
	// Флаги командной строки
	storage := flag.String("storage", getEnv("STORAGE_TYPE", "memory"), "storage type: memory|postgres")
	pgDSN := flag.String("pg-dsn", getEnv("PG_DSN", "postgres://user:pass@localhost:5432/shortener?sslmode=disable"), "PostgreSQL DSN")
	port := flag.String("port", getEnv("PORT", ":8080"), "HTTP server port")
	// baseURL := flag.String("base-url", getEnv("BASE_URL", "http://localhost:8080"), "base URL for short links")
	flag.Parse()

	// Валидация
	if *storage != "memory" && *storage != "postgres" {
		return nil, fmt.Errorf("invalid storage type: %s (expected: memory|postgres)", *storage)
	}

	return &Config{
		StorageType: *storage,
		PgDSN:       *pgDSN,
		Port:        *port,
		// BaseURL:     *baseURL,
	}, nil
}

// getEnv возвращает значение переменной окружения или дефолт, если не задана
func getEnv(key, defaultValue string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return defaultValue
}