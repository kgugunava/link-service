package handler

import (
	"context"

	// "github.com/kgugunava/link-service/internal/service"
	// "github.com/kgugunava/link-service/internal/api/model"
)

type URLServiceInterface interface {
	Shorten(ctx context.Context, originalUrl string) (string, error) 
	GetOriginal(ctx context.Context, shortCode string) (string, error)
}

type URLHandler struct {
	urlService URLServiceInterface

}

func NewURLHandler(urlService URLServiceInterface) *URLHandler {
	return &URLHandler{
		urlService: urlService,
	}
}