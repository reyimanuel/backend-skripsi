package api

import (
	"github.com/reyimanuel/letter-administration/internal/api/correspondence"
	"github.com/reyimanuel/letter-administration/internal/api/letters"
	user "github.com/reyimanuel/letter-administration/internal/api/users"
	"github.com/reyimanuel/letter-administration/internal/infrastructures/middleware"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func RegisterRoutes(r *gin.Engine, db *gorm.DB) {
	api := r.Group("/api")

	user.RegisterRoutes(api.Group("/users"), db)

	protected := api.Group("/")
	protected.Use(middleware.MiddlewareAuth)

	letters.RegisterRoutes(protected.Group("/letters"), db)
	correspondence.RegisterRoutes(protected.Group("/correspondence"), db)
}
