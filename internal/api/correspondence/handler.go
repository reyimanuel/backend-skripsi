package correspondence

import (
	"fmt"
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
		errs.HandlerError(ctx, errs.BadRequest("payload tidak valid"))
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
		errs.HandlerError(ctx, errs.BadRequest("payload tidak valid"))
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
		errs.HandlerError(ctx, errs.InternalServerError("Gagal membuka file surat"))
		return
	}
	defer file.Close()

	ctx.Header("Content-Type", "application/pdf")
	ctx.Header("Content-Disposition", fmt.Sprintf("inline; filename=%q", fileName))

	http.ServeContent(ctx.Writer, ctx.Request, fileName, time.Now(), file)
}
