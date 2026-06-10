package handler

import (
	"context"
	"errors"
	"net/http"
	"net/url"

	"github.com/gin-gonic/gin"

	"github.com/kgugunava/link-service/internal/api/model"
)

type URLServiceInterface interface {
	Shorten(ctx context.Context, originalUrl string) (string, error) 
	GetOriginal(ctx context.Context, shortCode string) (string, error)
}

type URLHandler struct {
	urlService URLServiceInterface
}

var (
	ErrInvalidURL = errors.New("invalid original URL")
	ErrNotFound   = errors.New("short code not found")
)

func NewURLHandler(urlService URLServiceInterface) *URLHandler {
	return &URLHandler{
		urlService: urlService,
	}
}

func (h *URLHandler) Shorten(c *gin.Context) {
	var req model.URLShortenPostRequest

	// 1. Парсинг JSON-тела
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, "invalid_json", "request body must be valid JSON")
		return
	}

	// 2. Валидация входных данных
	if err := validateShortenRequest(&req); err != nil {
		respondError(c, http.StatusBadRequest, "invalid_url", err.Error())
		return
	}

		// 3. Вызов бизнес-логики
	shortCode, err := h.urlService.Shorten(c.Request.Context(), req.URL)
	if err != nil {
		// Маппинг доменных ошибок сервиса в HTTP-статусы
		if errors.Is(err, ErrInvalidURL) {
			respondError(c, http.StatusBadRequest, "invalid_url", err.Error())
			return
		}
		// Любая другая ошибка (БД, генератор) → 500
		respondError(c, http.StatusInternalServerError, "internal_error", "failed to process request")
		return
	}

	// 4. Успешный ответ (201 Created)
	c.JSON(http.StatusCreated, model.URLShortenPostResponse{
		ShortenURL: shortCode,
	})
}

func (h *URLHandler) GetOriginal(c *gin.Context) {
	// Извлекаем short_code из пути: /Ab3_xK9mLp
	shortCode := c.Param("code")
	if shortCode == "" {
		respondError(c, http.StatusBadRequest, "missing_code", "short code is required")
		return
	}

	// Валидация формата короткого кода
	if !isValidShortCode(shortCode) {
		respondError(c, http.StatusBadRequest, "invalid_code", "short code must be 10 characters [A-Za-z0-9_]")
		return
	}

	// Запрос к сервису
	originalURL, err := h.urlService.GetOriginal(c.Request.Context(), shortCode)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			respondError(c, http.StatusNotFound, "not_found", "short URL not found")
			return
		}
		respondError(c, http.StatusInternalServerError, "internal_error", "failed to resolve URL")
		return
	}

	c.JSON(http.StatusOK, model.URLOriginalURLGetResponse{
		OriginalURL: originalURL,
	})

}

// respondError унифицирует формат ошибок и устанавливает корректный HTTP-статус
func respondError(c *gin.Context, status int, code, message string) {
	c.JSON(status, model.ErrorResponse{
		Error: model.ErrorResponseError{
			Code:    code,
			Message: message,
		},
	})
}

func validateShortenRequest(req *model.URLShortenPostRequest) error {
	if req.URL == "" {
		return errors.New("url field is required")
	}
	// Проверка формата через стандартную библиотеку
	_, err := url.ParseRequestURI(req.URL)
	if err != nil {
		return errors.New("url must be valid HTTP/HTTPS link")
	}
	return nil
}

// isValidShortCode проверяет формат короткого кода: ровно 10 символов [A-Za-z0-9_]
func isValidShortCode(code string) bool {
	if len(code) != 10 {
		return false
	}
	for _, c := range code {
		if !((c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '_') {
			return false
		}
	}
	return true
}