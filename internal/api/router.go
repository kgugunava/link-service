package api

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/kgugunava/link-service/internal/api/handler"
)

type Route struct {
	// Name is the name of this Route.
	Name string
	// Method is the string for the HTTP method. ex) GET, POST etc..
	Method string
	// Pattern is the pattern of the URI.
	Pattern string
	// HandlerFunc is the handler function of this route.
	HandlerFunc gin.HandlerFunc
}

func NewRouter(URLHandler *handler.URLHandler) *gin.Engine {
	return NewRouterWithGinEngine(gin.Default(), URLHandler)
}

func NewRouterWithGinEngine(router *gin.Engine, URLHandler *handler.URLHandler) *gin.Engine {
	for _, route := range getRoutes(URLHandler) {
		if route.HandlerFunc == nil {
			route.HandlerFunc = DefaultHandleFunc
		}
		switch route.Method {
		case http.MethodGet:
			router.GET(route.Pattern, route.HandlerFunc)
		case http.MethodPost:
			router.POST(route.Pattern, route.HandlerFunc)
		case http.MethodPut:
			router.PUT(route.Pattern, route.HandlerFunc)
		case http.MethodPatch:
			router.PATCH(route.Pattern, route.HandlerFunc)
		case http.MethodDelete:
			router.DELETE(route.Pattern, route.HandlerFunc)
		}
	}

	return router
}

func DefaultHandleFunc(c *gin.Context) {
	c.String(http.StatusNotImplemented, "501 not implemented")
}

func getRoutes(URLHandler *handler.URLHandler) []Route {
	return []Route{
		{
			"ShortenURL",
			http.MethodPost,
			"/api/shorten",
			URLHandler.Shorten,
		},
		{
			"GetOriginalURL",
			http.MethodGet,
			"/api/original/:code",
			URLHandler.GetOriginal,
		},
	}
}
