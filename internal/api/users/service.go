package user

import (
	"context"
	"errors"
	"log"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/reyimanuel/letter-administration/internal/constants"
	config "github.com/reyimanuel/letter-administration/internal/infrastructures/config"
	"github.com/reyimanuel/letter-administration/internal/infrastructures/pkg/errs"
	"github.com/reyimanuel/letter-administration/internal/infrastructures/pkg/helpers"
	"github.com/reyimanuel/letter-administration/internal/infrastructures/pkg/policy"
	"github.com/reyimanuel/letter-administration/internal/infrastructures/pkg/token"
	"github.com/reyimanuel/letter-administration/internal/migration"
	"github.com/reyimanuel/letter-administration/internal/realtime"
	"gorm.io/gorm"
)

type Service struct {
	Repo *Repository
}

var (
	staffNamePattern = regexp.MustCompile(`^[\p{L}\p{M}0-9 .,'-]{3,100}$`)
	staffNIPPattern  = regexp.MustCompile(`^[0-9]{8,30}$`)
)

func NewService(repo *Repository) *Service {
	return &Service{
		Repo: repo,
	}
}

func (s *Service) Login(payload *LoginRequest) (*Response, error) {
	user, err := s.Repo.GetByEmail(strings.TrimSpace(payload.Email))
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errs.Unauthorized("Email atau password salah")
		}
		log.Printf("error fetching user by email during login: err=%v", err)
		return nil, errs.InternalServerError("Terjadi gangguan pada server. Silakan coba lagi.")
	}

	if !helpers.CheckPasswordHash(payload.Password, user.Password) {
		return nil, errs.Unauthorized("Email atau password salah")
	}

	if err := s.ensureLoginEligibility(user); err != nil {
		return nil, err
	}

	access, err := token.GenerateToken(user.ID, user.Email, user.RoleSlice())
	if err != nil {
		log.Printf("error saat membuat token: %v", err)
		return nil, errs.InternalServerError("Gagal membuat akses token")
	}

	refresh, err := token.GenerateRefreshToken(user.ID)
	if err != nil {
		log.Printf("error saat membuat refresh token: %v", err)
		return nil, errs.InternalServerError("Gagal membuat refresh token")
	}

	return &Response{
		StatusCode: http.StatusOK,
		Message:    "Login Berhasil",
		Data: TokenResponse{
			AccessToken:  access,
			RefreshToken: refresh,
		},
	}, nil
}

func (s *Service) RefreshToken(req RefreshTokenRequest) (*Response, error) {
	refreshToken := strings.TrimSpace(req.RefreshToken)
	userID, err := token.ValidateRefreshToken(refreshToken)
	if err != nil {
		return nil, errs.Unauthorized("Refresh token tidak valid")
	}

	user, err := s.Repo.GetUserByID(s.Repo.DB, userID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errs.Unauthorized("Refresh token tidak valid")
		}
		log.Printf("error fetching user by id during refresh: err=%v", err)
		return nil, errs.InternalServerError("Terjadi gangguan pada server. Silakan coba lagi.")
	}

	if err := s.ensureLoginEligibility(user); err != nil {
		return nil, err
	}

	access, err := token.GenerateToken(user.ID, user.Email, user.RoleSlice())
	if err != nil {
		log.Printf("error saat membuat access token pada refresh: %v", err)
		return nil, errs.InternalServerError("Gagal membuat akses token")
	}

	refresh, err := token.GenerateRefreshToken(user.ID)
	if err != nil {
		log.Printf("error saat membuat refresh token baru: %v", err)
		return nil, errs.InternalServerError("Gagal membuat refresh token")
	}

	return &Response{
		StatusCode: http.StatusOK,
		Message:    "Token berhasil diperbarui",
		Data: TokenResponse{
			AccessToken:  access,
			RefreshToken: refresh,
		},
	}, nil
}

func (s *Service) Logout(userID uint, req *LogoutRequest) (*Response, error) {
	if req != nil {
		refreshToken := strings.TrimSpace(req.RefreshToken)
		if refreshToken != "" {
			tokenUserID, err := token.ValidateRefreshToken(refreshToken)
			if err != nil {
				return nil, errs.Unauthorized("Refresh token tidak valid")
			}
			if tokenUserID != userID {
				return nil, errs.Forbidden("Refresh token bukan milik user ini")
			}
		}
	}

	return &Response{StatusCode: http.StatusOK, Message: "Logout berhasil"}, nil
}

func (s *Service) GetPendingStudents(page, pageSize int) (*Response, error) {
	// Validate pagination parameters
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100 // Max limit
	}

	students, total, err := s.Repo.GetPendingStudents(s.Repo.DB, page, pageSize)
	if err != nil {
		log.Printf("error mengambil pending students: %v", err)
		return nil, errs.InternalServerError("gagal mengambil data mahasiswa pending")
	}

	result := make([]PendingStudentResponse, 0, len(students))
	for _, student := range students {
		item := PendingStudentResponse{
			StudentID:               student.ID,
			UserID:                  student.UserID,
			Name:                    student.User.Name,
			Email:                   student.User.Email,
			NIM:                     student.NIM,
			ProgramStudi:            student.ProgramStudi,
			Angkatan:                student.Angkatan,
			SemesterMasukKuliah:     student.SemesterMasukKuliah,
			Kredensial:              helpers.ToAbsoluteURL(student.KredensialPath),
			AdminVerificationStatus: student.AdminVerificationStatus,
			RejectionReason:         student.RejectionReason,
			EmailVerifiedAt:         student.User.EmailVerifiedAt,
			CreatedAt:               student.CreatedAt,
		}
		item.AdminVerifiedAt = student.AdminVerifiedAt

		result = append(result, item)
	}

	return &Response{
		StatusCode: http.StatusOK,
		Message:    "Data mahasiswa pending berhasil diambil",
		Data: PendingStudentListData{
			Items: result,
			Meta: PaginationMeta{
				Page:     page,
				PageSize: pageSize,
				Total:    total,
			},
		},
	}, nil
}

func (s *Service) GetAllUsers(page, pageSize int) (*Response, error) {
	// Validate pagination parameters
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100 // Max limit
	}

	users, total, err := s.Repo.GetAllUsers(s.Repo.DB, page, pageSize)
	if err != nil {
		log.Printf("error mengambil semua users: %v", err)
		return nil, errs.InternalServerError("gagal mengambil data users")
	}

	result := make([]UserListResponse, 0, len(users))
	for _, usr := range users {
		item := UserListResponse{
			UserID:          usr.ID,
			Name:            usr.Name,
			Email:           usr.Email,
			Roles:           usr.RoleSlice(),
			IsActive:        usr.IsActive,
			EmailVerifiedAt: usr.EmailVerifiedAt,
			CreatedAt:       usr.CreatedAt,
		}

		if usr.Student != nil {
			item.StudentID = &usr.Student.ID
			item.AdminVerificationStatus = usr.Student.AdminVerificationStatus
			item.AdminVerifiedAt = usr.Student.AdminVerifiedAt
			item.RejectionReason = usr.Student.RejectionReason
		}

		result = append(result, item)
	}

	return &Response{
		StatusCode: http.StatusOK,
		Message:    "Data users berhasil diambil",
		Data: UserListData{
			Items: result,
			Meta: PaginationMeta{
				Page:     page,
				PageSize: pageSize,
				Total:    total,
			},
		},
	}, nil
}

func (s *Service) AdminUpdateUser(userID uint, req AdminUpdateUserRequest) (*Response, error) {
	usr, err := s.Repo.GetUserByID(s.Repo.DB, userID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errs.NotFound("User tidak ditemukan")
		}
		log.Printf("error fetching user by id for update: user_id=%d err=%v", userID, err)
		return nil, errs.InternalServerError("Terjadi gangguan pada server. Silakan coba lagi.")
	}

	// Check if updating roles would reduce admin count to zero
	if req.IsActive != nil && !*req.IsActive {
		// If trying to deactivate user, check if they're an admin and if this would be the last admin
		isAdmin := false
		for _, role := range usr.Roles {
			if role.Code == "ADMIN" {
				isAdmin = true
				break
			}
		}

		if isAdmin {
			adminCount, err := s.Repo.CountAdminsWithRole("ADMIN")
			if err != nil {
				log.Printf("error counting admins: %v", err)
				return nil, errs.InternalServerError("Terjadi gangguan pada server. Silakan coba lagi.")
			}

			if adminCount <= 1 {
				return nil, errs.Forbidden("Tidak dapat tidak-mengaktifkan admin terakhir")
			}
		}
	}

	updates := map[string]any{}

	if req.Name != nil {
		name := strings.TrimSpace(*req.Name)
		if name == "" {
			return nil, errs.BadRequest("nama tidak boleh kosong")
		}
		updates["name"] = name
	}

	if req.Email != nil {
		email := strings.TrimSpace(*req.Email)
		if email == "" {
			return nil, errs.BadRequest("email tidak boleh kosong")
		}

		if !strings.EqualFold(strings.TrimSpace(usr.Email), email) {
			if existing, err := s.Repo.GetByEmail(email); err == nil {
				if existing.ID != usr.ID {
					return nil, errs.BadRequest("email sudah terdaftar")
				}
			} else if !errors.Is(err, gorm.ErrRecordNotFound) {
				log.Printf("error checking email uniqueness: %v", err)
				return nil, errs.InternalServerError("gagal memproses update user")
			}
		}

		updates["email"] = email
	}

	if req.IsActive != nil {
		updates["is_active"] = *req.IsActive
	}

	if req.ProfilePhoto != nil {
		updates["profile_photo"] = strings.TrimSpace(*req.ProfilePhoto)
	}

	if len(updates) == 0 {
		return nil, errs.BadRequest("tidak ada data yang diupdate")
	}

	if err := s.Repo.UpdateUserFields(s.Repo.DB, userID, updates); err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return nil, errs.BadRequest("email sudah terdaftar")
		}
		log.Printf("error updating user %d: %v", userID, err)
		return nil, errs.InternalServerError("gagal memperbarui user")
	}

	realtime.PublishTopics([]string{"users", "me"}, "user-updated", userID)

	return &Response{StatusCode: http.StatusOK, Message: "User berhasil diperbarui"}, nil
}

func (s *Service) AdminDeleteUser(userID uint) (*Response, error) {
	// Check if user exists
	if _, err := s.Repo.GetUserByID(s.Repo.DB, userID); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errs.NotFound("User tidak ditemukan")
		}
		log.Printf("error fetching user by id for delete: user_id=%d err=%v", userID, err)
		return nil, errs.InternalServerError("Terjadi gangguan pada server. Silakan coba lagi.")
	}

	// Check if this is the last admin user
	adminCount, err := s.Repo.CountAdminsWithRole("ADMIN")
	if err != nil {
		log.Printf("error counting admins: %v", err)
		return nil, errs.InternalServerError("Terjadi gangguan pada server. Silakan coba lagi.")
	}

	// Check if the user being deleted is an admin
	user, err := s.Repo.GetUserByID(s.Repo.DB, userID)
	if err != nil {
		log.Printf("error fetching user by id: %v", err)
		return nil, errs.InternalServerError("Terjadi gangguan pada server. Silakan coba lagi.")
	}

	isAdmin := false
	for _, role := range user.Roles {
		if role.Code == "ADMIN" {
			isAdmin = true
			break
		}
	}

	// If user is admin and this would be the last admin, prevent deletion
	if isAdmin && adminCount <= 1 {
		return nil, errs.Forbidden("Tidak dapat menghapus admin terakhir")
	}

	if err := s.Repo.DeleteUser(s.Repo.DB, userID); err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23503" {
			return nil, errs.BadRequest("user tidak dapat dihapus karena masih memiliki data terkait")
		}
		log.Printf("error deleting user: %v", err)
		return nil, errs.InternalServerError("gagal menghapus user")
	}

	realtime.Publish("users", "user-deleted", userID)

	return &Response{StatusCode: http.StatusOK, Message: "User berhasil dihapus"}, nil
}

func (s *Service) ApproveStudent(studentID uint, adminID uint, req *ApproveStudentRequest) (*Response, error) {
	var kredensialPath string

	log.Printf("approving student: student_id=%d admin_id=%d payload_present=%t", studentID, adminID, req != nil)
	err := s.Repo.DB.Transaction(func(tx *gorm.DB) error {
		student, err := s.resolvePendingStudentTx(tx, studentID)
		if err != nil {
			return err
		}
		kredensialPath = student.KredensialPath

		userUpdates := map[string]any{}
		studentUpdates := map[string]any{}

		if req != nil {
			if name := strings.TrimSpace(req.Name); name != "" {
				userUpdates["name"] = name
			}
			if nim := strings.TrimSpace(req.NIM); nim != "" {
				studentUpdates["nim"] = nim
			}
			if ps := strings.TrimSpace(req.ProgramStudi); ps != "" {
				studentUpdates["program_studi"] = ps
			}
			if req.Angkatan != nil {
				studentUpdates["angkatan"] = *req.Angkatan
			}
			if rawSemester := strings.TrimSpace(req.SemesterMasukKuliah); rawSemester != "" {
				semester := normalizeSemesterMasukKuliah(rawSemester)
				if semester == "" {
					return errs.BadRequest("semester masuk kuliah harus Ganjil atau Genap")
				}
				studentUpdates["semester_masuk_kuliah"] = semester
			}
		}

		if err := s.Repo.UpdateUserFields(tx, student.UserID, userUpdates); err != nil {
			log.Printf("error updating user fields for student %d: %v", studentID, err)
			return errs.InternalServerError("gagal memperbarui data mahasiswa")
		}

		if err := s.Repo.UpdateStudentFields(tx, student.ID, studentUpdates); err != nil {
			var pgErr *pgconn.PgError
			if errors.As(err, &pgErr) && pgErr.Code == "23505" {
				return errs.BadRequest("NIM sudah terdaftar")
			}
			log.Printf("error updating student fields %d: %v", studentID, err)
			return errs.InternalServerError("gagal memperbarui data mahasiswa")
		}

		if err := s.Repo.UpdateStudentAdminVerification(tx, student.ID, "approved", &adminID, ""); err != nil {
			log.Printf("error approve student %d: %v", studentID, err)
			return errs.InternalServerError("gagal menyetujui mahasiswa")
		}

		if kredensialPath != "" {
			if err := s.Repo.ClearStudentKredensial(tx, student.ID); err != nil {
				log.Printf("erroxr membersihkan path kredensial mahasiswa %d: %v", studentID, err)
				return errs.InternalServerError("gagal memproses persetujuan mahasiswa")
			}
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	if kredensialPath != "" {
		if err := os.Remove(kredensialPath); err != nil && !os.IsNotExist(err) {
			log.Printf("gagal menghapus file kredensial %s: %v", kredensialPath, err)
		}
	}

	realtime.PublishTopics([]string{"users", "pending-students"}, "student-approved", studentID)

	return &Response{StatusCode: http.StatusOK, Message: "Mahasiswa berhasil disetujui oleh admin"}, nil
}

func (s *Service) RejectStudent(studentID uint, adminID uint, reason string) (*Response, error) {
	student, err := s.resolvePendingStudent(studentID)
	if err != nil {
		return nil, err
	}

	if err := s.Repo.UpdateStudentAdminVerification(s.Repo.DB, student.ID, "rejected", &adminID, strings.TrimSpace(reason)); err != nil {
		log.Printf("error reject student %d: %v", studentID, err)
		return nil, errs.InternalServerError("gagal menolak mahasiswa")
	}

	if student.KredensialPath != "" {
		if err := os.Remove(student.KredensialPath); err != nil && !os.IsNotExist(err) {
			log.Printf("gagal menghapus file kredensial %s: %v", student.KredensialPath, err)
			return nil, errs.InternalServerError("gagal menghapus file kredensial mahasiswa")
		}
	}

	realtime.PublishTopics([]string{"users", "pending-students"}, "student-rejected", studentID)

	return &Response{
		StatusCode: http.StatusOK,
		Message:    "Mahasiswa berhasil ditolak oleh admin",
	}, nil
}

func (s *Service) GetMe(userID uint) (*Response, error) {
	usr, err := s.Repo.GetUserByID(s.Repo.DB, userID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errs.NotFound("User tidak ditemukan")
		}
		log.Printf("error fetching profile user: user_id=%d err=%v", userID, err)
		return nil, errs.InternalServerError("Terjadi gangguan pada server. Silakan coba lagi.")
	}

	isStudent := false
	isAtasan := false
	for _, role := range usr.Roles {
		if strings.EqualFold(role.Code, "MAHASISWA") {
			isStudent = true
			continue
		}
		if constants.IsAtasanRoleCode(role.Code) {
			isAtasan = true
		}
	}

	resp := MeResponse{
		UserID:          usr.ID,
		Name:            usr.Name,
		Email:           usr.Email,
		Roles:           usr.RoleSlice(),
		IsActive:        usr.IsActive,
		EmailVerifiedAt: usr.EmailVerifiedAt,
		CreatedAt:       usr.CreatedAt,
	}
	if usr.ProfilePhoto != nil {
		pp := helpers.ToAbsoluteURL(*usr.ProfilePhoto)
		if pp != "" {
			resp.ProfilePhoto = &pp
		}
	}

	if isStudent {
		student := usr.Student
		if student == nil {
			student, err = s.Repo.GetStudentByUserID(s.Repo.DB, usr.ID)
			if err != nil {
				if errors.Is(err, gorm.ErrRecordNotFound) {
					return nil, errs.NotFound("Data mahasiswa tidak ditemukan")
				}
				log.Printf("error fetching student profile: user_id=%d err=%v", usr.ID, err)
				return nil, errs.InternalServerError("Terjadi gangguan pada server. Silakan coba lagi.")
			}
		}

		resp.StudentID = &student.ID
		resp.NIM = student.NIM
		resp.ProgramStudi = student.ProgramStudi
		resp.Angkatan = student.Angkatan
		resp.SemesterMasukKuliah = student.SemesterMasukKuliah
		resp.KredensialPath = helpers.ToAbsoluteURL(student.KredensialPath)
		resp.AdminVerificationStatus = student.AdminVerificationStatus
		resp.AdminVerifiedAt = student.AdminVerifiedAt
		resp.RejectionReason = student.RejectionReason
	}

	if isAtasan {
		atasan, err := s.Repo.GetAtasanByUserID(s.Repo.DB, usr.ID)
		if err != nil {
			log.Printf("error getting atasan profile: user_id=%d err=%v", usr.ID, err)
			return nil, errs.InternalServerError("gagal mengambil data atasan")
		}
		if atasan == nil {
			return nil, errs.NotFound("data atasan tidak ditemukan")
		}

		onDuty := atasan.IsOnDuty
		resp.AtasanID = &atasan.ID
		resp.NIP = atasan.NIP
		resp.Pangkat = atasan.Pangkat
		resp.Jabatan = atasan.Jabatan
		resp.Signature = helpers.ToAbsoluteURL(atasan.Signature)
		resp.IsOnDuty = &onDuty
	}

	return &Response{StatusCode: http.StatusOK, Message: "Profil berhasil diambil", Data: resp}, nil
}

func (s *Service) CreateStudentInvitation(adminID uint, req CreateStudentInvitationRequest) (*Response, error) {
	input := studentInvitationInput{
		Name:                req.Name,
		NIM:                 req.NIM,
		Email:               req.Email,
		ProgramStudi:        req.ProgramStudi,
		Angkatan:            req.Angkatan,
		SemesterMasukKuliah: req.SemesterMasukKuliah,
	}
	if err := s.createStudentInvitationRecord(adminID, input); err != nil {
		return nil, err
	}

	return &Response{StatusCode: http.StatusCreated, Message: "Undangan aktivasi mahasiswa berhasil dikirim ke email pengguna."}, nil
}

type studentInvitationInput struct {
	Name                string
	NIM                 string
	Email               string
	ProgramStudi        string
	Angkatan            int
	SemesterMasukKuliah string
}

func (s *Service) createStudentInvitationRecord(adminID uint, input studentInvitationInput) error {
	name := strings.TrimSpace(input.Name)
	email := strings.TrimSpace(input.Email)
	nim := strings.TrimSpace(input.NIM)
	semesterMasukKuliah := normalizeSemesterMasukKuliah(input.SemesterMasukKuliah)
	if name == "" {
		return errs.BadRequest("nama mahasiswa wajib diisi")
	}
	if email == "" {
		return errs.BadRequest("email mahasiswa wajib diisi")
	}
	if nim == "" {
		return errs.BadRequest("NIM mahasiswa wajib diisi")
	}
	if strings.TrimSpace(input.SemesterMasukKuliah) != "" && semesterMasukKuliah == "" {
		return errs.BadRequest("semester masuk kuliah harus Ganjil atau Genap")
	}

	programStudi := strings.TrimSpace(input.ProgramStudi)
	if programStudi == "" {
		return errs.BadRequest("program studi wajib diisi")
	}
	if input.Angkatan <= 0 {
		return errs.BadRequest("angkatan wajib diisi")
	}

	if _, err := s.Repo.GetByNIM(nim); err == nil {
		return errs.BadRequest("NIM sudah terdaftar")
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		log.Printf("error checking invited student NIM uniqueness: err=%v", err)
		return errs.InternalServerError("Terjadi gangguan pada server. Silakan coba lagi.")
	}

	if _, err := s.Repo.GetByEmail(email); err == nil {
		return errs.BadRequest("Email sudah terdaftar")
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		log.Printf("error checking invited student email uniqueness: err=%v", err)
		return errs.InternalServerError("Terjadi gangguan pada server. Silakan coba lagi.")
	}

	placeholderPassword := "pending-student-invitation:" + uuid.NewString()
	hashedPwd, err := helpers.HashPassword(placeholderPassword)
	if err != nil {
		return errs.InternalServerError("gagal memproses password")
	}

	mahasiswaRole, err := s.Repo.GetRoleByCode(s.Repo.DB, "MAHASISWA")
	if err != nil {
		log.Printf("role MAHASISWA tidak ditemukan: %v", err)
		return errs.InternalServerError("konfigurasi akses tidak ditemukan")
	}

	user := &migration.User{
		Name:     name,
		Email:    email,
		Password: hashedPwd,
		Roles:    []migration.Role{*mahasiswaRole},
		IsActive: true,
	}
	student := &migration.Student{
		NIM:                     nim,
		ProgramStudi:            programStudi,
		Angkatan:                input.Angkatan,
		SemesterMasukKuliah:     semesterMasukKuliah,
		AdminVerificationStatus: "invited",
	}

	if err := s.Repo.DB.Transaction(func(tx *gorm.DB) error {
		return s.Repo.CreateStudentWithUser(tx, user, student)
	}); err != nil {
		log.Printf("error creating student invitation by admin %d: %v", adminID, err)
		return errs.InternalServerError("gagal membuat undangan mahasiswa")
	}

	realtime.Publish("users", "student-invitation-created", user.ID)

	invitationToken, err := token.GenerateStudentInvitationToken(user.ID, user.Email, nim)
	if err != nil {
		log.Printf("error generating student invitation token: user_id=%d err=%v", user.ID, err)
		return errs.InternalServerError("gagal membuat link aktivasi mahasiswa")
	}

	inviterName := "Admin"
	if admin, err := s.Repo.GetUserByID(s.Repo.DB, adminID); err == nil && strings.TrimSpace(admin.Name) != "" {
		inviterName = strings.TrimSpace(admin.Name)
	}

	invitationLink := buildStudentInvitationLink(invitationToken)
	if err := helpers.SendStudentInvitationEmailWithContext(context.Background(), user.Email, user.Name, nim, inviterName, invitationLink); err != nil {
		log.Printf("error sending student invitation email: user_id=%d err=%v", user.ID, err)
		return invitationEmailError()
	}

	return nil
}

func (s *Service) CompleteStudentInvitation(req CompleteStudentInvitationRequest) (*Response, error) {
	invitation, err := token.ValidateStudentInvitationToken(strings.TrimSpace(req.Token))
	if err != nil {
		return nil, errs.Unauthorized("Link aktivasi mahasiswa tidak valid atau sudah kedaluwarsa")
	}

	usr, err := s.Repo.GetUserByID(s.Repo.DB, invitation.UserID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errs.NotFound("User mahasiswa tidak ditemukan")
		}
		log.Printf("error fetching invited student: user_id=%d err=%v", invitation.UserID, err)
		return nil, errs.InternalServerError("Terjadi gangguan pada server. Silakan coba lagi.")
	}
	if !strings.EqualFold(strings.TrimSpace(usr.Email), strings.TrimSpace(invitation.Email)) {
		return nil, errs.Unauthorized("Link aktivasi mahasiswa tidak sesuai dengan akun")
	}
	if usr.EmailVerifiedAt != nil {
		return nil, errs.BadRequest("Undangan mahasiswa sudah digunakan")
	}
	if !userHasRole(usr, "MAHASISWA") {
		return nil, errs.BadRequest("Role mahasiswa pada undangan tidak sesuai")
	}

	student, err := s.Repo.GetStudentByUserID(s.Repo.DB, usr.ID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errs.NotFound("Data mahasiswa tidak ditemukan")
		}
		log.Printf("error fetching invited student profile: user_id=%d err=%v", usr.ID, err)
		return nil, errs.InternalServerError("Terjadi gangguan pada server. Silakan coba lagi.")
	}
	if strings.TrimSpace(student.NIM) != strings.TrimSpace(invitation.NIM) {
		return nil, errs.Unauthorized("Link aktivasi mahasiswa tidak sesuai dengan NIM")
	}
	if strings.ToLower(strings.TrimSpace(student.AdminVerificationStatus)) != "invited" {
		return nil, errs.BadRequest("Undangan mahasiswa sudah diproses")
	}

	hashedPwd, err := helpers.HashPassword(req.Password)
	if err != nil {
		log.Printf("error hashing student invitation password: user_id=%d err=%v", usr.ID, err)
		return nil, errs.InternalServerError("gagal memproses password")
	}

	now := time.Now()
	err = s.Repo.DB.Transaction(func(tx *gorm.DB) error {
		if err := s.Repo.UpdateUserFields(tx, usr.ID, map[string]any{
			"password":          hashedPwd,
			"email_verified_at": now,
		}); err != nil {
			log.Printf("error activating invited student user: user_id=%d err=%v", usr.ID, err)
			return errs.InternalServerError("gagal mengaktifkan akun mahasiswa")
		}

		if err := s.Repo.UpdateStudentFields(tx, student.ID, map[string]any{
			"admin_verification_status": "approved",
			"admin_verified_at":         now,
			"rejection_reason":          "",
		}); err != nil {
			log.Printf("error completing invited student: student_id=%d err=%v", student.ID, err)
			return errs.InternalServerError("gagal melengkapi data mahasiswa")
		}

		return nil
	})
	if err != nil {
		return nil, err
	}

	realtime.PublishTopics([]string{"users", "pending-students"}, "student-invitation-completed", usr.ID)

	return &Response{StatusCode: http.StatusOK, Message: "Akun mahasiswa berhasil diaktifkan. Silakan login."}, nil
}

func (s *Service) CreateStaff(adminID uint, req CreateStaffRequest) (*Response, error) {
	roleCode := strings.ToUpper(strings.TrimSpace(req.RoleCode))
	if roleCode == "" {
		return nil, errs.BadRequest("role wajib diisi")
	}
	if roleCode != "ADMIN" && roleCode != "ATASAN" {
		return nil, errs.BadRequest("role staff tidak valid")
	}

	name := strings.TrimSpace(req.Name)
	email := strings.ToLower(strings.TrimSpace(req.Email))
	if err := validateStaffName(name, "nama staff"); err != nil {
		return nil, err
	}
	if email == "" {
		return nil, errs.BadRequest("email staff wajib diisi")
	}
	if len(email) > 254 {
		return nil, errs.BadRequest("email staff maksimal 254 karakter")
	}
	jabatan := strings.TrimSpace(req.Jabatan)
	if roleCode == "ATASAN" && jabatan == "" {
		return nil, errs.BadRequest("jabatan wajib diisi untuk role atasan")
	}
	if roleCode == "ATASAN" && jabatan != "" {
		normalizedJabatan, ok := normalizeAtasanJabatan(jabatan)
		if !ok {
			return nil, errs.BadRequest("jabatan harus salah satu dari: Dekan, Wakil Dekan 1, Wakil Dekan 2, Wakil Dekan 3, Koprodi, Kabag, Kajur")
		}
		jabatan = normalizedJabatan
	} else if jabatan != "" {
		if err := validateStaffName(jabatan, "jabatan"); err != nil {
			return nil, err
		}
	}

	if _, err := s.Repo.GetByEmail(email); err == nil {
		return nil, errs.BadRequest("Email sudah terdaftar")
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		log.Printf("error checking staff email uniqueness: err=%v", err)
		return nil, errs.InternalServerError("Terjadi gangguan pada server. Silakan coba lagi.")
	}

	placeholderPassword := "pending-staff-invitation:" + uuid.NewString()
	hashedPwd, err := helpers.HashPassword(placeholderPassword)
	if err != nil {
		return nil, errs.InternalServerError("gagal memproses password")
	}

	role, err := s.Repo.GetRoleByCode(s.Repo.DB, roleCode)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			role = &migration.Role{Code: roleCode, Name: defaultStaffRoleName(roleCode)}
			if createRoleErr := s.Repo.DB.Create(role).Error; createRoleErr != nil {
				log.Printf("error creating missing staff role: role=%q err=%v", roleCode, createRoleErr)
				return nil, errs.InternalServerError("Gagal menyiapkan role staff")
			}
		} else {
			log.Printf("error fetching staff role: role=%q err=%v", roleCode, err)
			return nil, errs.InternalServerError("Terjadi gangguan pada server. Silakan coba lagi.")
		}
	}

	isAtasanRole := constants.IsAtasanRoleCode(roleCode)

	roles := []migration.Role{*role}

	user := &migration.User{
		Name:     name,
		Email:    email,
		Password: hashedPwd,
		Roles:    roles,
		IsActive: true,
	}

	createErr := s.Repo.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(user).Error; err != nil {
			return err
		}

		if !isAtasanRole {
			return nil
		}

		atasan := &migration.Atasan{
			UserID:   user.ID,
			Jabatan:  jabatan,
			IsOnDuty: true,
		}

		return tx.Create(atasan).Error
	})
	if createErr != nil {
		log.Printf("error creating staff by admin %d: %v", adminID, createErr)
		return nil, errs.InternalServerError("gagal membuat akun staff")
	}

	realtime.Publish("users", "staff-invitation-created", user.ID)

	invitationToken, err := token.GenerateStaffInvitationToken(user.ID, user.Email, roleCode)
	if err != nil {
		log.Printf("error generating staff invitation token: user_id=%d err=%v", user.ID, err)
		return nil, errs.InternalServerError("gagal membuat link aktivasi staff")
	}

	inviterName := "Admin"
	if admin, err := s.Repo.GetUserByID(s.Repo.DB, adminID); err == nil && strings.TrimSpace(admin.Name) != "" {
		inviterName = strings.TrimSpace(admin.Name)
	}

	invitationLink := buildStaffInvitationLink(invitationToken)
	if err := helpers.SendStaffInvitationEmailWithContext(context.Background(), user.Email, user.Name, inviterName, invitationLink); err != nil {
		log.Printf("error sending staff invitation email: user_id=%d err=%v", user.ID, err)
		return nil, invitationEmailError()
	}

	return &Response{StatusCode: http.StatusCreated, Message: "Undangan aktivasi staff berhasil dikirim ke email pengguna."}, nil
}

func (s *Service) ResendInvitation(adminID uint, userID uint) (*Response, error) {
	usr, err := s.Repo.GetUserByID(s.Repo.DB, userID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errs.NotFound("User tidak ditemukan")
		}
		log.Printf("error fetching user for resend invitation: user_id=%d err=%v", userID, err)
		return nil, errs.InternalServerError("Terjadi gangguan pada server. Silakan coba lagi.")
	}
	if usr.EmailVerifiedAt != nil {
		return nil, errs.BadRequest("Akun sudah terverifikasi, link aktivasi tidak perlu dikirim ulang")
	}

	inviterName := "Admin"
	if admin, err := s.Repo.GetUserByID(s.Repo.DB, adminID); err == nil && strings.TrimSpace(admin.Name) != "" {
		inviterName = strings.TrimSpace(admin.Name)
	}

	roles := usr.RoleSlice()
	if userHasRole(usr, "MAHASISWA") {
		student, err := s.Repo.GetStudentByUserID(s.Repo.DB, usr.ID)
		if err != nil {
			return nil, errs.BadRequest("Data mahasiswa undangan tidak ditemukan")
		}
		tokenValue, err := token.GenerateStudentInvitationToken(usr.ID, usr.Email, student.NIM)
		if err != nil {
			log.Printf("error generating student invitation token: user_id=%d err=%v", usr.ID, err)
			return nil, errs.InternalServerError("gagal membuat link aktivasi mahasiswa")
		}
		link := buildStudentInvitationLink(tokenValue)
		if err := helpers.SendStudentInvitationEmailWithContext(context.Background(), usr.Email, usr.Name, student.NIM, inviterName, link); err != nil {
			log.Printf("error resending student invitation email: user_id=%d err=%v", usr.ID, err)
			return nil, invitationEmailError()
		}
		realtime.Publish("users", "student-invitation-resent", usr.ID)
		return &Response{StatusCode: http.StatusOK, Message: "Link aktivasi mahasiswa berhasil dikirim ulang"}, nil
	}

	roleCode := ""
	for _, role := range roles {
		normalized := strings.ToUpper(strings.TrimSpace(role))
		if normalized == "ADMIN" || constants.IsAtasanRoleCode(normalized) {
			roleCode = normalized
			break
		}
	}
	if roleCode == "" {
		return nil, errs.BadRequest("User tidak memiliki role yang dapat dikirim link aktivasi")
	}

	tokenValue, err := token.GenerateStaffInvitationToken(usr.ID, usr.Email, roleCode)
	if err != nil {
		log.Printf("error generating staff invitation token: user_id=%d err=%v", usr.ID, err)
		return nil, errs.InternalServerError("gagal membuat link aktivasi staff")
	}
	link := buildStaffInvitationLink(tokenValue)
	if err := helpers.SendStaffInvitationEmailWithContext(context.Background(), usr.Email, usr.Name, inviterName, link); err != nil {
		log.Printf("error resending staff invitation email: user_id=%d err=%v", usr.ID, err)
		return nil, invitationEmailError()
	}

	realtime.Publish("users", "staff-invitation-resent", usr.ID)

	return &Response{StatusCode: http.StatusOK, Message: "Link aktivasi staff berhasil dikirim ulang"}, nil
}

func validateStaffName(value string, label string) error {
	if value == "" {
		return errs.BadRequest(label + " wajib diisi")
	}
	if !staffNamePattern.MatchString(value) {
		return errs.BadRequest(label + " harus 3-100 karakter dan hanya boleh berisi huruf, angka, spasi, titik, koma, apostrof, atau tanda hubung")
	}
	if !helpers.IsSafeHTML(value) {
		return errs.BadRequest(label + " mengandung karakter tidak aman")
	}
	return nil
}

func invitationEmailError() error {
	return errs.BadRequest("email gagal dikirim. Silakan coba kirim lagi.")
}

func (s *Service) CompleteStaffInvitation(req CompleteStaffInvitationRequest, signatureFile *multipart.FileHeader) (*Response, error) {
	invitation, err := token.ValidateStaffInvitationToken(strings.TrimSpace(req.Token))
	if err != nil {
		return nil, errs.Unauthorized("Link aktivasi staff tidak valid atau sudah kedaluwarsa")
	}

	usr, err := s.Repo.GetUserByID(s.Repo.DB, invitation.UserID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errs.NotFound("User staff tidak ditemukan")
		}
		log.Printf("error fetching invited staff: user_id=%d err=%v", invitation.UserID, err)
		return nil, errs.InternalServerError("Terjadi gangguan pada server. Silakan coba lagi.")
	}
	if !strings.EqualFold(strings.TrimSpace(usr.Email), strings.TrimSpace(invitation.Email)) {
		return nil, errs.Unauthorized("Link aktivasi staff tidak sesuai dengan akun")
	}
	if usr.EmailVerifiedAt != nil {
		return nil, errs.BadRequest("Undangan staff sudah digunakan")
	}
	if !userHasRole(usr, invitation.RoleCode) {
		return nil, errs.BadRequest("Role staff pada undangan tidak sesuai")
	}

	passwordHash, err := helpers.HashPassword(req.Password)
	if err != nil {
		log.Printf("error hashing staff invitation password: user_id=%d err=%v", usr.ID, err)
		return nil, errs.InternalServerError("gagal memproses password")
	}

	roleCode := strings.ToUpper(strings.TrimSpace(invitation.RoleCode))
	isAtasanRole := constants.IsAtasanRoleCode(roleCode)
	signaturePath := ""
	if isAtasanRole {
		nip := strings.TrimSpace(req.NIP)
		if nip == "" {
			return nil, errs.BadRequest("NIP wajib diisi")
		}
		if !staffNIPPattern.MatchString(nip) {
			return nil, errs.BadRequest("NIP harus 8-30 digit angka")
		}
		if strings.TrimSpace(req.Pangkat) != "" {
			if err := validateStaffName(strings.TrimSpace(req.Pangkat), "pangkat"); err != nil {
				return nil, err
			}
		}
		if strings.TrimSpace(req.Jabatan) != "" {
			if err := validateStaffName(strings.TrimSpace(req.Jabatan), "jabatan"); err != nil {
				return nil, err
			}
		}
		if signatureFile == nil {
			return nil, errs.BadRequest("File tanda tangan wajib dilampirkan")
		}
	}

	if signatureFile != nil {
		ext := strings.ToLower(filepath.Ext(signatureFile.Filename))
		if ext != ".jpg" && ext != ".jpeg" && ext != ".png" {
			return nil, errs.BadRequest("file tanda tangan harus berupa gambar (jpg/jpeg/png)")
		}

		fileName := helpers.GenerateUniqueFileName(signatureFile.Filename)
		signaturePath = filepath.Join("public", "images", "signatures", fileName)
		if err := helpers.SaveUploadedFile(signatureFile, signaturePath); err != nil {
			log.Printf("error menyimpan file signature staff: %v", err)
			return nil, errs.InternalServerError("gagal menyimpan file tanda tangan")
		}
	}

	nip := strings.TrimSpace(req.NIP)
	pangkat := strings.TrimSpace(req.Pangkat)
	jabatan := strings.TrimSpace(req.Jabatan)
	if isAtasanRole && jabatan != "" {
		normalizedJabatan, ok := normalizeAtasanJabatan(jabatan)
		if !ok {
			if signaturePath != "" {
				_ = os.Remove(signaturePath)
			}
			return nil, errs.BadRequest("jabatan harus salah satu dari: Dekan, Wakil Dekan 1, Wakil Dekan 2, Wakil Dekan 3, Koprodi, Kabag, Kajur")
		}
		jabatan = normalizedJabatan
	}
	if isAtasanRole && jabatan == "" {
		if existingAtasan, err := s.Repo.GetAtasanByUserID(s.Repo.DB, usr.ID); err == nil && existingAtasan != nil {
			jabatan = strings.TrimSpace(existingAtasan.Jabatan)
		}
		if jabatan == "" {
			jabatan = defaultStaffJabatan(roleCode)
		}
	}
	if len(nip) > 50 {
		if signaturePath != "" {
			_ = os.Remove(signaturePath)
		}
		return nil, errs.BadRequest("NIP maksimal 50 karakter")
	}
	if len(pangkat) > 100 {
		if signaturePath != "" {
			_ = os.Remove(signaturePath)
		}
		return nil, errs.BadRequest("Pangkat maksimal 100 karakter")
	}
	if len(jabatan) > 100 {
		if signaturePath != "" {
			_ = os.Remove(signaturePath)
		}
		return nil, errs.BadRequest("Jabatan maksimal 100 karakter")
	}

	now := time.Now()
	err = s.Repo.DB.Transaction(func(tx *gorm.DB) error {
		if err := s.Repo.UpdateUserFields(tx, usr.ID, map[string]any{
			"password":          passwordHash,
			"email_verified_at": now,
		}); err != nil {
			log.Printf("error activating staff user: user_id=%d err=%v", usr.ID, err)
			return errs.InternalServerError("gagal mengaktifkan akun staff")
		}

		if !isAtasanRole && signaturePath == "" && nip == "" && pangkat == "" && jabatan == "" {
			return nil
		}

		atasan, err := s.Repo.GetAtasanByUserID(tx, usr.ID)
		if err != nil {
			log.Printf("error fetching atasan during staff activation: user_id=%d err=%v", usr.ID, err)
			return errs.InternalServerError("gagal memproses data jabatan staff")
		}

		updates := map[string]any{
			"nip":       nip,
			"pangkat":   pangkat,
			"jabatan":   jabatan,
			"is_active": true,
		}
		if signaturePath != "" {
			updates["signature"] = signaturePath
		}

		if atasan != nil {
			if err := tx.Model(&migration.Atasan{}).Where("id = ?", atasan.ID).Updates(updates).Error; err != nil {
				log.Printf("error updating atasan during staff activation: user_id=%d err=%v", usr.ID, err)
				return errs.InternalServerError("gagal memperbarui data jabatan staff")
			}
			return nil
		}

		atasan = &migration.Atasan{
			UserID:    usr.ID,
			NIP:       nip,
			Pangkat:   pangkat,
			Jabatan:   jabatan,
			Signature: signaturePath,
			IsOnDuty:  true,
		}
		if err := tx.Create(atasan).Error; err != nil {
			log.Printf("error creating atasan during staff activation: user_id=%d err=%v", usr.ID, err)
			return errs.InternalServerError("gagal menyimpan data jabatan staff")
		}
		return nil
	})
	if err != nil {
		if signaturePath != "" {
			if removeErr := os.Remove(signaturePath); removeErr != nil && !os.IsNotExist(removeErr) {
				log.Printf("gagal menghapus file signature %s: %v", signaturePath, removeErr)
			}
		}
		return nil, err
	}

	realtime.PublishTopics([]string{"users", "me"}, "staff-invitation-completed", usr.ID)

	return &Response{StatusCode: http.StatusOK, Message: "Akun staff berhasil diaktifkan. Silakan login."}, nil
}

// UpdateMyProfile allows a user to update their own profile (name and profile_photo).
func (s *Service) UpdateMyProfile(userID uint, req UpdateMyProfileRequest) (*Response, error) {
	_, err := s.Repo.GetUserByID(s.Repo.DB, userID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errs.NotFound("User tidak ditemukan")
		}
		log.Printf("error fetching user by id for profile update: user_id=%d err=%v", userID, err)
		return nil, errs.InternalServerError("Terjadi gangguan pada server. Silakan coba lagi.")
	}

	updates := map[string]any{}

	if req.Name != nil {
		name := strings.TrimSpace(*req.Name)
		if name == "" {
			return nil, errs.BadRequest("nama tidak boleh kosong")
		}
		updates["name"] = name
	}

	if req.ProfilePhoto != nil {
		updates["profile_photo"] = strings.TrimSpace(*req.ProfilePhoto)
	}

	if len(updates) == 0 {
		return nil, errs.BadRequest("tidak ada data yang diupdate")
	}

	if err := s.Repo.UpdateUserFields(s.Repo.DB, userID, updates); err != nil {
		log.Printf("error updating user profile: user_id=%d err=%v", userID, err)
		return nil, errs.InternalServerError("Gagal memperbarui profil")
	}

	realtime.Publish("me", "profile-updated", userID)

	return &Response{StatusCode: http.StatusOK, Message: "Profil berhasil diperbarui"}, nil
}

func (s *Service) UpsertFCMToken(userID uint, req UpsertFCMTokenRequest) (*Response, error) {
	tokenStr := strings.TrimSpace(req.Token)
	if tokenStr == "" {
		return nil, errs.BadRequest("token wajib diisi")
	}

	platform := strings.TrimSpace(req.Platform)
	if platform == "" {
		platform = "web"
	}

	now := time.Now()
	if err := s.Repo.UpsertUserDeviceToken(s.Repo.DB, userID, tokenStr, platform, now); err != nil {
		log.Printf("error upserting fcm token: user_id=%d platform=%q err=%v", userID, platform, err)
		return nil, errs.InternalServerError("gagal menyimpan token notifikasi")
	}

	return &Response{StatusCode: http.StatusOK, Message: "Token notifikasi berhasil disimpan"}, nil
}

func (s *Service) DeleteFCMToken(userID uint, req DeleteFCMTokenRequest) (*Response, error) {
	tokenStr := strings.TrimSpace(req.Token)
	if tokenStr == "" {
		return nil, errs.BadRequest("token wajib diisi")
	}

	if err := s.Repo.DeleteUserDeviceToken(s.Repo.DB, userID, tokenStr); err != nil {
		log.Printf("error deleting fcm token: user_id=%d err=%v", userID, err)
		return nil, errs.InternalServerError("gagal menghapus token notifikasi")
	}

	return &Response{StatusCode: http.StatusOK, Message: "Token notifikasi berhasil dihapus"}, nil
}

// helper functions

func (s *Service) resolvePendingStudentTx(tx *gorm.DB, studentID uint) (*migration.Student, error) {
	student, err := s.Repo.GetStudentByID(tx, studentID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errs.NotFound("Mahasiswa tidak ditemukan")
		}
		log.Printf("error fetching student by id: student_id=%d err=%v", studentID, err)
		return nil, errs.InternalServerError("Terjadi gangguan pada server. Silakan coba lagi.")
	}

	if strings.ToLower(strings.TrimSpace(student.AdminVerificationStatus)) != "pending" {
		return nil, errs.BadRequest("mahasiswa tidak dalam status pending")
	}

	return student, nil
}

func (s *Service) resolvePendingStudent(studentID uint) (*migration.Student, error) {
	return s.resolvePendingStudentTx(s.Repo.DB, studentID)
}

func defaultStaffJabatan(roleCode string) string {
	switch strings.ToUpper(strings.TrimSpace(roleCode)) {
	case "ATASAN":
		return "Dekan"
	case "KOPRODI":
		return "Koprodi"
	case "KABAG":
		return "Kabag"
	case "KAJUR":
		return "Kajur"
	case "ADMIN":
		return "Admin Fakultas"
	default:
		return "Staff"
	}
}

func normalizeAtasanJabatan(value string) (string, bool) {
	normalized := strings.ToUpper(strings.Join(strings.Fields(value), " "))
	normalized = strings.ReplaceAll(normalized, ".", "")
	switch normalized {
	case "DEKAN":
		return "Dekan", true
	case "WAKIL DEKAN 1":
		return "Wakil Dekan 1", true
	case "WAKIL DEKAN 2":
		return "Wakil Dekan 2", true
	case "WAKIL DEKAN 3":
		return "Wakil Dekan 3", true
	case "KOPRODI":
		return "Koprodi", true
	case "KABAG":
		return "Kabag", true
	case "KAJUR":
		return "Kajur", true
	default:
		return "", false
	}
}

func defaultStaffRoleName(roleCode string) string {
	switch strings.ToUpper(strings.TrimSpace(roleCode)) {
	case "ADMIN":
		return "Administrator"
	case "ATASAN":
		return "Atasan"
	case "KOPRODI":
		return "Koordinator Program Studi"
	case "KABAG":
		return "Kepala Bagian"
	case "KAJUR":
		return "Ketua Jurusan"
	default:
		return defaultStaffJabatan(roleCode)
	}
}

func userHasRole(usr *migration.User, roleCode string) bool {
	if usr == nil {
		return false
	}
	for _, role := range usr.Roles {
		if strings.EqualFold(role.Code, roleCode) {
			return true
		}
	}
	return false
}

func buildStaffInvitationLink(invitationToken string) string {
	frontEndURL := "http://localhost:3000"
	if cfg := config.Get(); cfg != nil && strings.TrimSpace(cfg.FrontEndURL) != "" {
		frontEndURL = strings.TrimRight(strings.TrimSpace(cfg.FrontEndURL), "/")
	}

	u, err := url.Parse(frontEndURL + "/staff/complete")
	if err != nil {
		return frontEndURL + "/staff/complete?token=" + url.QueryEscape(invitationToken)
	}

	query := u.Query()
	query.Set("token", invitationToken)
	u.RawQuery = query.Encode()
	return u.String()
}

func buildStudentInvitationLink(invitationToken string) string {
	frontEndURL := "http://localhost:3000"
	if cfg := config.Get(); cfg != nil && strings.TrimSpace(cfg.FrontEndURL) != "" {
		frontEndURL = strings.TrimRight(strings.TrimSpace(cfg.FrontEndURL), "/")
	}

	u, err := url.Parse(frontEndURL + "/student-invitation/complete")
	if err != nil {
		return frontEndURL + "/student-invitation/complete?token=" + url.QueryEscape(invitationToken)
	}

	query := u.Query()
	query.Set("token", invitationToken)
	u.RawQuery = query.Encode()
	return u.String()
}

func normalizeSemesterMasukKuliah(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "ganjil":
		return "Ganjil"
	case "genap":
		return "Genap"
	default:
		return ""
	}
}

func (s *Service) ensureLoginEligibility(user *migration.User) error {
	if user == nil {
		return errs.Unauthorized("Akun tidak valid")
	}

	if !user.IsActive {
		return errs.Unauthorized("Akun Anda tidak aktif")
	}

	if user.EmailVerifiedAt == nil {
		return errs.Unauthorized("Email belum diverifikasi. Silakan cek inbox email Anda.")
	}

	isStudent := false
	isAtasan := false
	for _, role := range user.Roles {
		if strings.EqualFold(role.Code, "MAHASISWA") {
			isStudent = true
			continue
		}
		if constants.IsAtasanRoleCode(role.Code) {
			isAtasan = true
		}
	}

	if isStudent {
		if user.Student == nil {
			student, err := s.Repo.GetStudentByUserID(s.Repo.DB, user.ID)
			if err != nil {
				if errors.Is(err, gorm.ErrRecordNotFound) {
					return errs.Unauthorized("Data mahasiswa tidak ditemukan")
				}
				log.Printf("error fetching student data for login eligibility: user_id=%d err=%v", user.ID, err)
				return errs.InternalServerError("Terjadi gangguan pada server. Silakan coba lagi.")
			}
			user.Student = student
		}

		if strings.ToLower(strings.TrimSpace(user.Student.AdminVerificationStatus)) != "approved" {
			return errs.Unauthorized("Akun mahasiswa belum disetujui admin")
		}
	}

	if isAtasan {
		atasan, err := s.Repo.GetAtasanByUserID(s.Repo.DB, user.ID)
		if err != nil {
			log.Printf("error fetching atasan data for login eligibility: user_id=%d err=%v", user.ID, err)
			return errs.InternalServerError("Gagal memvalidasi status atasan")
		}
		if err := policy.CanAtasanAct(user, atasan); err != nil {
			return errs.Unauthorized(err.Error())
		}
	}

	return nil
}
