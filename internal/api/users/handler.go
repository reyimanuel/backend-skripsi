package user

import (
	"log"
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

func (h *Handler) Login(ctx *gin.Context) {
	var payload LoginRequest
	if err := ctx.ShouldBindJSON(&payload); err != nil {
		log.Printf("error binding login payload: %v", err)
		errs.HandlerError(ctx, errs.BadRequest("Payload tidak valid"))
		return
	}

	response, err := h.Service.Login(&payload)
	if err != nil {
		errs.HandlerError(ctx, err)
		return
	}

	ctx.JSON(response.StatusCode, response)
}

func (h *Handler) RegisterStudent(ctx *gin.Context) {
	var payload RegisterStudentRequest
	if err := ctx.ShouldBind(&payload); err != nil {
		log.Printf("error binding register payload: %v", err)
		errs.HandlerError(ctx, errs.BadRequest("Payload tidak valid"))
		return
	}

	file, err := ctx.FormFile("kredensial")
	if err != nil {
		errs.HandlerError(ctx, errs.BadRequest("File kredensial wajib dilampirkan"))
		return
	}

	response, err := h.Service.RegisterStudent(&payload, file)
	if err != nil {
		errs.HandlerError(ctx, err)
		return
	}

	ctx.JSON(response.StatusCode, response)
}

func (h *Handler) GetPendingUsers(ctx *gin.Context) {
	response, err := h.Service.GetPendingUsers()
	if err != nil {
		errs.HandlerError(ctx, err)
		return
	}

	ctx.JSON(response.StatusCode, response)
}

func (h *Handler) GetAllUsers(ctx *gin.Context) {
	response, err := h.Service.GetAllUsers()
	if err != nil {
		errs.HandlerError(ctx, err)
		return
	}

	ctx.JSON(response.StatusCode, response)
}

func (h *Handler) ApproveUser(ctx *gin.Context) {
	id, err := strconv.ParseUint(ctx.Param("id"), 10, 64)
	if err != nil {
		errs.HandlerError(ctx, errs.BadRequest("ID tidak valid"))
		return
	}

	response, err := h.Service.ApproveUser(uint(id))
	if err != nil {
		errs.HandlerError(ctx, err)
		return
	}

	ctx.JSON(response.StatusCode, response)
}

func (h *Handler) RejectUser(ctx *gin.Context) {
	id, err := strconv.ParseUint(ctx.Param("id"), 10, 64)
	if err != nil {
		errs.HandlerError(ctx, errs.BadRequest("ID tidak valid"))
		return
	}

	response, err := h.Service.RejectUser(uint(id))
	if err != nil {
		errs.HandlerError(ctx, err)
		return
	}

	ctx.JSON(response.StatusCode, response)
}

func (h *Handler) RegisterWithKRS(ctx *gin.Context) {
	var payload RegisterWithKRSRequest
	if err := ctx.ShouldBind(&payload); err != nil {
		log.Printf("error binding krs registration payload: %v", err)
		errs.HandlerError(ctx, errs.BadRequest("Payload tidak valid"))
		return
	}

	file, err := ctx.FormFile("krs")
	if err != nil {
		errs.HandlerError(ctx, errs.BadRequest("File KRS wajib dilampirkan"))
		return
	}

	response, err := h.Service.RegisterWithKRS(&payload, file)
	if err != nil {
		errs.HandlerError(ctx, err)
		return
	}

	ctx.JSON(response.StatusCode, response)
}
