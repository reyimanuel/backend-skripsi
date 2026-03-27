package correspondence

import (
	"github.com/gin-gonic/gin"
	"github.com/reyimanuel/letter-administration/internal/api/letters"
	user "github.com/reyimanuel/letter-administration/internal/api/users"
	"github.com/reyimanuel/letter-administration/internal/infrastructures/middleware"
	"gorm.io/gorm"
)

func RegisterRoutes(r *gin.RouterGroup, db *gorm.DB) {
	repo := NewRepository(db)
	service := NewService(repo, letters.NewRepository(db), user.NewRepository(db))
	handler := NewHandler(service)

	r.GET("/letters", middleware.MiddlewareRole("MAHASISWA", "ADMIN"), handler.ListLetters)
	r.POST("/submit", middleware.MiddlewareRole("MAHASISWA"), handler.CreateSubmitLetter)
	r.DELETE("/:id", middleware.MiddlewareRole("MAHASISWA", "ADMIN"), handler.DeleteLetter)
	r.GET("/preview/:id", middleware.MiddlewareRole("MAHASISWA", "ADMIN"), handler.PreviewLetter)
	r.GET("/history/:id", middleware.MiddlewareRole("MAHASISWA", "ADMIN"), handler.GetLetterHistory)
	r.PATCH("/approve/:id", middleware.MiddlewareRole("ADMIN"), handler.ApproveLetter)
	r.POST("/:id/attachments", middleware.MiddlewareRole("MAHASISWA", "ADMIN"), handler.UploadAttachments)
}
