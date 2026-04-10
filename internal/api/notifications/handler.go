package notifications

import (
	"log"

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

func (h *Handler) SendTest(ctx *gin.Context) {
	userID, err := middleware.GetUserID(ctx)
	if err != nil {
		errs.HandlerError(ctx, errs.Unauthorized("user tidak terautentikasi"))
		return
	}

	var req SendTestNotificationRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		log.Printf("bind send test request failed: %v", err)
		errs.HandlerError(ctx, errs.BadRequest("Data notifikasi tidak valid"))
		return
	}

	resp, err := h.Service.SendTest(userID, req)
	if err != nil {
		errs.HandlerError(ctx, err)
		return
	}

	ctx.JSON(resp.StatusCode, resp)
}
