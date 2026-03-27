package letters

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/reyimanuel/letter-administration/internal/infrastructures/middleware"
	"github.com/reyimanuel/letter-administration/internal/infrastructures/pkg/errs"
)

type Handler struct {
	Service *Service
}

func NewHandler(service *Service) *Handler {
	return &Handler{Service: service}
}

func (h *Handler) UploadTemplate(ctx *gin.Context) {
	userID, err := middleware.GetUserID(ctx)
	if err != nil {
		errs.HandlerError(ctx, errs.Unauthorized("pengguna tidak terautentikasi"))
		return
	}

	letterTypeIDParam := ctx.Param("id")
	letterTypeID, err := strconv.ParseUint(letterTypeIDParam, 10, 64)
	if err != nil {
		errs.HandlerError(ctx, errs.BadRequest("tipe surat tidak valid"))
		return
	}

	file, err := ctx.FormFile("file")
	if err != nil {
		errs.HandlerError(ctx, errs.BadRequest("file tidak ditemukan"))
		return
	}
	fmt.Printf("Received file: %s (%d bytes)\n", file.Filename, file.Size)

	response, err := h.Service.UploadTemplate(userID, uint(letterTypeID), file)
	if err != nil {
		errs.HandlerError(ctx, err)
		return
	}

	ctx.JSON(http.StatusOK, response)
}

func (h *Handler) UploadTemplateFlexible(ctx *gin.Context) {
	userID, err := middleware.GetUserID(ctx)
	if err != nil {
		errs.HandlerError(ctx, errs.Unauthorized("pengguna tidak terautentikasi"))
		return
	}

	var req UploadTemplateFlexibleRequest
	if err := ctx.ShouldBind(&req); err != nil {
		errs.HandlerError(ctx, errs.BadRequest("Data upload template tidak valid"))
		return
	}

	file, err := ctx.FormFile("file")
	if err != nil {
		errs.HandlerError(ctx, errs.BadRequest("file tidak ditemukan"))
		return
	}

	response, err := h.Service.UploadTemplateFlexible(userID, req, file)
	if err != nil {
		errs.HandlerError(ctx, err)
		return
	}

	ctx.JSON(response.StatusCode, response)
}

func (h *Handler) PreviewTemplate(ctx *gin.Context) {
	idParam := ctx.Param("id")
	letterTypeID, err := strconv.ParseUint(idParam, 10, 64)
	if err != nil {
		errs.HandlerError(ctx, errs.BadRequest("id tidak valid"))
		return
	}

	pdfPath, err := h.Service.PreviewTemplate(uint(letterTypeID))
	if err != nil {
		errs.HandlerError(ctx, err)
		return
	}

	file, err := os.Open(pdfPath)
	if err != nil {
		log.Printf("failed opening template preview: letter_type_id=%d path=%q err=%v", letterTypeID, pdfPath, err)
		errs.HandlerError(ctx, errs.InternalServerError("Gagal membuka file"))
		return
	}
	defer file.Close()

	ctx.Header("Content-Type", "application/pdf")
	ctx.Header("Content-Disposition", "inline; filename=preview.pdf")

	http.ServeContent(ctx.Writer, ctx.Request, "preview.pdf", time.Now(), file)
}

func (h *Handler) DeleteTemplate(ctx *gin.Context) {
	idParam := ctx.Param("id")
	letterTypeID, err := strconv.ParseUint(idParam, 10, 64)
	if err != nil {
		errs.HandlerError(ctx, errs.BadRequest("id tidak valid"))
		return
	}

	response, err := h.Service.DeleteTemplate(uint(letterTypeID))
	if err != nil {
		errs.HandlerError(ctx, err)
		return
	}

	ctx.JSON(response.StatusCode, response)
}

func (h *Handler) GetAllTemplates(ctx *gin.Context) {
	response, err := h.Service.GetAllTemplates()
	if err != nil {
		errs.HandlerError(ctx, err)
		return
	}

	ctx.JSON(response.StatusCode, response)
}

func (h *Handler) GetAttachmentRequirements(ctx *gin.Context) {
	idParam := ctx.Param("id")
	letterTypeID, err := strconv.ParseUint(idParam, 10, 64)
	if err != nil {
		errs.HandlerError(ctx, errs.BadRequest("id tidak valid"))
		return
	}

	response, err := h.Service.GetAttachmentRequirements(uint(letterTypeID))
	if err != nil {
		errs.HandlerError(ctx, err)
		return
	}

	ctx.JSON(response.StatusCode, response)
}

func (h *Handler) UpdateAttachmentRequirements(ctx *gin.Context) {
	// ADMIN-only via route middleware
	idParam := ctx.Param("id")
	letterTypeID, err := strconv.ParseUint(idParam, 10, 64)
	if err != nil {
		errs.HandlerError(ctx, errs.BadRequest("id tidak valid"))
		return
	}

	var req UpdateAttachmentRequirementsRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		errs.HandlerError(ctx, errs.BadRequest("Data requirements tidak valid"))
		return
	}

	response, err := h.Service.UpdateAttachmentRequirements(uint(letterTypeID), req)
	if err != nil {
		errs.HandlerError(ctx, err)
		return
	}

	ctx.JSON(response.StatusCode, response)
}
