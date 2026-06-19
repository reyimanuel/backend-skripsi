package middleware

import (
	"log"
	"os"
	"strings"

	"github.com/gin-gonic/gin"
)

func configuredCORSOrigins() map[string]struct{} {
	raw := strings.TrimSpace(os.Getenv("ALLOW_ORIGIN"))
	if raw == "" || raw == "*" {
		if raw == "*" {
			log.Print("cors: wildcard ALLOW_ORIGIN is not permitted; using FRONTEND_URL or local defaults")
		}
		raw = strings.TrimSpace(os.Getenv("FRONTEND_URL"))
	}

	if raw == "" && !strings.EqualFold(strings.TrimSpace(os.Getenv("IS_PRODUCTION")), "true") {
		raw = "http://localhost:3000,http://127.0.0.1:3000"
	}

	origins := make(map[string]struct{})
	for _, item := range strings.Split(raw, ",") {
		origin := strings.TrimRight(strings.TrimSpace(item), "/")
		if origin == "" || origin == "*" {
			continue
		}
		origins[origin] = struct{}{}
	}

	return origins
}

func CORSMiddleware() gin.HandlerFunc {
	allowedOrigins := configuredCORSOrigins()

	return func(c *gin.Context) {
		origin := strings.TrimRight(strings.TrimSpace(c.GetHeader("Origin")), "/")
		if origin != "" {
			if _, allowed := allowedOrigins[origin]; !allowed {
				if c.Request.Method == "OPTIONS" {
					c.AbortWithStatusJSON(403, gin.H{"message": "Origin tidak diizinkan"})
					return
				}
				c.Next()
				return
			}

			c.Writer.Header().Set("Access-Control-Allow-Origin", origin)
			c.Writer.Header().Set("Access-Control-Allow-Credentials", "true")
			c.Writer.Header().Add("Vary", "Origin")
		}

		c.Writer.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, PATCH, OPTIONS")
		c.Writer.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		c.Writer.Header().Set("Access-Control-Max-Age", "600")

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}

		c.Next()
	}
}
