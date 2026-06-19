package api

import (
	"github.com/reyimanuel/letter-administration/internal/api/correspondence"
	"github.com/reyimanuel/letter-administration/internal/api/events"
	"github.com/reyimanuel/letter-administration/internal/api/letters"
	"github.com/reyimanuel/letter-administration/internal/api/notifications"
	user "github.com/reyimanuel/letter-administration/internal/api/users"
	"github.com/reyimanuel/letter-administration/internal/infrastructures/middleware"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func RegisterRoutes(r *gin.Engine, db *gorm.DB) {
	r.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "ok"})
	})

	api := r.Group("/api")

	user.RegisterRoutes(api.Group("/users"), db)

	protected := api.Group("/")
	protected.Use(middleware.MiddlewareAuth)

	events.RegisterRoutes(protected.Group("/events"))
	letters.RegisterRoutes(protected.Group("/letters"), db)
	correspondence.RegisterRoutes(protected.Group("/correspondence"), db)
	notifications.RegisterRoutes(protected.Group("/notifications"), db)
}
