package api

import (
	"github.com/reyimanuel/letter-administration/internal/api/correspondence"
	"github.com/reyimanuel/letter-administration/internal/api/letters"
	user "github.com/reyimanuel/letter-administration/internal/api/users"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func RegisterRoutes(r *gin.Engine, db *gorm.DB) {
	api := r.Group("/api")

	user.RegisterRoutes(api.Group("/users"), db)
	letters.RegisterRoutes(api.Group("/letters"), db)
	correspondence.RegisterRoutes(api.Group("/correspondence"), db)
}
