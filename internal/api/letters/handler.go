package letters

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/reyimanuel/letter-administration/internal/infrastructures/pkg/errs"
)

type Handler struct {
	Service *Service
}

func NewHandler(service *Service) *Handler {
	return &Handler{Service: service}
}

func (h *Handler) Create(ctx *gin.Context) {
	userIDAny, exists := ctx.Get("user_id")
	if !exists {
		errs.HandlerError(ctx, errs.Unauthorized("user tidak terautentikasi"))
		return
	}
	userID := userIDAny.(uint)

	var req CreateLetterRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		errs.HandlerError(ctx, errs.BadRequest("payload tidak valid"))
		return
	}

	response, err := h.Service.Create(userID, req)
	if err != nil {
		errs.HandlerError(ctx, err)
		return
	}

	ctx.JSON(http.StatusOK, response)
}
