package domain

import "errors"

var (
	ErrNotFound   = errors.New("not found")
	ErrInvalidURL = errors.New("invalid URL")
	ErrInvalidCode = errors.New("invalid short code")
	ErrShortCodeCollision = errors.New("short code collision")
)