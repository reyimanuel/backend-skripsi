package middleware

import (
	"fmt"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/time/rate"
)

// RateLimiterConfig holds the configuration for the rate limiter middleware
type RateLimiterConfig struct {
	Requests  int           // Maximum number of requests allowed
	Burst     int           // Maximum burst size
	Duration  time.Duration // Time window for the rate limit
	KeyGetter func(*gin.Context) string // Function to extract key from context (e.g., IP, user ID)
}

// RateLimiter returns a gin.HandlerFunc that limits the rate of requests.
func RateLimiter(config RateLimiterConfig) gin.HandlerFunc {
	limiter := rate.NewLimiter(rate.Every(config.Duration), config.Burst)

	return func(c *gin.Context) {
		if limiter.Allow() {
			c.Next()
			return
		}
		c.AbortWithStatusJSON(429, gin.H{
			"error":   "Too Many Requests",
			"message": "Rate limit exceeded. Please try again later.",
		})
}
}

// IPBasedLimiter returns a rate limiter middleware keyed by IP address
func IPBasedLimiter(requests int, burst int, duration time.Duration) gin.HandlerFunc {
	return RateLimiter(RateLimiterConfig{
		Requests: requests,
		Burst:    burst,
		Duration: duration,
		KeyGetter: func(c *gin.Context) string {
			return c.ClientIP()
		},
	})
}

// UserIDBasedLimiter returns a rate limiter middleware keyed by user ID (from auth context)
func UserIDBasedLimiter(requests int, burst int, duration time.Duration) gin.HandlerFunc {
	return RateLimiter(RateLimiterConfig{
		Requests: requests,
		Burst:    burst,
		Duration: duration,
		KeyGetter: func(c *gin.Context) string {
			if userID, exists := c.Get("user_id"); exists {
				if uid, ok := userID.(uint); ok {
					return fmt.Sprintf("user_%d", uid)
				}
			}
			return c.ClientIP() // Fallback to IP if user not authenticated
		},
	})
}