package user

import (
	"github.com/gin-gonic/gin"
	"github.com/reyimanuel/letter-administration/internal/infrastructures/middleware"
	"gorm.io/gorm"
)

func RegisterRoutes(r *gin.RouterGroup, db *gorm.DB) {
	repo := NewRepository(db)
	service := NewService(repo)
	handler := NewHandler(service)

	// Public routes
	r.POST("/login", handler.Login)
	r.POST("/register", handler.RegisterStudent)
	r.POST("/register/krs", handler.RegisterWithKRS)

	// Admin-only routes
	admin := r.Group("/", middleware.MiddlewareAuth, middleware.MiddlewareRole("ADMIN"))
	{
		admin.GET("/all", handler.GetAllUsers)
		admin.GET("/pending", handler.GetPendingUsers)
		admin.POST("/approve/:id", handler.ApproveUser)
		admin.DELETE("/reject/:id", handler.RejectUser)
	}
}
