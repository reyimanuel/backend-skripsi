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

	r.POST("/upload", middleware.MiddlewareRole("ADMIN"), handler.UploadTemplateFlexible)
	r.POST("/upload/:id", middleware.MiddlewareRole("ADMIN"), handler.UploadTemplate)
	r.DELETE("/template/:id", middleware.MiddlewareRole("ADMIN"), handler.DeleteTemplate)
	r.GET("/templates", handler.GetAllTemplates)
	r.GET("/preview/:id", handler.PreviewTemplate)
}
