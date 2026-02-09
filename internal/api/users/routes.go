package user

import (
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func RegisterRoutes(r *gin.RouterGroup, db *gorm.DB) {
	repo := NewRepository(db)
	service := NewService(repo)
	handler := NewHandler(service)

	r.POST("/login", handler.Login)
	// r.POST("/register", handler.Register)
	// r.GET("/me", middleware.Auth(), handler.Me)
}
