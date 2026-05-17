package user

import (
	"context"
	"errors"
	"fmt"
	"log"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/reyimanuel/letter-administration/internal/constants"
	"github.com/reyimanuel/letter-administration/internal/infrastructures/pkg/errs"
	"github.com/reyimanuel/letter-administration/internal/infrastructures/pkg/helpers"
	"github.com/reyimanuel/letter-administration/internal/infrastructures/pkg/policy"
	"github.com/reyimanuel/letter-administration/internal/infrastructures/pkg/push"
	"github.com/reyimanuel/letter-administration/internal/infrastructures/pkg/token"
	"github.com/reyimanuel/letter-administration/internal/migration"
	"gorm.io/gorm"
)

type Service struct {
	Repo *Repository
}

const emailVerificationCodeTTL = 15 * time.Minute

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

func (s *Service) RegisterStudent(payload *RegisterStudentRequest, file *multipart.FileHeader) (*Response, error) {
	if _, err := s.Repo.GetByNIM(payload.NIM); err == nil {
		return nil, errs.BadRequest("NIM sudah terdaftar")
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		log.Printf("error checking NIM uniqueness: err=%v", err)
		return nil, errs.InternalServerError("Terjadi gangguan pada server. Silakan coba lagi.")
	}

	if _, err := s.Repo.GetByEmail(payload.Email); err == nil {
		return nil, errs.BadRequest("Email sudah terdaftar")
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		log.Printf("error checking email uniqueness: err=%v", err)
		return nil, errs.InternalServerError("Terjadi gangguan pada server. Silakan coba lagi.")
	}

	ext := strings.ToLower(filepath.Ext(file.Filename))
	if ext != ".jpg" && ext != ".jpeg" && ext != ".png" {
		return nil, errs.BadRequest("file harus berupa gambar (jpg/jpeg/png)")
	}

	fileName := helpers.GenerateUniqueFileName(file.Filename)
	filePath := filepath.Join("public", "images", "student-cards", fileName)
	if err := helpers.SaveUploadedFile(file, filePath); err != nil {
		log.Printf("error menyimpan file kredensial: %v", err)
		return nil, errs.InternalServerError("gagal menyimpan file kredensial")
	}

	hashedPwd, err := helpers.HashPassword(payload.Password)
	if err != nil {
		os.Remove(filePath)
		log.Printf("error hashing password: %v", err)
		return nil, errs.InternalServerError("gagal memproses password")
	}

	mahasiswaRole, err := s.Repo.GetRoleByCode(s.Repo.DB, "MAHASISWA")
	if err != nil {
		os.Remove(filePath)
		log.Printf("role MAHASISWA tidak ditemukan: %v", err)
		return nil, errs.InternalServerError("konfigurasi akses tidak ditemukan")
	}

	user := &migration.User{
		Name:     payload.Name,
		Email:    payload.Email,
		Password: hashedPwd,
		Roles:    []migration.Role{*mahasiswaRole},
		IsActive: true,
	}
	student := &migration.Student{
		NIM:                     payload.NIM,
		ProgramStudi:            payload.ProgramStudi,
		Angkatan:                payload.Angkatan,
		KredensialPath:          filePath,
		AdminVerificationStatus: "pending",
	}

	if err := s.Repo.DB.Transaction(func(tx *gorm.DB) error {
		return s.Repo.CreateStudentWithUser(tx, user, student)
	}); err != nil {
		os.Remove(filePath)
		log.Printf("error registering student: %v", err)
		return nil, errs.InternalServerError("gagal mendaftarkan akun mahasiswa")
	}

	if err := s.issueEmailVerificationCode(context.Background(), user); err != nil {
		log.Printf("error sending verification email: %v", err)
	}

	// Best-effort: notify admins about new pending registration.
	ctx, cancel := context.WithTimeout(context.Background(), constants.ExternalServiceTimeout)
	defer cancel()
	if _, err := push.SendToRole(ctx, s.Repo.DB, "ADMIN", "Registrasi Mahasiswa Baru", fmt.Sprintf("%s mendaftar (NIM: %s)", user.Name, payload.NIM), map[string]string{
		"type":            "student_registered",
		"student_user_id": fmt.Sprint(user.ID),
		"nim":             strings.TrimSpace(payload.NIM),
	}); err != nil {
		log.Printf("push admin notify (student_registered) failed: user_id=%d err=%v", user.ID, err)
	}

	return &Response{
		StatusCode: http.StatusCreated,
		Message:    "Pendaftaran berhasil. Silakan verifikasi email lalu tunggu verifikasi admin.",
	}, nil
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
	isOfficial := false
	for _, role := range usr.Roles {
		if strings.EqualFold(role.Code, "MAHASISWA") {
			isStudent = true
			continue
		}
		if role.Code == "DEKAN" || role.Code == "WAKIL_DEKAN" {
			isOfficial = true
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
		resp.KredensialPath = helpers.ToAbsoluteURL(student.KredensialPath)
		resp.AdminVerificationStatus = student.AdminVerificationStatus
		resp.AdminVerifiedAt = student.AdminVerifiedAt
		resp.RejectionReason = student.RejectionReason
	}

	if isOfficial {
		official, err := s.Repo.GetOfficialByUserID(s.Repo.DB, usr.ID)
		if err != nil {
			log.Printf("error getting official profile: user_id=%d err=%v", usr.ID, err)
			return nil, errs.InternalServerError("gagal mengambil data official")
		}
		if official == nil {
			return nil, errs.NotFound("data official tidak ditemukan")
		}

		onDuty := official.IsOnDuty
		resp.OfficialID = &official.ID
		resp.NIP = official.NIP
		resp.Pangkat = official.Pangkat
		resp.Jabatan = official.Jabatan
		resp.Signature = helpers.ToAbsoluteURL(official.Signature)
		resp.IsOnDuty = &onDuty
	}

	return &Response{StatusCode: http.StatusOK, Message: "Profil berhasil diambil", Data: resp}, nil
}

func (s *Service) VerifyEmail(req VerifyEmailRequest) (*Response, error) {
	email := strings.TrimSpace(req.Email)
	code := strings.TrimSpace(req.Code)

	usr, err := s.Repo.GetByEmail(email)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errs.NotFound("User tidak ditemukan")
		}
		log.Printf("error fetching user for email verification: email=%q err=%v", email, err)
		return nil, errs.InternalServerError("Terjadi gangguan pada server. Silakan coba lagi.")
	}
	if usr.EmailVerifiedAt != nil {
		return &Response{StatusCode: http.StatusOK, Message: "Email sudah terverifikasi"}, nil
	}
	if strings.TrimSpace(usr.EmailVerificationCodeHash) == "" || usr.EmailVerificationExpiresAt == nil {
		return nil, errs.BadRequest("Kode verifikasi belum tersedia. Silakan kirim ulang kode.")
	}
	if time.Now().After(*usr.EmailVerificationExpiresAt) {
		return nil, errs.BadRequest("Kode verifikasi sudah kedaluwarsa. Silakan kirim ulang kode.")
	}
	if !helpers.CheckPasswordHash(code, usr.EmailVerificationCodeHash) {
		return nil, errs.BadRequest("Kode verifikasi tidak valid")
	}

	now := time.Now()
	if err := s.Repo.SetUserEmailVerified(s.Repo.DB, usr.ID, now); err != nil {
		log.Printf("error verifying email for user %d: %v", usr.ID, err)
		return nil, errs.InternalServerError("Gagal memverifikasi email")
	}

	return &Response{StatusCode: http.StatusOK, Message: "Email berhasil diverifikasi"}, nil
}

func (s *Service) ResendVerificationEmail(req ResendVerificationRequest) (*Response, error) {
	usr, err := s.Repo.GetByEmail(strings.TrimSpace(req.Email))
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errs.NotFound("User tidak ditemukan")
		}
		log.Printf("error fetching user for resend verification: email=%q err=%v", strings.TrimSpace(req.Email), err)
		return nil, errs.InternalServerError("Terjadi gangguan pada server. Silakan coba lagi.")
	}

	if usr.EmailVerifiedAt != nil {
		return &Response{StatusCode: http.StatusOK, Message: "Email sudah terverifikasi"}, nil
	}

	if err := s.issueEmailVerificationCode(context.Background(), usr); err != nil {
		log.Printf("error resend verification email: %v", err)
		return nil, errs.InternalServerError("Gagal mengirim ulang email verifikasi")
	}

	return &Response{StatusCode: http.StatusOK, Message: "Email verifikasi berhasil dikirim ulang"}, nil
}

func (s *Service) CreateStaff(adminID uint, req CreateStaffRequest, signatureFile *multipart.FileHeader) (*Response, error) {
	roleCode := strings.ToUpper(strings.TrimSpace(req.RoleCode))
	if roleCode == "" {
		return nil, errs.BadRequest("role wajib diisi")
	}

	if _, err := s.Repo.GetByEmail(req.Email); err == nil {
		return nil, errs.BadRequest("Email sudah terdaftar")
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		log.Printf("error checking staff email uniqueness: err=%v", err)
		return nil, errs.InternalServerError("Terjadi gangguan pada server. Silakan coba lagi.")
	}

	hashedPwd, err := helpers.HashPassword(req.Password)
	if err != nil {
		return nil, errs.InternalServerError("gagal memproses password")
	}

	role, err := s.Repo.GetRoleByCode(s.Repo.DB, roleCode)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errs.BadRequest("Role staff tidak ditemukan")
		}
		log.Printf("error fetching staff role: role=%q err=%v", roleCode, err)
		return nil, errs.InternalServerError("Terjadi gangguan pada server. Silakan coba lagi.")
	}

	isOfficialRole := roleCode == "DEKAN" || roleCode == "WAKIL_DEKAN"
	signaturePath := ""
	if isOfficialRole {
		if strings.TrimSpace(req.Jabatan) == "" {
			return nil, errs.BadRequest("jabatan wajib diisi untuk role dekan/wakil dekan")
		}

		if signatureFile == nil {
			return nil, errs.BadRequest("File tanda tangan wajib dilampirkan untuk role dekan/wakil dekan")
		}

		ext := strings.ToLower(filepath.Ext(signatureFile.Filename))
		if ext != ".jpg" && ext != ".jpeg" && ext != ".png" {
			return nil, errs.BadRequest("file tanda tangan harus berupa gambar (jpg/jpeg/png)")
		}

		nip := strings.TrimSpace(req.NIP)
		if len(nip) > 50 {
			return nil, errs.BadRequest("NIP maksimal 50 karakter")
		}
		pangkat := strings.TrimSpace(req.Pangkat)
		if len(pangkat) > 100 {
			return nil, errs.BadRequest("Pangkat maksimal 100 karakter")
		}
		jabatan := strings.TrimSpace(req.Jabatan)
		if len(jabatan) > 100 {
			return nil, errs.BadRequest("Jabatan maksimal 100 karakter")
		}

		fileName := helpers.GenerateUniqueFileName(signatureFile.Filename)
		signaturePath = filepath.Join("public", "images", "signatures", fileName)
		if err := helpers.SaveUploadedFile(signatureFile, signaturePath); err != nil {
			log.Printf("error menyimpan file signature: %v", err)
			return nil, errs.InternalServerError("gagal menyimpan file signature")
		}
	}

	roles := []migration.Role{*role}

	user := &migration.User{
		Name:     req.Name,
		Email:    req.Email,
		Password: hashedPwd,
		Roles:    roles,
		IsActive: true,
	}

	createErr := s.Repo.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(user).Error; err != nil {
			return err
		}

		if !isOfficialRole {
			return nil
		}

		official := &migration.Official{
			UserID:    user.ID,
			NIP:       strings.TrimSpace(req.NIP),
			Pangkat:   strings.TrimSpace(req.Pangkat),
			Jabatan:   strings.TrimSpace(req.Jabatan),
			Signature: signaturePath,
			IsOnDuty:  true,
		}

		return tx.Create(official).Error
	})
	if createErr != nil {
		log.Printf("error creating staff by admin %d: %v", adminID, createErr)
		if signaturePath != "" {
			if err := os.Remove(signaturePath); err != nil && !os.IsNotExist(err) {
				log.Printf("gagal menghapus file signature %s: %v", signaturePath, err)
			}
		}
		return nil, errs.InternalServerError("gagal membuat akun staff")
	}

	if err := s.issueEmailVerificationCode(context.Background(), user); err != nil {
		log.Printf("error sending staff verification email: %v", err)
	}

	return &Response{StatusCode: http.StatusCreated, Message: "Staff berhasil dibuat. Silakan verifikasi email staff."}, nil
}

// RegisterWithKRS registers a new student by extracting their data automatically
// from the uploaded KRS (Kartu Rencana Studi) image using Google Cloud Vision OCR.
// The student only has to supply their email, password, and KRS image.
func (s *Service) RegisterWithKRS(payload *RegisterWithKRSRequest, file *multipart.FileHeader) (*Response, error) {
	// Validate file type
	ext := strings.ToLower(filepath.Ext(file.Filename))
	if ext != ".jpg" && ext != ".jpeg" && ext != ".png" {
		return nil, errs.BadRequest("file KRS harus berupa gambar (jpg/jpeg/png)")
	}

	// Check e-mail uniqueness early so we don't waste OCR quota
	if _, err := s.Repo.GetByEmail(payload.Email); err == nil {
		return nil, errs.BadRequest("Email sudah terdaftar")
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		log.Printf("error checking KRS registration email uniqueness: err=%v", err)
		return nil, errs.InternalServerError("Terjadi gangguan pada server. Silakan coba lagi.")
	}

	// Save the uploaded image
	fileName := helpers.GenerateUniqueFileName(file.Filename)
	filePath := filepath.Join("public", "images", "student-cards", fileName)
	if err := helpers.SaveUploadedFile(file, filePath); err != nil {
		log.Printf("error menyimpan file KRS: %v", err)
		return nil, errs.InternalServerError("gagal menyimpan file KRS")
	}

	// Run OCR via local Tesseract engine
	rawText, err := helpers.ExtractTextFromImage(filePath)
	if err != nil {
		os.Remove(filePath)
		log.Printf("error OCR pada file %s: %v", filePath, err)
		return nil, errs.InternalServerError("gagal memproses gambar KRS dengan OCR")
	}

	// OCR result logged only in debug mode to avoid leaking sensitive data
	if len(rawText) > 0 {
		log.Printf("OCR processing complete, text length: %d", len(rawText))
	}
	// Parse extracted text into structured student data
	krsData, err := helpers.ParseKRSData(rawText)
	if err != nil {
		os.Remove(filePath)
		return nil, errs.BadRequest(err.Error())
	}

	// Check NIM uniqueness
	if _, err := s.Repo.GetByNIM(krsData.NIM); err == nil {
		os.Remove(filePath)
		return nil, errs.BadRequest("NIM sudah terdaftar")
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		os.Remove(filePath)
		log.Printf("error checking KRS registration NIM uniqueness: err=%v", err)
		return nil, errs.InternalServerError("Terjadi gangguan pada server. Silakan coba lagi.")
	}

	// Hash password
	hashedPwd, err := helpers.HashPassword(payload.Password)
	if err != nil {
		os.Remove(filePath)
		log.Printf("error hashing password: %v", err)
		return nil, errs.InternalServerError("gagal memproses password")
	}

	// Fetch MAHASISWA role
	mahasiswaRole, err := s.Repo.GetRoleByCode(s.Repo.DB, "MAHASISWA")
	if err != nil {
		os.Remove(filePath)
		log.Printf("role MAHASISWA tidak ditemukan: %v", err)
		return nil, errs.InternalServerError("konfigurasi akses tidak ditemukan")
	}

	user := &migration.User{
		Name:     krsData.Name,
		Email:    payload.Email,
		Password: hashedPwd,
		Roles:    []migration.Role{*mahasiswaRole},
		IsActive: true,
	}
	student := &migration.Student{
		NIM:                     krsData.NIM,
		ProgramStudi:            krsData.ProgramStudi,
		Angkatan:                krsData.Angkatan,
		KredensialPath:          filePath,
		AdminVerificationStatus: "pending",
	}

	if err := s.Repo.DB.Transaction(func(tx *gorm.DB) error {
		return s.Repo.CreateStudentWithUser(tx, user, student)
	}); err != nil {
		os.Remove(filePath)
		log.Printf("error mendaftarkan mahasiswa via KRS: %v", err)
		return nil, errs.InternalServerError("gagal mendaftarkan akun mahasiswa")
	}

	if err := s.issueEmailVerificationCode(context.Background(), user); err != nil {
		log.Printf("error sending verification email: %v", err)
	}

	// Best-effort: notify admins about new pending registration.
	ctx, cancel := context.WithTimeout(context.Background(), constants.ExternalServiceTimeout)
	defer cancel()
	if _, err := push.SendToRole(ctx, s.Repo.DB, "ADMIN", "Registrasi Mahasiswa Baru", fmt.Sprintf("%s mendaftar (NIM: %s)", user.Name, krsData.NIM), map[string]string{
		"type":            "student_registered",
		"student_user_id": fmt.Sprint(user.ID),
		"nim":             strings.TrimSpace(krsData.NIM),
	}); err != nil {
		log.Printf("push admin notify (student_registered/krs) failed: user_id=%d err=%v", user.ID, err)
	}

	return &Response{
		StatusCode: http.StatusCreated,
		Message:    "Pendaftaran berhasil. Data diekstrak dari KRS. Silakan tunggu verifikasi admin.",
		Data: KRSPreviewResponse{
			UserID:       user.ID,
			Name:         krsData.Name,
			NIM:          krsData.NIM,
			ProgramStudi: krsData.ProgramStudi,
			Angkatan:     krsData.Angkatan,
		},
	}, nil
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
	isOfficial := false
	for _, role := range user.Roles {
		if strings.EqualFold(role.Code, "MAHASISWA") {
			isStudent = true
			continue
		}
		if role.Code == "DEKAN" || role.Code == "WAKIL_DEKAN" {
			isOfficial = true
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

	if isOfficial {
		official, err := s.Repo.GetOfficialByUserID(s.Repo.DB, user.ID)
		if err != nil {
			log.Printf("error fetching official data for login eligibility: user_id=%d err=%v", user.ID, err)
			return errs.InternalServerError("Gagal memvalidasi status official")
		}
		if err := policy.CanOfficialAct(user, official); err != nil {
			return errs.Unauthorized(err.Error())
		}
	}

	return nil
}

func (s *Service) issueEmailVerificationCode(ctx context.Context, usr *migration.User) error {
	code, err := helpers.GenerateEmailVerificationCode()
	if err != nil {
		return err
	}

	codeHash, err := helpers.HashPassword(code)
	if err != nil {
		return err
	}

	if err := s.Repo.SetUserEmailVerificationCode(s.Repo.DB, usr.ID, codeHash, time.Now().Add(emailVerificationCodeTTL)); err != nil {
		return err
	}

	return helpers.SendVerificationEmailWithContext(ctx, usr.Email, usr.Name, code)
}
