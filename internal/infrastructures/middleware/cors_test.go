package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func newCORSTestRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(CORSMiddleware())
	router.GET("/test", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})
	return router
}

func TestCORSMiddlewareAllowsConfiguredOrigin(t *testing.T) {
	t.Setenv("IS_PRODUCTION", "true")
	t.Setenv("ALLOW_ORIGIN", "https://frontend.example.ac.id, https://admin.example.ac.id/")
	router := newCORSTestRouter()

	request := httptest.NewRequest(http.MethodOptions, "/test", nil)
	request.Header.Set("Origin", "https://admin.example.ac.id")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusNoContent {
		t.Fatalf("expected status 204, got %d", response.Code)
	}
	if got := response.Header().Get("Access-Control-Allow-Origin"); got != "https://admin.example.ac.id" {
		t.Fatalf("expected exact allowed origin, got %q", got)
	}
	if got := response.Header().Get("Access-Control-Allow-Credentials"); got != "true" {
		t.Fatalf("expected credentials header, got %q", got)
	}
}

func TestCORSMiddlewareRejectsUnknownPreflightOrigin(t *testing.T) {
	t.Setenv("IS_PRODUCTION", "true")
	t.Setenv("ALLOW_ORIGIN", "https://frontend.example.ac.id")
	router := newCORSTestRouter()

	request := httptest.NewRequest(http.MethodOptions, "/test", nil)
	request.Header.Set("Origin", "https://attacker.example")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusForbidden {
		t.Fatalf("expected status 403, got %d", response.Code)
	}
	if got := response.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Fatalf("unexpected allow-origin header %q", got)
	}
}

func TestCORSMiddlewareDoesNotAllowWildcard(t *testing.T) {
	t.Setenv("IS_PRODUCTION", "true")
	t.Setenv("ALLOW_ORIGIN", "*")
	t.Setenv("FRONTEND_URL", "https://frontend.example.ac.id/")
	router := newCORSTestRouter()

	request := httptest.NewRequest(http.MethodOptions, "/test", nil)
	request.Header.Set("Origin", "https://frontend.example.ac.id")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusNoContent {
		t.Fatalf("expected status 204, got %d", response.Code)
	}
	if got := response.Header().Get("Access-Control-Allow-Origin"); got == "*" {
		t.Fatal("wildcard origin must never be returned")
	}
}
