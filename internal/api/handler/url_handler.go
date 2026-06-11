package handler

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/url"

	"github.com/gin-gonic/gin"

	"github.com/kgugunava/link-service/internal/api/model"
	"github.com/kgugunava/link-service/internal/domain"
)

type URLServiceInterface interface {
	Shorten(ctx context.Context, originalURL string) (string, error)
	GetOriginal(ctx context.Context, shortCode string) (string, error)
}

type URLHandler struct {
	urlService URLServiceInterface
	logger     *slog.Logger
}

func NewURLHandler(urlService URLServiceInterface, logger *slog.Logger) *URLHandler {
	if logger == nil {
		logger = slog.Default()
	}
	return &URLHandler{
		urlService: urlService,
		logger:     logger,
	}
}

func (h *URLHandler) Shorten(c *gin.Context) {
	var req model.URLShortenPostRequest

	if err := c.ShouldBindJSON(&req); err != nil {
		h.logger.Warn("invalid json", "error", err)
		respondError(c, http.StatusBadRequest, "invalid_json", "request body must be valid JSON")
		return
	}

	if err := validateShortenRequest(&req); err != nil {
		h.logger.Warn("invalid url", "url", req.URL, "error", err)
		respondError(c, http.StatusBadRequest, "invalid_url", err.Error())
		return
	}

	shortCode, err := h.urlService.Shorten(c.Request.Context(), req.URL)
	if err != nil {
		if h.handleServiceError(c, err, "shorten") {
			return
		}
		h.logger.Error("unexpected error", "operation", "shorten", "url", req.URL, "error", err)
		respondError(c, http.StatusInternalServerError, "internal_error", "failed to process request")
		return
	}

	h.logger.Info("url shortened", "original_url", req.URL, "short_code", shortCode)
	c.JSON(http.StatusCreated, model.URLShortenPostResponse{
		ShortenURL: shortCode,
	})
}

func (h *URLHandler) GetOriginal(c *gin.Context) {
	shortCode := c.Param("code")
	if shortCode == "" {
		h.logger.Warn("missing code parameter")
		respondError(c, http.StatusBadRequest, "missing_code", "short code is required")
		return
	}

	if !isValidShortCode(shortCode) {
		h.logger.Warn("invalid code format", "code", shortCode)
		respondError(c, http.StatusBadRequest, "invalid_code", "short code must be 10 characters [A-Za-z0-9_]")
		return
	}

	originalURL, err := h.urlService.GetOriginal(c.Request.Context(), shortCode)
	if err != nil {
		if h.handleServiceError(c, err, "get_original") {
			return
		}
		h.logger.Error("unexpected error", "operation", "get_original", "code", shortCode, "error", err)
		respondError(c, http.StatusInternalServerError, "internal_error", "failed to resolve URL")
		return
	}

	h.logger.Info("original url resolved", "short_code", shortCode, "original_url", originalURL)
	c.JSON(http.StatusOK, model.URLOriginalURLGetResponse{
		OriginalURL: originalURL,
	})
}

func (h *URLHandler) handleServiceError(c *gin.Context, err error, operation string) bool {
	switch {
	case errors.Is(err, domain.ErrInvalidURL):
		h.logger.Warn("invalid url", "operation", operation, "error", err)
		respondError(c, http.StatusBadRequest, "invalid_url", "invalid original URL")
		return true

	case errors.Is(err, domain.ErrNotFound):
		h.logger.Debug("not found", "operation", operation, "error", err)
		respondError(c, http.StatusNotFound, "not_found", "short URL not found")
		return true

	default:
		return false
	}
}

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

	parsed, err := url.ParseRequestURI(req.URL)
	if err != nil {
		return errors.New("url must be valid HTTP/HTTPS link")
	}

	if parsed.Host == "" {
		return errors.New("url must have a valid host")
	}

	return nil
}

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
