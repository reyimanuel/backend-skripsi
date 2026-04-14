package letters

import (
	"github.com/gin-gonic/gin"
	"github.com/reyimanuel/letter-administration/internal/infrastructures/middleware"
	"gorm.io/gorm"
)

func RegisterRoutes(r *gin.RouterGroup, db *gorm.DB) {
	repo := NewRepository(db)
	service := NewService(repo)
	handler := NewHandler(service)

	// v2: upload + analyze placeholders
	r.POST("/templates/upload", middleware.MiddlewareRole("ADMIN"), handler.UploadTemplateV2)

	r.DELETE("/template/:id", middleware.MiddlewareRole("ADMIN"), handler.DeleteTemplate)
	r.GET("/templates", handler.GetAllTemplates)
	r.GET("/preview/:id", handler.PreviewTemplate)

	r.GET("/requirements/:id", handler.GetAttachmentRequirements)
	r.PUT("/requirements/:id", middleware.MiddlewareRole("ADMIN"), handler.UpdateAttachmentRequirements)
}
