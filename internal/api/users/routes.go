package user

import (
	"github.com/gin-gonic/gin"
	"github.com/reyimanuel/letter-administration/internal/infrastructures/middleware"
	"gorm.io/gorm"
	"time"
)

func RegisterRoutes(r *gin.RouterGroup, db *gorm.DB) {
	repo := NewRepository(db)
	service := NewService(repo)
	handler := NewHandler(service)

	// Public routes - login and refresh with strict rate limiting
	publicAuth := r.Group("")
	publicAuth.Use(middleware.IPBasedLimiter(5, 5, 1*time.Minute)) // 5 requests per minute for login attempts
	{
		publicAuth.POST("/login", handler.Login)
		publicAuth.POST("/refresh", handler.RefreshToken)
	}
	
	// Public routes - registration and verification with moderate rate limiting
	publicReg := r.Group("")
	publicReg.Use(middleware.IPBasedLimiter(20, 20, 1*time.Hour)) // 20 requests per hour for registration/verification
	{
		publicReg.POST("/register", handler.RegisterStudent)
		publicReg.POST("/register/krs", handler.RegisterWithKRS)
		publicReg.POST("/verify-email", handler.VerifyEmail)
		publicReg.POST("/resend-verification", handler.ResendVerificationEmail)
	}
	
	// Protected routes (require authentication)
	r.POST("/logout", middleware.MiddlewareAuth, handler.Logout)
	r.GET("/me", middleware.MiddlewareAuth, handler.GetMe)
	r.PATCH("/me", middleware.MiddlewareAuth, handler.UpdateMyProfile)
	r.POST("/me/fcm-token", middleware.MiddlewareAuth, handler.UpsertFCMToken)
	r.DELETE("/me/fcm-token", middleware.MiddlewareAuth, handler.DeleteFCMToken)

	// Admin-only routes
	admin := r.Group("/", middleware.MiddlewareAuth, middleware.MiddlewareRole("ADMIN"))
	{
		admin.GET("/all", handler.GetAllUsers)
		admin.PATCH("/:id", handler.AdminUpdateUser)
		admin.DELETE("/:id", handler.AdminDeleteUser)
		admin.GET("/pending/students", handler.GetPendingStudents)
		admin.POST("/students/:id/approve", handler.ApproveStudent)
		admin.POST("/students/:id/reject", handler.RejectStudent)
		admin.POST("/staffs", handler.CreateStaff)
	}
}
