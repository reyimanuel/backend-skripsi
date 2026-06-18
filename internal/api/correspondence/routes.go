package correspondence

import (
	"github.com/gin-gonic/gin"
	"github.com/reyimanuel/letter-administration/internal/api/letters"
	user "github.com/reyimanuel/letter-administration/internal/api/users"
	"github.com/reyimanuel/letter-administration/internal/constants"
	"github.com/reyimanuel/letter-administration/internal/infrastructures/middleware"
	"gorm.io/gorm"
)

func adminAndOfficialRoles() []string {
	return append([]string{"ADMIN"}, constants.OfficialRoleCodes...)
}

func RegisterRoutes(r *gin.RouterGroup, db *gorm.DB) {
	repo := NewRepository(db)
	service := NewService(repo, letters.NewRepository(db), user.NewRepository(db))
	handler := NewHandler(service)

	r.GET("/letters", middleware.MiddlewareRole("MAHASISWA", "ADMIN"), handler.ListLetters)
	r.GET("/forwarded", middleware.MiddlewareRole(constants.OfficialRoleCodes...), handler.ListForwardedLetters)
	r.GET("/officials", middleware.MiddlewareRole(adminAndOfficialRoles()...), handler.ListActiveOfficials)
	r.POST("/drafts", middleware.MiddlewareRole("MAHASISWA"), handler.CreateDraftLetter)
	r.PATCH("/drafts/:id", middleware.MiddlewareRole("MAHASISWA"), handler.UpdateDraftLetter)
	r.POST("/submit/:id", middleware.MiddlewareRole("MAHASISWA"), handler.SubmitDraftLetter)
	r.DELETE("/:id", middleware.MiddlewareRole("MAHASISWA", "ADMIN"), handler.DeleteLetter)
	r.GET("/preview/:id", middleware.MiddlewareRole(append([]string{"MAHASISWA"}, adminAndOfficialRoles()...)...), handler.PreviewLetter)
	r.GET("/history/:id", middleware.MiddlewareRole(append([]string{"MAHASISWA"}, adminAndOfficialRoles()...)...), handler.GetHistoryAndDetail)
	r.PATCH("/approve/:id", middleware.MiddlewareRole(adminAndOfficialRoles()...), handler.ReviewLetter)
	r.POST("/attachments/:id", middleware.MiddlewareRole("MAHASISWA", "ADMIN"), handler.UploadAttachments)
}
