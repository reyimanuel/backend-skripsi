package correspondence

import (
	"fmt"
	"log"
	"mime/multipart"
	"net/http"
	"os"
	"slices"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/reyimanuel/letter-administration/internal/infrastructures/middleware"
	"github.com/reyimanuel/letter-administration/internal/infrastructures/pkg/errs"
	"github.com/reyimanuel/letter-administration/internal/infrastructures/pkg/token"
)

type Handler struct {
	Service *Service
}

func NewHandler(service *Service) *Handler {
	return &Handler{Service: service}
}

func (h *Handler) CreateSubmitLetter(ctx *gin.Context) {
	userID, err := middleware.GetUserID(ctx)
	if err != nil {
		errs.HandlerError(ctx, errs.Unauthorized("user tidak terautentikasi"))
		return
	}

	var req SubmitLetterRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		errs.HandlerError(ctx, errs.BadRequest("Data pengajuan surat tidak valid"))
		return
	}

	response, err := h.Service.CreateSubmitLetter(userID, req)
	if err != nil {
		errs.HandlerError(ctx, err)
		return
	}

	ctx.JSON(http.StatusOK, response)
}

func (h *Handler) ApproveLetter(ctx *gin.Context) {
	userID, err := middleware.GetUserID(ctx)
	if err != nil {
		errs.HandlerError(ctx, errs.Unauthorized("user tidak terautentikasi"))
		return
	}

	var req ApproveLetterRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		errs.HandlerError(ctx, errs.BadRequest("Data persetujuan surat tidak valid"))
		return
	}

	letterIDParam := ctx.Param("id")
	letterID, err := strconv.Atoi(letterIDParam)
	if err != nil {
		errs.HandlerError(ctx, errs.BadRequest("surat tidak valid"))
		return
	}

	response, err := h.Service.ApproveLetter(uint(letterID), userID, req)

	if err != nil {
		errs.HandlerError(ctx, err)
		return
	}

	ctx.JSON(http.StatusOK, response)
}

func (h *Handler) PreviewLetter(ctx *gin.Context) {
	userID, err := middleware.GetUserID(ctx)
	if err != nil {
		errs.HandlerError(ctx, errs.Unauthorized("user tidak terautentikasi"))
		return
	}

	auth, exists := ctx.Get("auth")
	if !exists {
		errs.HandlerError(ctx, errs.Unauthorized("user tidak terautentikasi"))
		return
	}

	claims, ok := auth.(*token.UserAuthToken)
	if !ok {
		errs.HandlerError(ctx, errs.Unauthorized("user tidak terautentikasi"))
		return
	}

	letterIDParam := ctx.Param("id")
	letterID, err := strconv.Atoi(letterIDParam)
	if err != nil {
		errs.HandlerError(ctx, errs.BadRequest("surat tidak valid"))
		return
	}

	pdfPath, fileName, err := h.Service.PreviewLetter(uint(letterID), userID, slices.Contains(claims.Roles, "ADMIN"))
	if err != nil {
		errs.HandlerError(ctx, err)
		return
	}

	file, err := os.Open(pdfPath)
	if err != nil {
		log.Printf("failed opening preview file: letter_id=%d path=%q err=%v", letterID, pdfPath, err)
		errs.HandlerError(ctx, errs.InternalServerError("Gagal membuka file surat"))
		return
	}
	defer file.Close()

	ctx.Header("Content-Type", "application/pdf")
	ctx.Header("Content-Disposition", fmt.Sprintf("inline; filename=%q", fileName))

	http.ServeContent(ctx.Writer, ctx.Request, fileName, time.Now(), file)
}

func (h *Handler) DeleteLetter(ctx *gin.Context) {
	userID, err := middleware.GetUserID(ctx)
	if err != nil {
		errs.HandlerError(ctx, errs.Unauthorized("user tidak terautentikasi"))
		return
	}

	auth, exists := ctx.Get("auth")
	if !exists {
		errs.HandlerError(ctx, errs.Unauthorized("user tidak terautentikasi"))
		return
	}

	claims, ok := auth.(*token.UserAuthToken)
	if !ok {
		errs.HandlerError(ctx, errs.Unauthorized("user tidak terautentikasi"))
		return
	}

	letterIDParam := ctx.Param("id")
	letterID, err := strconv.Atoi(letterIDParam)
	if err != nil {
		errs.HandlerError(ctx, errs.BadRequest("surat tidak valid"))
		return
	}

	response, err := h.Service.DeleteLetter(uint(letterID), userID, slices.Contains(claims.Roles, "ADMIN"))
	if err != nil {
		errs.HandlerError(ctx, err)
		return
	}

	ctx.JSON(response.StatusCode, response)
}

func (h *Handler) GetLetterHistory(ctx *gin.Context) {
	userID, err := middleware.GetUserID(ctx)
	if err != nil {
		errs.HandlerError(ctx, errs.Unauthorized("user tidak terautentikasi"))
		return
	}

	auth, exists := ctx.Get("auth")
	if !exists {
		errs.HandlerError(ctx, errs.Unauthorized("user tidak terautentikasi"))
		return
	}

	claims, ok := auth.(*token.UserAuthToken)
	if !ok {
		errs.HandlerError(ctx, errs.Unauthorized("user tidak terautentikasi"))
		return
	}

	letterIDParam := ctx.Param("id")
	letterID, err := strconv.Atoi(letterIDParam)
	if err != nil {
		errs.HandlerError(ctx, errs.BadRequest("surat tidak valid"))
		return
	}

	response, err := h.Service.GetLetterHistory(uint(letterID), userID, slices.Contains(claims.Roles, "ADMIN"))
	if err != nil {
		errs.HandlerError(ctx, err)
		return
	}

	ctx.JSON(response.StatusCode, response)
}

func (h *Handler) ListLetters(ctx *gin.Context) {
	userID, err := middleware.GetUserID(ctx)
	if err != nil {
		errs.HandlerError(ctx, errs.Unauthorized("user tidak terautentikasi"))
		return
	}

	auth, exists := ctx.Get("auth")
	if !exists {
		errs.HandlerError(ctx, errs.Unauthorized("user tidak terautentikasi"))
		return
	}

	claims, ok := auth.(*token.UserAuthToken)
	if !ok {
		errs.HandlerError(ctx, errs.Unauthorized("user tidak terautentikasi"))
		return
	}

	var q ListLettersQuery
	if err := ctx.ShouldBindQuery(&q); err != nil {
		errs.HandlerError(ctx, errs.BadRequest("Parameter pencarian surat tidak valid"))
		return
	}

	response, err := h.Service.ListLetters(userID, slices.Contains(claims.Roles, "ADMIN"), q)
	if err != nil {
		errs.HandlerError(ctx, err)
		return
	}

	ctx.JSON(response.StatusCode, response)
}

func (h *Handler) UploadAttachments(ctx *gin.Context) {
	userID, err := middleware.GetUserID(ctx)
	if err != nil {
		errs.HandlerError(ctx, errs.Unauthorized("user tidak terautentikasi"))
		return
	}

	auth, exists := ctx.Get("auth")
	if !exists {
		errs.HandlerError(ctx, errs.Unauthorized("user tidak terautentikasi"))
		return
	}

	claims, ok := auth.(*token.UserAuthToken)
	if !ok {
		errs.HandlerError(ctx, errs.Unauthorized("user tidak terautentikasi"))
		return
	}

	letterIDParam := ctx.Param("id")
	letterID, err := strconv.Atoi(letterIDParam)
	if err != nil {
		errs.HandlerError(ctx, errs.BadRequest("surat tidak valid"))
		return
	}

	form, err := ctx.MultipartForm()
	if err != nil {
		errs.HandlerError(ctx, errs.BadRequest("Data upload tidak valid"))
		return
	}

	files := form.File["files"]
	if len(files) == 0 {
		// fallback: allow single file named "file"
		if single, err := ctx.FormFile("file"); err == nil && single != nil {
			files = []*multipart.FileHeader{single}
		}
	}

	response, err := h.Service.UploadAttachments(uint(letterID), userID, slices.Contains(claims.Roles, "ADMIN"), files)
	if err != nil {
		errs.HandlerError(ctx, err)
		return
	}

	ctx.JSON(response.StatusCode, response)
}
