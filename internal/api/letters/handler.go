package letters

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/reyimanuel/letter-administration/internal/infrastructures/pkg/errs"
)

type Handler struct {
	Service *Service
}

func NewHandler(service *Service) *Handler {
	return &Handler{Service: service}
}

func (h *Handler) UploadTemplate(ctx *gin.Context) {
	userID := ctx.GetUint("user_id")

	letterTypeID, err := strconv.ParseUint(ctx.PostForm("letter_type_id"), 10, 64)
	if err != nil {
		errs.HandlerError(ctx, errs.BadRequest("letter_type_id tidak valid"))
		return
	}

	file, err := ctx.FormFile("file")
	if err != nil {
		errs.HandlerError(ctx, errs.BadRequest("file wajib diupload"))
		return
	}

	response, err := h.Service.UploadTemplate(userID, uint(letterTypeID), file)
	if err != nil {
		errs.HandlerError(ctx, err)
		return
	}

	ctx.JSON(http.StatusOK, response)
}
