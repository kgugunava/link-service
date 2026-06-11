package config

import (
	"flag"
	"fmt"
	"os"
	"time"
)

type Config struct {
	StorageType     string
	PgDSN           string
	Port            string
	URLTTL          time.Duration
	CleanerInterval time.Duration
	Debug           bool
	LogFormat       string
}

func Parse() (*Config, error) {
	storage := flag.String("storage", getEnv("STORAGE_TYPE", "memory"), "storage type: memory|postgres")
	pgDSN := flag.String("pg-dsn", getEnv("PG_DSN", "postgres://user:pass@localhost:5432/shortener?sslmode=disable"), "PostgreSQL DSN")
	port := flag.String("port", getEnv("PORT", ":8080"), "HTTP server port")
	urlTTL := flag.String("url-ttl", getEnv("URL_TTL", "720h"), "URL time-to-live (e.g., 720h, 30d, 1h30m)")
	cleanerInterval := flag.String("cleaner-interval", getEnv("CLEANER_INTERVAL", "1h"), "cleaner interval (e.g., 1h, 30m)")
	debug := flag.Bool("debug", getEnv("DEBUG", "0") == "1", "enable debug logging")
	logFormat := flag.String("log-format", getEnv("LOG_FORMAT", "text"), "log format: text|json")
	flag.Parse()

	if *storage != "memory" && *storage != "postgres" {
		return nil, fmt.Errorf("invalid storage type: %s (expected: memory|postgres)", *storage)
	}

	if *logFormat != "text" && *logFormat != "json" {
		return nil, fmt.Errorf("invalid log format: %s (expected: text|json)", *logFormat)
	}

	ttl, err := parseDuration(*urlTTL)
	if err != nil {
		return nil, fmt.Errorf("invalid URL_TTL: %w", err)
	}

	interval, err := parseDuration(*cleanerInterval)
	if err != nil {
		return nil, fmt.Errorf("invalid CLEANER_INTERVAL: %w", err)
	}

	return &Config{
		StorageType:     *storage,
		PgDSN:           *pgDSN,
		Port:            *port,
		URLTTL:          ttl,
		CleanerInterval: interval,
		Debug:           *debug,
		LogFormat:       *logFormat,
	}, nil
}

func getEnv(key, defaultValue string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return defaultValue
}

func parseDuration(s string) (time.Duration, error) {
	if d, err := time.ParseDuration(s); err == nil {
		return d, nil
	}

	if len(s) > 0 && s[len(s)-1] == 'd' {
		var days int
		if _, err := fmt.Sscanf(s, "%dd", &days); err == nil {
			return time.Duration(days) * 24 * time.Hour, nil
		}
	}

	return 0, fmt.Errorf("cannot parse duration: %s", s)
}
