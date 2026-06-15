package routes

import (
	"net/http"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/custom/imagegen/handler"

	"github.com/gin-gonic/gin"
)

func TestRegisterPublicGalleryRouteAddsCustomOpenAIRoutesOnly(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	group := router.Group("/api/v1")

	RegisterPublicGalleryRoute(group, &handler.ImageGenerationHandler{})

	routes := router.Routes()
	want := map[string]bool{
		"GET /api/v1/custom/gallery/images":                false,
		"POST /api/v1/custom/openai/v1/images/generations": false,
		"POST /api/v1/custom/openai/v1/images/edits":       false,
	}
	for _, route := range routes {
		key := route.Method + " " + route.Path
		if _, ok := want[key]; ok {
			want[key] = true
		}
		if strings.HasPrefix(route.Path, "/api/v1/images/") || strings.HasPrefix(route.Path, "/v1/images/") {
			t.Fatalf("custom imagegen must not register main gateway image path: %s %s", route.Method, route.Path)
		}
	}
	for key, seen := range want {
		if !seen {
			t.Fatalf("route %s was not registered; got %+v", key, routes)
		}
	}
}

func TestRegisterUserRoutesDoesNotRegisterOpenAICompatibleRoutes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	group := router.Group("/api/v1")

	RegisterUserRoutes(group, &handler.ImageGenerationHandler{})

	for _, route := range router.Routes() {
		if route.Method == http.MethodPost && strings.HasPrefix(route.Path, "/api/v1/custom/openai/") {
			t.Fatalf("OpenAI compatible route must stay outside JWT user route group: %s", route.Path)
		}
	}
}
