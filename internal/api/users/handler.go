package user

import (
	"errors"
	"io"
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

func (h *Handler) Login(ctx *gin.Context) {
	var payload LoginRequest
	if err := ctx.ShouldBindJSON(&payload); err != nil {
		log.Printf("error binding login payload: %v", err)
		errs.HandlerError(ctx, errs.BadRequest("Email dan password wajib diisi"))
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
		errs.HandlerError(ctx, errs.BadRequest("Data pendaftaran tidak valid. Periksa kembali form yang diisi"))
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

func (h *Handler) GetMe(ctx *gin.Context) {
	userID, err := middleware.GetUserID(ctx)
	if err != nil {
		errs.HandlerError(ctx, errs.Unauthorized("user tidak terautentikasi"))
		return
	}

	response, err := h.Service.GetMe(userID)
	if err != nil {
		errs.HandlerError(ctx, err)
		return
	}

	ctx.JSON(response.StatusCode, response)
}

func (h *Handler) VerifyEmail(ctx *gin.Context) {
	var req VerifyEmailRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		errs.HandlerError(ctx, errs.BadRequest("Token verifikasi tidak valid"))
		return
	}

	response, err := h.Service.VerifyEmail(req)
	if err != nil {
		errs.HandlerError(ctx, err)
		return
	}

	ctx.JSON(response.StatusCode, response)
}

func (h *Handler) ResendVerificationEmail(ctx *gin.Context) {
	var req ResendVerificationRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		errs.HandlerError(ctx, errs.BadRequest("Email untuk kirim ulang verifikasi tidak valid"))
		return
	}

	response, err := h.Service.ResendVerificationEmail(req)
	if err != nil {
		errs.HandlerError(ctx, err)
		return
	}

	ctx.JSON(response.StatusCode, response)
}

func (h *Handler) GetPendingStudents(ctx *gin.Context) {
	response, err := h.Service.GetPendingStudents()
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

func (h *Handler) ApproveStudent(ctx *gin.Context) {
	adminID, err := middleware.GetUserID(ctx)
	if err != nil {
		errs.HandlerError(ctx, errs.Unauthorized("user tidak terautentikasi"))
		return
	}

	id, err := strconv.ParseUint(ctx.Param("id"), 10, 64)
	if err != nil {
		errs.HandlerError(ctx, errs.BadRequest("ID tidak valid"))
		return
	}

	var req ApproveStudentRequest
	var reqPtr *ApproveStudentRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		if !errors.Is(err, io.EOF) {
			errs.HandlerError(ctx, errs.BadRequest("Data persetujuan mahasiswa tidak valid"))
			return
		}
	} else {
		reqPtr = &req
	}

	response, err := h.Service.ApproveStudent(uint(id), adminID, reqPtr)
	if err != nil {
		errs.HandlerError(ctx, err)
		return
	}

	ctx.JSON(response.StatusCode, response)
}

func (h *Handler) RejectStudent(ctx *gin.Context) {
	adminID, err := middleware.GetUserID(ctx)
	if err != nil {
		errs.HandlerError(ctx, errs.Unauthorized("user tidak terautentikasi"))
		return
	}

	var req RejectStudentRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		errs.HandlerError(ctx, errs.BadRequest("Alasan penolakan wajib diisi"))
		return
	}

	id, err := strconv.ParseUint(ctx.Param("id"), 10, 64)
	if err != nil {
		errs.HandlerError(ctx, errs.BadRequest("ID tidak valid"))
		return
	}

	response, err := h.Service.RejectStudent(uint(id), adminID, req.Reason)
	if err != nil {
		errs.HandlerError(ctx, err)
		return
	}

	ctx.JSON(response.StatusCode, response)
}

func (h *Handler) CreateOfficial(ctx *gin.Context) {
	adminID, err := middleware.GetUserID(ctx)
	if err != nil {
		errs.HandlerError(ctx, errs.Unauthorized("user tidak terautentikasi"))
		return
	}

	var req CreateOfficialRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		errs.HandlerError(ctx, errs.BadRequest("Data official tidak valid"))
		return
	}

	response, err := h.Service.CreateOfficial(adminID, req)
	if err != nil {
		errs.HandlerError(ctx, err)
		return
	}

	ctx.JSON(response.StatusCode, response)
}

func (h *Handler) AdminUpdateUser(ctx *gin.Context) {
	id, err := strconv.ParseUint(ctx.Param("id"), 10, 64)
	if err != nil {
		errs.HandlerError(ctx, errs.BadRequest("ID tidak valid"))
		return
	}

	var req AdminUpdateUserRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		errs.HandlerError(ctx, errs.BadRequest("Data perubahan user tidak valid"))
		return
	}

	response, err := h.Service.AdminUpdateUser(uint(id), req)
	if err != nil {
		errs.HandlerError(ctx, err)
		return
	}

	ctx.JSON(response.StatusCode, response)
}

func (h *Handler) AdminDeleteUser(ctx *gin.Context) {
	id, err := strconv.ParseUint(ctx.Param("id"), 10, 64)
	if err != nil {
		errs.HandlerError(ctx, errs.BadRequest("ID tidak valid"))
		return
	}

	response, err := h.Service.AdminDeleteUser(uint(id))
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
		errs.HandlerError(ctx, errs.BadRequest("Data pendaftaran KRS tidak valid. Periksa email dan password"))
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
