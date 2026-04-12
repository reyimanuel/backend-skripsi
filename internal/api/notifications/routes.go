package notifications

import (
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func RegisterRoutes(r *gin.RouterGroup, db *gorm.DB) {
	repo := NewRepository(db)
	service := NewService(repo)
	handler := NewHandler(service)

	r.GET("/recent", handler.GetRecent)
	r.PATCH("/:id/read", handler.MarkRead)
	r.POST("/test", handler.SendTest)
}
