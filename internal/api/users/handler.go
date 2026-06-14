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

func (h *Handler) RefreshToken(ctx *gin.Context) {
	var req RefreshTokenRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		errs.HandlerError(ctx, errs.BadRequest("Refresh token tidak valid"))
		return
	}

	response, err := h.Service.RefreshToken(req)
	if err != nil {
		errs.HandlerError(ctx, err)
		return
	}

	ctx.JSON(response.StatusCode, response)
}

func (h *Handler) Logout(ctx *gin.Context) {
	userID, err := middleware.GetUserID(ctx)
	if err != nil {
		errs.HandlerError(ctx, errs.Unauthorized("user tidak terautentikasi"))
		return
	}

	var req LogoutRequest
	var reqPtr *LogoutRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		if !errors.Is(err, io.EOF) {
			errs.HandlerError(ctx, errs.BadRequest("Data logout tidak valid"))
			return
		}
	} else {
		reqPtr = &req
	}

	response, err := h.Service.Logout(userID, reqPtr)
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

func (h *Handler) UpsertFCMToken(ctx *gin.Context) {
	userID, err := middleware.GetUserID(ctx)
	if err != nil {
		errs.HandlerError(ctx, errs.Unauthorized("user tidak terautentikasi"))
		return
	}

	var req UpsertFCMTokenRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		errs.HandlerError(ctx, errs.BadRequest("Data token notifikasi tidak valid"))
		return
	}

	response, err := h.Service.UpsertFCMToken(userID, req)
	if err != nil {
		errs.HandlerError(ctx, err)
		return
	}

	ctx.JSON(response.StatusCode, response)
}

func (h *Handler) DeleteFCMToken(ctx *gin.Context) {
	userID, err := middleware.GetUserID(ctx)
	if err != nil {
		errs.HandlerError(ctx, errs.Unauthorized("user tidak terautentikasi"))
		return
	}

	var req DeleteFCMTokenRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		errs.HandlerError(ctx, errs.BadRequest("Data token notifikasi tidak valid"))
		return
	}

	response, err := h.Service.DeleteFCMToken(userID, req)
	if err != nil {
		errs.HandlerError(ctx, err)
		return
	}

	ctx.JSON(response.StatusCode, response)
}

func (h *Handler) VerifyEmail(ctx *gin.Context) {
	var req VerifyEmailRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		errs.HandlerError(ctx, errs.BadRequest("Email dan kode verifikasi 5 digit wajib diisi dengan benar"))
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

func (h *Handler) CreateStudentInvitation(ctx *gin.Context) {
	adminID, err := middleware.GetUserID(ctx)
	if err != nil {
		errs.HandlerError(ctx, errs.Unauthorized("user tidak terautentikasi"))
		return
	}

	var req CreateStudentInvitationRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		errs.HandlerError(ctx, errs.BadRequest("Data undangan mahasiswa tidak valid"))
		log.Printf("error binding create student invitation payload: %v", err)
		return
	}

	response, err := h.Service.CreateStudentInvitation(adminID, req)
	if err != nil {
		errs.HandlerError(ctx, err)
		return
	}

	ctx.JSON(response.StatusCode, response)
}

func (h *Handler) BulkImportStudentInvitations(ctx *gin.Context) {
	adminID, err := middleware.GetUserID(ctx)
	if err != nil {
		errs.HandlerError(ctx, errs.Unauthorized("user tidak terautentikasi"))
		return
	}

	file, err := ctx.FormFile("file")
	if err != nil {
		errs.HandlerError(ctx, errs.BadRequest("File Excel atau CSV wajib dilampirkan"))
		return
	}

	response, err := h.Service.BulkImportStudentInvitations(adminID, file)
	if err != nil {
		errs.HandlerError(ctx, err)
		return
	}

	ctx.JSON(response.StatusCode, response)
}

func (h *Handler) CompleteStudentInvitation(ctx *gin.Context) {
	var req CompleteStudentInvitationRequest
	if err := ctx.ShouldBind(&req); err != nil {
		errs.HandlerError(ctx, errs.BadRequest("Data aktivasi mahasiswa tidak valid"))
		log.Printf("error binding complete student invitation payload: %v", err)
		return
	}

	file, err := ctx.FormFile("kredensial")
	if err != nil {
		errs.HandlerError(ctx, errs.BadRequest("File kredensial wajib dilampirkan"))
		return
	}

	response, err := h.Service.CompleteStudentInvitation(req, file)
	if err != nil {
		errs.HandlerError(ctx, err)
		return
	}

	ctx.JSON(response.StatusCode, response)
}

func (h *Handler) GetPendingStudents(ctx *gin.Context) {
	var query GetUsersQuery
	if err := ctx.ShouldBindQuery(&query); err != nil {
		// Set default values if binding fails
		query.Page = 1
		query.PageSize = 20
	}

	// Ensure reasonable bounds
	if query.Page < 1 {
		query.Page = 1
	}
	if query.PageSize < 1 {
		query.PageSize = 20
	}
	if query.PageSize > 100 {
		query.PageSize = 100
	}

	response, err := h.Service.GetPendingStudents(query.Page, query.PageSize)
	if err != nil {
		errs.HandlerError(ctx, err)
		return
	}

	ctx.JSON(response.StatusCode, response)
}

func (h *Handler) GetAllUsers(ctx *gin.Context) {
	var query GetUsersQuery
	if err := ctx.ShouldBindQuery(&query); err != nil {
		// Set default values if binding fails
		query.Page = 1
		query.PageSize = 20
	}

	// Ensure reasonable bounds
	if query.Page < 1 {
		query.Page = 1
	}
	if query.PageSize < 1 {
		query.PageSize = 20
	}
	if query.PageSize > 100 {
		query.PageSize = 100
	}

	response, err := h.Service.GetAllUsers(query.Page, query.PageSize)
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

func (h *Handler) CreateStaff(ctx *gin.Context) {
	adminID, err := middleware.GetUserID(ctx)
	if err != nil {
		errs.HandlerError(ctx, errs.Unauthorized("user tidak terautentikasi"))
		return
	}

	var req CreateStaffRequest
	if err := ctx.ShouldBind(&req); err != nil {
		errs.HandlerError(ctx, errs.BadRequest("Data staff tidak valid"))
		log.Printf("error binding create staff payload: %v", err)
		return
	}

	response, err := h.Service.CreateStaff(adminID, req)
	if err != nil {
		errs.HandlerError(ctx, err)
		return
	}

	ctx.JSON(response.StatusCode, response)
}

func (h *Handler) CompleteStaffInvitation(ctx *gin.Context) {
	var req CompleteStaffInvitationRequest
	if err := ctx.ShouldBind(&req); err != nil {
		errs.HandlerError(ctx, errs.BadRequest("Data aktivasi staff tidak valid"))
		log.Printf("error binding complete staff invitation payload: %v", err)
		return
	}

	signatureFile, err := ctx.FormFile("signature")
	if err != nil {
		signatureFile = nil
	}

	response, err := h.Service.CompleteStaffInvitation(req, signatureFile)
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

func (h *Handler) UpdateMyProfile(ctx *gin.Context) {
	userID, err := middleware.GetUserID(ctx)
	if err != nil {
		errs.HandlerError(ctx, errs.Unauthorized("user tidak terautentikasi"))
		return
	}

	var req UpdateMyProfileRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		errs.HandlerError(ctx, errs.BadRequest("Data profil tidak valid"))
		return
	}

	response, err := h.Service.UpdateMyProfile(userID, req)
	if err != nil {
		errs.HandlerError(ctx, err)
		return
	}

	ctx.JSON(response.StatusCode, response)
}
