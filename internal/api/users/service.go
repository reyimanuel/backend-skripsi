package user

import (
	"log"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/reyimanuel/letter-administration/internal/infrastructures/pkg/errs"
	"github.com/reyimanuel/letter-administration/internal/infrastructures/pkg/helpers"
	"github.com/reyimanuel/letter-administration/internal/infrastructures/pkg/token"
	"github.com/reyimanuel/letter-administration/internal/migration"
	"gorm.io/gorm"
)

type Service struct {
	Repo     *Repository
	EmailSvc *helpers.SendGridEmailService
}

func NewService(repo *Repository) *Service {
	return &Service{
		Repo:     repo,
		EmailSvc: helpers.NewSendGridEmailService(),
	}
}

func (s *Service) Login(payload *LoginRequest) (*Response, error) {
	user, err := s.Repo.GetByEmail(payload.Email)
	if err != nil {
		return nil, errs.Unauthorized("Email atau Password Salah")
	}

	if !helpers.CheckPasswordHash(payload.Password, user.Password) {
		return nil, errs.Unauthorized("Email atau Password Salah")
	}

	// For students: check email verification first, then admin activation.
	for _, role := range user.Roles {
		if role.Code == "MAHASISWA" {
			student, err := s.Repo.GetStudentByUserID(s.Repo.DB, user.ID)
			if err != nil || !student.EmailVerified {
				return nil, errs.Unauthorized("Email belum diverifikasi. Silakan cek inbox email Anda.")
			}
			break
		}
	}

	if !user.Verified {
		return nil, errs.Unauthorized("Akun Anda belum diaktifkan. Silakan tunggu konfirmasi admin.")
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
		Verified: false,
		Roles:    []migration.Role{*mahasiswaRole},
	}
	student := &migration.Student{
		NIM:            payload.NIM,
		ProgramStudi:   payload.ProgramStudi,
		Angkatan:       payload.Angkatan,
		KredensialPath: filePath,
	}

	if err := s.Repo.DB.Transaction(func(tx *gorm.DB) error {
		return s.Repo.CreateStudentWithUser(tx, user, student)
	}); err != nil {
		os.Remove(filePath)
		log.Printf("error mendaftarkan mahasiswa: %v", err)
		return nil, errs.InternalServerError("gagal mendaftarkan akun mahasiswa")
	}

	return &Response{
		StatusCode: http.StatusCreated,
		Message:    "Pendaftaran berhasil. silahkan tunggu verifikasi admin.",
	}, nil
}

func (s *Service) GetPendingStudents() (*Response, error) {
	students, err := s.Repo.GetPendingStudents(s.Repo.DB)
	if err != nil {
		log.Printf("error mengambil pending students: %v", err)
		return nil, errs.InternalServerError("gagal mengambil data mahasiswa pending")
	}

	result := make([]PendingStudentResponse, 0, len(students))
	for _, st := range students {
		result = append(result, PendingStudentResponse{
			UserID:     st.UserID,
			Name:       st.User.Name,
			Email:      st.User.Email,
			NIM:        st.NIM,
			Kredensial: st.KredensialPath,
		})
	}

	return &Response{
		StatusCode: http.StatusOK,
		Message:    "Data mahasiswa pending berhasil diambil",
		Data:       result,
	}, nil
}

func (s *Service) ApproveStudent(studentID uint) (*Response, error) {
	student, err := s.Repo.GetStudentByID(s.Repo.DB, studentID)
	if err != nil {
		return nil, errs.NotFound("mahasiswa tidak ditemukan")
	}

	if err := s.Repo.VerifyUser(s.Repo.DB, student.UserID); err != nil {
		log.Printf("error mengaktifkan akun mahasiswa %d: %v", studentID, err)
		return nil, errs.InternalServerError("gagal mengaktifkan akun mahasiswa")
	}

	if student.KredensialPath != "" {
		if err := os.Remove(student.KredensialPath); err != nil && !os.IsNotExist(err) {
			log.Printf("gagal menghapus file kredensial %s: %v", student.KredensialPath, err)
		}
		if err := s.Repo.ClearStudentKredensial(s.Repo.DB, studentID); err != nil {
			log.Printf("error membersihkan path kredensial mahasiswa %d: %v", studentID, err)
		}
	}

	return &Response{
		StatusCode: http.StatusOK,
		Message:    "Data berhasil diverifikasi dan akun telah diaktifkan",
	}, nil
}

func (s *Service) RejectStudent(studentID uint) (*Response, error) {
	student, err := s.Repo.GetStudentByID(s.Repo.DB, studentID)
	if err != nil {
		return nil, errs.NotFound("mahasiswa tidak ditemukan")
	}

	if student.KredensialPath != "" {
		if err := os.Remove(student.KredensialPath); err != nil && !os.IsNotExist(err) {
			log.Printf("gagal menghapus file kredensial %s: %v", student.KredensialPath, err)
			return nil, errs.InternalServerError("gagal menghapus file kredensial mahasiswa")
		}
	}

	if err := s.Repo.DeleteUser(s.Repo.DB, student.UserID); err != nil {
		log.Printf("error menolak pendaftaran mahasiswa %d: %v", studentID, err)
		return nil, errs.InternalServerError("gagal menolak pendaftaran mahasiswa")
	}

	return &Response{
		StatusCode: http.StatusOK,
		Message:    "Pendaftaran mahasiswa berhasil ditolak",
	}, nil
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
		Verified: false,
		Roles:    []migration.Role{*mahasiswaRole},
	}
	student := &migration.Student{
		NIM:            krsData.NIM,
		ProgramStudi:   krsData.ProgramStudi,
		Angkatan:       krsData.Angkatan,
		KredensialPath: filePath,
	}

	if err := s.Repo.DB.Transaction(func(tx *gorm.DB) error {
		return s.Repo.CreateStudentWithUser(tx, user, student)
	}); err != nil {
		os.Remove(filePath)
		log.Printf("error mendaftarkan mahasiswa via KRS: %v", err)
		return nil, errs.InternalServerError("gagal mendaftarkan akun mahasiswa")
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
