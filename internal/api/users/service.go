package user

import (
	"log"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/reyimanuel/letter-administration/internal/infrastructures/pkg/errs"
	"github.com/reyimanuel/letter-administration/internal/infrastructures/pkg/helpers"
	"github.com/reyimanuel/letter-administration/internal/infrastructures/pkg/policy"
	"github.com/reyimanuel/letter-administration/internal/infrastructures/pkg/token"
	"github.com/reyimanuel/letter-administration/internal/migration"
	"gorm.io/gorm"
)

type Service struct {
	Repo *Repository
}

func NewService(repo *Repository) *Service {
	return &Service{
		Repo: repo,
	}
}

func (s *Service) Login(payload *LoginRequest) (*Response, error) {
	user, err := s.Repo.GetByEmail(strings.TrimSpace(payload.Email))
	if err != nil {
		return nil, errs.Unauthorized("Email atau Password Salah")
	}

	if !helpers.CheckPasswordHash(payload.Password, user.Password) {
		return nil, errs.Unauthorized("Email atau Password Salah")
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

func (s *Service) RegisterStudent(payload *RegisterStudentRequest, file *multipart.FileHeader) (*Response, error) {
	if _, err := s.Repo.GetByNIM(payload.NIM); err == nil {
		return nil, errs.BadRequest("NIM sudah terdaftar")
	}

	if _, err := s.Repo.GetByEmail(payload.Email); err == nil {
		return nil, errs.BadRequest("Email sudah terdaftar")
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
		log.Printf("error mendaftarkan mahasiswa: %v", err)
		return nil, errs.InternalServerError("gagal mendaftarkan akun mahasiswa")
	}

	if err := helpers.SendVerificationEmail(user.ID, user.Email, user.Name); err != nil {
		log.Printf("error sending verification email: %v", err)
	}

	return &Response{
		StatusCode: http.StatusCreated,
		Message:    "Pendaftaran berhasil. Silakan verifikasi email lalu tunggu verifikasi admin.",
	}, nil
}

func (s *Service) GetPendingStudents() (*Response, error) {
	students, err := s.Repo.GetPendingStudents(s.Repo.DB)
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
			Kredensial:              student.KredensialPath,
			AdminVerificationStatus: student.AdminVerificationStatus,
			RejectionReason:         student.RejectionReason,
			EmailVerifiedAt:         derefTime(student.User.EmailVerifiedAt),
			CreatedAt:               student.CreatedAt,
		}
		if student.AdminVerifiedAt != nil {
			item.AdminVerifiedAt = *student.AdminVerifiedAt
		}

		result = append(result, item)
	}

	return &Response{
		StatusCode: http.StatusOK,
		Message:    "Data mahasiswa pending berhasil diambil",
		Data:       result,
	}, nil
}

func (s *Service) GetAllUsers() (*Response, error) {
	users, err := s.Repo.GetAllUsers(s.Repo.DB)
	if err != nil {
		log.Printf("error mengambil semua users: %v", err)
		return nil, errs.InternalServerError("gagal mengambil data users")
	}

	result := make([]UserListResponse, 0, len(users))
	for _, usr := range users {
		result = append(result, UserListResponse{
			UserID:                  usr.ID,
			Name:                    usr.Name,
			Email:                   usr.Email,
			Roles:                   usr.RoleSlice(),
			IsActive:                usr.IsActive,
			EmailVerifiedAt:         usr.EmailVerifiedAt,
			AdminVerificationStatus: studentStatus(usr.Student),
			AdminVerifiedAt:         studentVerifiedAt(usr.Student),
			RejectionReason:         studentRejection(usr.Student),
			CreatedAt:               usr.CreatedAt,
		})
		if usr.Student != nil {
			result[len(result)-1].StudentID = &usr.Student.ID
		}
	}

	return &Response{
		StatusCode: http.StatusOK,
		Message:    "Data users berhasil diambil",
		Data:       result,
	}, nil
}

func (s *Service) ApproveStudent(studentID uint, adminID uint) (*Response, error) {
	student, err := s.resolvePendingStudent(studentID)
	if err != nil {
		return nil, err
	}

	if err := s.Repo.UpdateStudentAdminVerification(s.Repo.DB, student.ID, "approved", &adminID, ""); err != nil {
		log.Printf("error approve student %d: %v", studentID, err)
		return nil, errs.InternalServerError("gagal menyetujui mahasiswa")
	}

	if student.KredensialPath != "" {
		if err := os.Remove(student.KredensialPath); err != nil && !os.IsNotExist(err) {
			log.Printf("gagal menghapus file kredensial %s: %v", student.KredensialPath, err)
		}
		if err := s.Repo.ClearStudentKredensial(s.Repo.DB, student.ID); err != nil {
			log.Printf("error membersihkan path kredensial mahasiswa %d: %v", studentID, err)
		}
	}

	return &Response{
		StatusCode: http.StatusOK,
		Message:    "Mahasiswa berhasil disetujui oleh admin",
	}, nil
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
		return nil, errs.NotFound("user tidak ditemukan")
	}

	resp := MeResponse{
		UserID:          usr.ID,
		Name:            usr.Name,
		Email:           usr.Email,
		Roles:           usr.RoleSlice(),
		IsActive:        usr.IsActive,
		EmailVerifiedAt: usr.EmailVerifiedAt,
	}

	if usr.Student != nil {
		resp.StudentID = &usr.Student.ID
		resp.NIM = usr.Student.NIM
		resp.ProgramStudi = usr.Student.ProgramStudi
		resp.AdminVerificationStatus = usr.Student.AdminVerificationStatus
		resp.AdminVerifiedAt = usr.Student.AdminVerifiedAt
		resp.RejectionReason = usr.Student.RejectionReason
	}

	return &Response{StatusCode: http.StatusOK, Message: "Profil berhasil diambil", Data: resp}, nil
}

func (s *Service) VerifyEmail(req VerifyEmailRequest) (*Response, error) {
	userID, email, err := token.ValidateEmailVerificationToken(req.Token)
	if err != nil {
		return nil, errs.BadRequest("Token verifikasi tidak valid")
	}

	usr, err := s.Repo.GetUserByID(s.Repo.DB, userID)
	if err != nil {
		return nil, errs.NotFound("user tidak ditemukan")
	}
	if !strings.EqualFold(strings.TrimSpace(usr.Email), strings.TrimSpace(email)) {
		return nil, errs.BadRequest("Token verifikasi tidak sesuai")
	}
	if usr.EmailVerifiedAt != nil {
		return &Response{StatusCode: http.StatusOK, Message: "Email sudah terverifikasi"}, nil
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
		return nil, errs.NotFound("user tidak ditemukan")
	}

	if usr.EmailVerifiedAt != nil {
		return &Response{StatusCode: http.StatusOK, Message: "Email sudah terverifikasi"}, nil
	}

	if err := helpers.SendVerificationEmail(usr.ID, usr.Email, usr.Name); err != nil {
		log.Printf("error resend verification email: %v", err)
		return nil, errs.InternalServerError("Gagal mengirim ulang email verifikasi")
	}

	return &Response{StatusCode: http.StatusOK, Message: "Email verifikasi berhasil dikirim ulang"}, nil
}

func (s *Service) CreateOfficial(adminID uint, req CreateOfficialRequest) (*Response, error) {
	if _, err := s.Repo.GetByEmail(req.Email); err == nil {
		return nil, errs.BadRequest("Email sudah terdaftar")
	}

	hashedPwd, err := helpers.HashPassword(req.Password)
	if err != nil {
		return nil, errs.InternalServerError("gagal memproses password")
	}

	role, err := s.Repo.GetRoleByCode(s.Repo.DB, req.RoleCode)
	if err != nil {
		return nil, errs.BadRequest("Role official tidak ditemukan")
	}

	user := &migration.User{
		Name:     req.Name,
		Email:    req.Email,
		Password: hashedPwd,
		Roles:    []migration.Role{*role},
		IsActive: true,
	}

	official := &migration.Official{
		NIP:       req.NIP,
		Pangkat:   req.Pangkat,
		Jabatan:   req.Jabatan,
		Signature: req.Signature,
		IsOnDuty:  true,
	}

	if err := s.Repo.DB.Transaction(func(tx *gorm.DB) error {
		return s.Repo.CreateOfficialWithUser(tx, user, official)
	}); err != nil {
		log.Printf("error creating official by admin %d: %v", adminID, err)
		return nil, errs.InternalServerError("gagal membuat akun official")
	}

	if err := helpers.SendVerificationEmail(user.ID, user.Email, user.Name); err != nil {
		log.Printf("error sending official verification email: %v", err)
	}

	return &Response{StatusCode: http.StatusCreated, Message: "Official berhasil dibuat. Silakan verifikasi email official."}, nil
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

	log.Println("========== OCR RESULT ==========")
	log.Println(rawText)
	log.Println("================================")
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

	if err := helpers.SendVerificationEmail(user.ID, user.Email, user.Name); err != nil {
		log.Printf("error sending verification email for KRS registration: %v", err)
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

// helper functions

func (s *Service) resolvePendingStudent(studentID uint) (*migration.Student, error) {
	student, err := s.Repo.GetStudentByID(s.Repo.DB, studentID)
	if err != nil {
		return nil, errs.NotFound("mahasiswa tidak ditemukan")
	}

	if strings.ToLower(strings.TrimSpace(student.AdminVerificationStatus)) != "pending" {
		return nil, errs.BadRequest("mahasiswa tidak dalam status pending")
	}

	return student, nil
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

	if hasRole(user.Roles, "MAHASISWA") {
		if user.Student == nil {
			student, err := s.Repo.GetStudentByUserID(s.Repo.DB, user.ID)
			if err != nil {
				return errs.Unauthorized("Data mahasiswa tidak ditemukan")
			}
			user.Student = student
		}

		if strings.ToLower(strings.TrimSpace(user.Student.AdminVerificationStatus)) != "approved" {
			return errs.Unauthorized("Akun mahasiswa belum disetujui admin")
		}
	}

	if hasOfficialRole(user.Roles) {
		official, err := s.Repo.GetOfficialByUserID(s.Repo.DB, user.ID)
		if err != nil {
			return errs.InternalServerError("Gagal memvalidasi status official")
		}
		if err := policy.CanOfficialAct(user, official); err != nil {
			return errs.Unauthorized(err.Error())
		}
	}

	return nil
}

func hasRole(roles []migration.Role, code string) bool {
	for _, role := range roles {
		if strings.EqualFold(role.Code, code) {
			return true
		}
	}
	return false
}

func hasOfficialRole(roles []migration.Role) bool {
	for _, role := range roles {
		if role.Code == "DEKAN" || role.Code == "WAKIL_DEKAN" {
			return true
		}
	}
	return false
}

func studentStatus(student *migration.Student) string {
	if student == nil {
		return ""
	}
	return student.AdminVerificationStatus
}

func studentVerifiedAt(student *migration.Student) *time.Time {
	if student == nil {
		return nil
	}
	return student.AdminVerifiedAt
}

func studentRejection(student *migration.Student) string {
	if student == nil {
		return ""
	}
	return student.RejectionReason
}

func derefTime(value *time.Time) time.Time {
	if value == nil {
		return time.Time{}
	}
	return *value
}
