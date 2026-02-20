package letters

import (
	"fmt"
	"net/http"
	"strconv"

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

	var req UploadTemplateRequest
	if err := ctx.ShouldBind(&req); err != nil {
		fmt.Println("Bind error:", err)
		errs.HandlerError(ctx, errs.BadRequest("permintaan tidak valid"))
		return
	}

	letterTypeIDParam := ctx.Param("id")
	letterTypeID, err := strconv.ParseUint(letterTypeIDParam, 10, 64)
	if err != nil {
		errs.HandlerError(ctx, errs.BadRequest("tipe surat tidak valid"))
		return
	}

	response, err := h.Service.UploadTemplate(userID, uint(letterTypeID), req.File)
	if err != nil {
		errs.HandlerError(ctx, err)
		return
	}

	ctx.JSON(http.StatusOK, response)
}
