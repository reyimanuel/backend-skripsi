package user

import (
	"log"

	"github.com/gin-gonic/gin"
	"github.com/reyimanuel/letter-administration/internal/infrastructures/pkg/errs"
)

type Handler struct {
	Service *Service
}

func NewHandler(service *Service) *Handler {
	return &Handler{Service: service}
}

func (h *Handler) Login(ctx *gin.Context) {
	var payload LoginRequest
	if err := ctx.ShouldBindJSON(&payload); err != nil {
		log.Printf("error binding login payload: %v", err)
		errs.HandlerError(ctx, errs.BadRequest("payload tidak valid"))
		return
	}

	response, err := h.Service.Login(&payload)
	if err != nil {
		errs.HandlerError(ctx, err)
		return
	}

	ctx.JSON(response.StatusCode, response)
}
