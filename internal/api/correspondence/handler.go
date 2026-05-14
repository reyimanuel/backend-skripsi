package correspondence

import (
	"fmt"
	"log"
	"mime/multipart"
	"net/http"
	"os"
	"slices"
	"strconv"
	"strings"
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

func (h *Handler) CreateDraftLetter(ctx *gin.Context) {
	userID, err := middleware.GetUserID(ctx)
	if err != nil {
		errs.HandlerError(ctx, errs.Unauthorized("user tidak terautentikasi"))
		return
	}

	var req CreateDraftRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		errs.HandlerError(ctx, errs.BadRequest("Data draft surat tidak valid"))
		return
	}

	response, err := h.Service.CreateDraftLetter(userID, req)
	if err != nil {
		errs.HandlerError(ctx, err)
		return
	}

	ctx.JSON(response.StatusCode, response)
}

func (h *Handler) UpdateDraftLetter(ctx *gin.Context) {
	userID, err := middleware.GetUserID(ctx)
	if err != nil {
		errs.HandlerError(ctx, errs.Unauthorized("user tidak terautentikasi"))
		return
	}

	letterIDParam := ctx.Param("id")
	letterID, err := strconv.Atoi(letterIDParam)
	if err != nil {
		errs.HandlerError(ctx, errs.BadRequest("surat tidak valid"))
		return
	}

	var req UpdateDraftRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		errs.HandlerError(ctx, errs.BadRequest("Data update draft tidak valid"))
		return
	}

	response, err := h.Service.UpdateDraftLetter(uint(letterID), userID, req)
	if err != nil {
		errs.HandlerError(ctx, err)
		return
	}

	ctx.JSON(response.StatusCode, response)
}

func (h *Handler) SubmitDraftLetter(ctx *gin.Context) {
	userID, err := middleware.GetUserID(ctx)
	if err != nil {
		errs.HandlerError(ctx, errs.Unauthorized("user tidak terautentikasi"))
		return
	}

	letterIDParam := ctx.Param("id")
	letterID, err := strconv.Atoi(letterIDParam)
	if err != nil {
		errs.HandlerError(ctx, errs.BadRequest("surat tidak valid"))
		return
	}

	response, err := h.Service.SubmitDraftLetter(uint(letterID), userID)
	if err != nil {
		errs.HandlerError(ctx, err)
		return
	}

	ctx.JSON(response.StatusCode, response)
}

func (h *Handler) ListForwardedLetters(ctx *gin.Context) {
	userID, err := middleware.GetUserID(ctx)
	if err != nil {
		errs.HandlerError(ctx, errs.Unauthorized("user tidak terautentikasi"))
		return
	}

	var q ListLettersQuery
	if err := ctx.ShouldBindQuery(&q); err != nil {
		errs.HandlerError(ctx, errs.BadRequest("Parameter pencarian surat tidak valid"))
		return
	}

	response, err := h.Service.ListForwardedLetters(userID, q)
	if err != nil {
		errs.HandlerError(ctx, err)
		return
	}

	ctx.JSON(response.StatusCode, response)
}

func (h *Handler) ReviewLetter(ctx *gin.Context) {
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

	response, err := h.Service.ReviewLetter(uint(letterID), userID, req)

	if err != nil {
		errs.HandlerError(ctx, err)
		return
	}

	ctx.JSON(response.StatusCode, response)
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

	isAdmin := slices.Contains(claims.Roles, "ADMIN")
	isOfficial := slices.Contains(claims.Roles, "DEKAN") || slices.Contains(claims.Roles, "WAKIL_DEKAN")
	pdfPath, fileName, err := h.Service.PreviewLetter(uint(letterID), userID, isAdmin, isOfficial)
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

func (h *Handler) GetHistoryAndDetail(ctx *gin.Context) {
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

	isAdmin := slices.Contains(claims.Roles, "ADMIN")
	isOfficial := slices.Contains(claims.Roles, "DEKAN") || slices.Contains(claims.Roles, "WAKIL_DEKAN")
	response, err := h.Service.GetHistoryAndDetail(uint(letterID), userID, isAdmin, isOfficial)
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

	// attachment key(s)
	defaultKey := strings.TrimSpace(ctx.PostForm("key"))
	keys := form.Value["keys"]
	if len(keys) == 0 {
		keys = form.Value["keys[]"]
	}

	response, err := h.Service.UploadAttachments(uint(letterID), userID, slices.Contains(claims.Roles, "ADMIN"), files, keys, defaultKey)
	if err != nil {
		errs.HandlerError(ctx, err)
		return
	}

	ctx.JSON(response.StatusCode, response)
}
