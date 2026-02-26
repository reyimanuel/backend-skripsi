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

	r.POST("/submit", middleware.MiddlewareRole("MAHASISWA"), handler.CreateSubmitLetter)
	r.PATCH("/approve/:id", middleware.MiddlewareRole("ADMIN"), handler.ApproveLetter)
}
