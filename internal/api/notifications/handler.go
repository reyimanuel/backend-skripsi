package notifications

import (
	"log"
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

func (h *Handler) GetRecent(ctx *gin.Context) {
	userID, err := middleware.GetUserID(ctx)
	if err != nil {
		errs.HandlerError(ctx, errs.Unauthorized("user tidak terautentikasi"))
		return
	}

	days := 7
	if v := ctx.Query("days"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			days = n
		}
	}
	if days < 1 {
		days = 1
	}
	if days > 30 {
		days = 30
	}

	resp, err := h.Service.ListRecent(userID, days)
	if err != nil {
		errs.HandlerError(ctx, err)
		return
	}

	ctx.JSON(resp.StatusCode, resp)
}

func (h *Handler) MarkRead(ctx *gin.Context) {
	userID, err := middleware.GetUserID(ctx)
	if err != nil {
		errs.HandlerError(ctx, errs.Unauthorized("user tidak terautentikasi"))
		return
	}

	idStr := ctx.Param("id")
	id64, err := strconv.ParseUint(idStr, 10, 64)
	if err != nil || id64 == 0 {
		errs.HandlerError(ctx, errs.BadRequest("ID notifikasi tidak valid"))
		return
	}

	resp, err := h.Service.MarkRead(userID, uint(id64))
	if err != nil {
		errs.HandlerError(ctx, err)
		return
	}

	ctx.JSON(resp.StatusCode, resp)
}
