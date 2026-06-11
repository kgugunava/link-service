package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/kgugunava/link-service/internal/api/model"
	"github.com/kgugunava/link-service/internal/domain"
)

// =============================================================================
// Мок сервиса
// =============================================================================

// mockURLService реализует handler.URLServiceInterface для тестов
type mockURLService struct {
	mock.Mock
}

func (m *mockURLService) Shorten(ctx context.Context, originalURL string) (string, error) {
	args := m.Called(ctx, originalURL)
	return args.String(0), args.Error(1)
}

func (m *mockURLService) GetOriginal(ctx context.Context, shortCode string) (string, error) {
	args := m.Called(ctx, shortCode)
	return args.String(0), args.Error(1)
}

// =============================================================================
// Тесты для POST /shorten
// =============================================================================

func TestURLHandler_Shorten(t *testing.T) {
	// Отключаем вывод логов Gin в тестах
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name           string
		requestBody    string
		mockSetup      func(*mockURLService)
		expectedStatus int
		expectedBody   func(t *testing.T, body []byte)
	}{
		{
			name:        "201 Created on valid request",
			requestBody: `{"url":"https://example.com/path"}`,
			mockSetup: func(m *mockURLService) {
				m.On("Shorten", mock.Anything, "https://example.com/path").
					Return("Ab3_xK9mLp", nil)
			},
			expectedStatus: http.StatusCreated,
			expectedBody: func(t *testing.T, body []byte) {
				var resp model.URLShortenPostResponse
				err := json.Unmarshal(body, &resp)
				require.NoError(t, err)
				assert.Equal(t, "Ab3_xK9mLp", resp.ShortenURL)
			},
		},
		{
			name:           "400 on invalid JSON",
			requestBody:    `{"url": invalid}`,
			mockSetup:      func(m *mockURLService) {},
			expectedStatus: http.StatusBadRequest,
			expectedBody: func(t *testing.T, body []byte) {
				var resp model.ErrorResponse
				err := json.Unmarshal(body, &resp)
				require.NoError(t, err)
				assert.Equal(t, "invalid_json", resp.Error.Code)
			},
		},
		{
			name:           "400 on invalid URL format",
			requestBody:    `{"url":"not-a-valid-url"}`,
			mockSetup:      func(m *mockURLService) {},
			expectedStatus: http.StatusBadRequest,
			expectedBody: func(t *testing.T, body []byte) {
				var resp model.ErrorResponse
				err := json.Unmarshal(body, &resp)
				require.NoError(t, err)
				assert.Equal(t, "invalid_url", resp.Error.Code)
			},
		},
		{
			name:        "400 on service ErrInvalidURL",
			requestBody: `{"url":"https://example.com"}`,
			mockSetup: func(m *mockURLService) {
				m.On("Shorten", mock.Anything, "https://example.com").
					Return("", domain.ErrInvalidURL)
			},
			expectedStatus: http.StatusBadRequest,
			expectedBody: func(t *testing.T, body []byte) {
				var resp model.ErrorResponse
				err := json.Unmarshal(body, &resp)
				require.NoError(t, err)
				assert.Equal(t, "invalid_url", resp.Error.Code)
			},
		},
		{
			name:        "500 on internal service error",
			requestBody: `{"url":"https://example.com"}`,
			mockSetup: func(m *mockURLService) {
				m.On("Shorten", mock.Anything, "https://example.com").
					Return("", errors.New("database connection failed"))
			},
			expectedStatus: http.StatusInternalServerError,
			expectedBody: func(t *testing.T, body []byte) {
				var resp model.ErrorResponse
				err := json.Unmarshal(body, &resp)
				require.NoError(t, err)
				assert.Equal(t, "internal_error", resp.Error.Code)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			svc := new(mockURLService)
			tt.mockSetup(svc)
			h := NewURLHandler(svc, slog.Default())

			req := httptest.NewRequest(http.MethodPost, "/shorten", bytes.NewBufferString(tt.requestBody))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			c.Request = req

			// Act
			h.Shorten(c)

			// Assert
			assert.Equal(t, tt.expectedStatus, rec.Code)
			if tt.expectedBody != nil {
				tt.expectedBody(t, rec.Body.Bytes())
			}
			svc.AssertExpectations(t)
		})
	}
}

// =============================================================================
// Тесты для GET /:code
// =============================================================================

func TestURLHandler_GetOriginal(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name           string
		shortCode      string
		mockSetup      func(*mockURLService)
		expectedStatus int
		expectedBody   func(t *testing.T, body []byte)
	}{
		{
			name:      "200 OK with original URL on valid code",
			shortCode: "Ab3_xK9mLp",
			mockSetup: func(m *mockURLService) {
				m.On("GetOriginal", mock.Anything, "Ab3_xK9mLp").
					Return("https://example.com/path", nil)
			},
			expectedStatus: http.StatusOK,
			expectedBody: func(t *testing.T, body []byte) {
				var resp model.URLOriginalURLGetResponse
				err := json.Unmarshal(body, &resp)
				require.NoError(t, err)
				assert.Equal(t, "https://example.com/path", resp.OriginalURL)
			},
		},
		{
			name:           "400 on missing code parameter",
			shortCode:      "",
			mockSetup:      func(m *mockURLService) {},
			expectedStatus: http.StatusBadRequest,
			expectedBody: func(t *testing.T, body []byte) {
				var resp model.ErrorResponse
				err := json.Unmarshal(body, &resp)
				require.NoError(t, err)
				assert.Equal(t, "missing_code", resp.Error.Code)
			},
		},
		{
			name:           "400 on invalid code format (wrong length)",
			shortCode:      "short",
			mockSetup:      func(m *mockURLService) {},
			expectedStatus: http.StatusBadRequest,
			expectedBody: func(t *testing.T, body []byte) {
				var resp model.ErrorResponse
				err := json.Unmarshal(body, &resp)
				require.NoError(t, err)
				assert.Equal(t, "invalid_code", resp.Error.Code)
			},
		},
		{
			name:           "400 on invalid code format (invalid characters)",
			shortCode:      "bad!code#12",
			mockSetup:      func(m *mockURLService) {},
			expectedStatus: http.StatusBadRequest,
			expectedBody: func(t *testing.T, body []byte) {
				var resp model.ErrorResponse
				err := json.Unmarshal(body, &resp)
				require.NoError(t, err)
				assert.Equal(t, "invalid_code", resp.Error.Code)
			},
		},
		{
			name:      "404 on service ErrNotFound",
			shortCode: "NotExist12",
			mockSetup: func(m *mockURLService) {
				m.On("GetOriginal", mock.Anything, "NotExist12").
					Return("", domain.ErrNotFound)
			},
			expectedStatus: http.StatusNotFound,
			expectedBody: func(t *testing.T, body []byte) {
				var resp model.ErrorResponse
				err := json.Unmarshal(body, &resp)
				require.NoError(t, err)
				assert.Equal(t, "not_found", resp.Error.Code)
			},
		},
		{
			name:      "500 on internal service error",
			shortCode: "Ab3_xK9mLp",
			mockSetup: func(m *mockURLService) {
				m.On("GetOriginal", mock.Anything, "Ab3_xK9mLp").
					Return("", errors.New("database timeout"))
			},
			expectedStatus: http.StatusInternalServerError,
			expectedBody: func(t *testing.T, body []byte) {
				var resp model.ErrorResponse
				err := json.Unmarshal(body, &resp)
				require.NoError(t, err)
				assert.Equal(t, "internal_error", resp.Error.Code)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			svc := new(mockURLService)
			tt.mockSetup(svc)
			h := NewURLHandler(svc, slog.Default())

			// Формируем путь с параметром :code
			path := "/" + tt.shortCode
			req := httptest.NewRequest(http.MethodGet, path, nil)
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			c.Request = req

			// Важно: явно устанавливаем параметры пути для Gin
			if tt.shortCode != "" {
				c.Params = gin.Params{{Key: "code", Value: tt.shortCode}}
			}

			// Act
			h.GetOriginal(c)

			// Assert
			assert.Equal(t, tt.expectedStatus, rec.Code)
			if tt.expectedBody != nil {
				tt.expectedBody(t, rec.Body.Bytes())
			}
			svc.AssertExpectations(t)
		})
	}
}

// =============================================================================
// Вспомогательные тесты для функций валидации
// =============================================================================

func TestIsValidShortCode(t *testing.T) {
	tests := []struct {
		name     string
		code     string
		expected bool
	}{
		{"valid mixed case", "Ab3_xK9mLp", true},
		{"valid lowercase", "abcdefghij", true},
		{"valid uppercase", "ABCDEFGHIJ", true},
		{"valid with underscore", "A_b_1_2_3_", true},
		{"too short", "Ab3_xK9mL", false},
		{"too long", "Ab3_xK9mLpX", false},
		{"has hyphen", "Ab3-xK9mLp", false},
		{"has dot", "Ab3.xK9mLp", false},
		{"has slash", "Ab3/xK9mLp", false},
		{"has exclamation", "Ab3!xK9mLp", false},
		{"empty", "", false},
		{"cyrillic", "Абвгдежзик", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, isValidShortCode(tt.code))
		})
	}
}

func TestValidateShortenRequest(t *testing.T) {
	tests := []struct {
		name        string
		url         string
		expectError bool
	}{
		{"valid https", "https://example.com", false},
		{"valid http", "http://example.com/path?query=1", false},
		{"valid with port", "https://localhost:8080/api", false},
		{"empty", "", true},
		{"no scheme", "example.com", true},
		{"invalid chars", "https://example .com", true},
		{"just scheme", "https://", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := &model.URLShortenPostRequest{URL: tt.url}
			err := validateShortenRequest(req)
			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
