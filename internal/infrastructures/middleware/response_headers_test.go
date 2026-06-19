package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestResponseHeadersSetsNoSniff(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(ResponseHeaders())
	router.GET("/test", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	request := httptest.NewRequest(http.MethodGet, "/test", nil)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if got := response.Header().Get("X-Content-Type-Options"); got != "nosniff" {
		t.Fatalf("expected nosniff header, got %q", got)
	}
}
