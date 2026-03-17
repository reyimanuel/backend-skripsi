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
	r.POST("/verify-email", handler.VerifyEmail)
	r.POST("/resend-verification", handler.ResendVerificationEmail)
	r.GET("/me", middleware.MiddlewareAuth, handler.GetMe)

	// Admin-only routes
	admin := r.Group("/", middleware.MiddlewareAuth, middleware.MiddlewareRole("ADMIN"))
	{
		admin.GET("/all", handler.GetAllUsers)
		admin.PATCH("/:id", handler.AdminUpdateUser)
		admin.DELETE("/:id", handler.AdminDeleteUser)
		admin.GET("/pending/students", handler.GetPendingStudents)
		admin.POST("/students/:id/approve", handler.ApproveStudent)
		admin.POST("/students/:id/reject", handler.RejectStudent)
		admin.POST("/officials", handler.CreateOfficial)
	}
}
