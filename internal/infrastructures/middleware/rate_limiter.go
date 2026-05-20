package middleware

import (
	"fmt"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/time/rate"
)

// RateLimiterConfig holds the configuration for the rate limiter middleware
type RateLimiterConfig struct {
	Requests  int                       // Maximum number of requests allowed
	Burst     int                       // Maximum burst size
	Duration  time.Duration             // Time window for the rate limit
	KeyGetter func(*gin.Context) string // Function to extract key from context (e.g., IP, user ID)
}

// RateLimiter returns a gin.HandlerFunc that limits the rate of requests.
func RateLimiter(config RateLimiterConfig) gin.HandlerFunc {
	requests := config.Requests
	if requests <= 0 {
		requests = 1
	}
	burst := config.Burst
	if burst <= 0 {
		burst = requests
	}
	if config.Duration <= 0 {
		config.Duration = time.Minute
	}

	type clientLimiter struct {
		limiter  *rate.Limiter
		lastSeen time.Time
	}

	limiters := make(map[string]*clientLimiter)
	var mu sync.Mutex
	limit := rate.Every(config.Duration / time.Duration(requests))

	getKey := config.KeyGetter
	if getKey == nil {
		getKey = func(c *gin.Context) string {
			return c.ClientIP()
		}
	}

	return func(c *gin.Context) {
		key := getKey(c)
		if key == "" {
			key = c.ClientIP()
		}

		now := time.Now()
		mu.Lock()
		cl, exists := limiters[key]
		if !exists {
			cl = &clientLimiter{
				limiter: rate.NewLimiter(limit, burst),
			}
			limiters[key] = cl
		}
		cl.lastSeen = now

		// Opportunistic cleanup to avoid unbounded growth in long-running processes.
		for k, item := range limiters {
			if now.Sub(item.lastSeen) > config.Duration*2 {
				delete(limiters, k)
			}
		}
		allowed := cl.limiter.Allow()
		mu.Unlock()

		if allowed {
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
