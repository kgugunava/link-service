package domain

import (
	"time"
)

type URL struct {
	OriginalURL string
	ShortCode   string
	CreatedAt   time.Time
	ExpiresAt   time.Time
}
