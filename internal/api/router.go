package api

import (
	"github.com/reyimanuel/letter-administration/internal/api/user"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func RegisterRoutes(r *gin.Engine, db *gorm.DB) {
	api := r.Group("/api")

	user.RegisterRoutes(api.Group("/users"), db)
}
