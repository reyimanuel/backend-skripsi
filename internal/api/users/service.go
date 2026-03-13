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

func (s *Service) GetPendingUsers() (*Response, error) {
	users, err := s.Repo.GetPendingUsers(s.Repo.DB)
	if err != nil {
		log.Printf("error mengambil pending users: %v", err)
		return nil, errs.InternalServerError("gagal mengambil data user pending")
	}

	officialMap, err := s.loadOfficialMap(extractUserIDs(users))
	if err != nil {
		log.Printf("error mengambil data official pending: %v", err)
		return nil, errs.InternalServerError("gagal mengambil data user pending")
	}

	result := make([]PendingUserResponse, 0, len(users))
	for _, usr := range users {
		item := PendingUserResponse{
			UserID:    usr.ID,
			Name:      usr.Name,
			Email:     usr.Email,
			Roles:     usr.RoleSlice(),
			CreatedAt: usr.CreatedAt,
		}

		if usr.Student != nil {
			item.UserType = "student"
			item.NIM = usr.Student.NIM
			item.ProgramStudi = usr.Student.ProgramStudi
			item.Kredensial = usr.Student.KredensialPath
		} else if official, exists := officialMap[usr.ID]; exists {
			item.UserType = "official"
			item.Jabatan = official.Jabatan
			item.NIP = official.NIP
		} else {
			item.UserType = "user"
		}

		result = append(result, item)
	}

	return &Response{
		StatusCode: http.StatusOK,
		Message:    "Data user pending berhasil diambil",
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
		emailVerified := false
		for _, role := range usr.Roles {
			if role.Code == "MAHASISWA" && usr.Student != nil {
				emailVerified = usr.Student.EmailVerified
				break
			}
		}

		result = append(result, UserListResponse{
			UserID:        usr.ID,
			Name:          usr.Name,
			Email:         usr.Email,
			Roles:         usr.RoleSlice(),
			Verified:      usr.Verified,
			EmailVerified: emailVerified,
			CreatedAt:     usr.CreatedAt,
		})
	}

	return &Response{
		StatusCode: http.StatusOK,
		Message:    "Data users berhasil diambil",
		Data:       result,
	}, nil
}

func (s *Service) ApproveUser(userID uint) (*Response, error) {
	user, _, err := s.resolvePendingUser(userID)
	if err != nil {
		return nil, err
	}

	if err := s.Repo.VerifyUser(s.Repo.DB, userID); err != nil {
		log.Printf("error memverifikasi user %d: %v", userID, err)
		return nil, errs.InternalServerError("gagal memverifikasi user")
	}

	if user.Student != nil && user.Student.KredensialPath != "" {
		if err := os.Remove(user.Student.KredensialPath); err != nil && !os.IsNotExist(err) {
			log.Printf("gagal menghapus file kredensial %s: %v", user.Student.KredensialPath, err)
		}
		if err := s.Repo.ClearStudentKredensial(s.Repo.DB, user.Student.ID); err != nil {
			log.Printf("error membersihkan path kredensial mahasiswa user %d: %v", userID, err)
		}
	}

	return &Response{
		StatusCode: http.StatusOK,
		Message:    "Data berhasil diverifikasi dan akun telah diaktifkan",
	}, nil
}

func (s *Service) RejectUser(userID uint) (*Response, error) {
	user, _, err := s.resolvePendingUser(userID)
	if err != nil {
		return nil, err
	}

	if user.Student != nil && user.Student.KredensialPath != "" {
		if err := os.Remove(user.Student.KredensialPath); err != nil && !os.IsNotExist(err) {
			log.Printf("gagal menghapus file kredensial %s: %v", user.Student.KredensialPath, err)
			return nil, errs.InternalServerError("gagal menghapus file kredensial mahasiswa")
		}
	}

	if err := s.Repo.DeleteUser(s.Repo.DB, userID); err != nil {
		log.Printf("error menolak pendaftaran user %d: %v", userID, err)
		return nil, errs.InternalServerError("gagal menolak pendaftaran user")
	}

	return &Response{
		StatusCode: http.StatusOK,
		Message:    "Pendaftaran user berhasil ditolak",
	}, nil
}

func extractUserIDs(users []migration.User) []uint {
	ids := make([]uint, 0, len(users))
	for _, usr := range users {
		ids = append(ids, usr.ID)
	}
	return ids
}

func (s *Service) loadOfficialMap(userIDs []uint) (map[uint]migration.Official, error) {
	officials, err := s.Repo.GetOfficialsByUserIDs(s.Repo.DB, userIDs)
	if err != nil {
		return nil, err
	}

	officialMap := make(map[uint]migration.Official, len(officials))
	for _, official := range officials {
		officialMap[official.UserID] = official
	}

	return officialMap, nil
}

func (s *Service) resolvePendingUser(userID uint) (*migration.User, *migration.Official, error) {
	user, err := s.Repo.GetUserByID(s.Repo.DB, userID)
	if err != nil {
		return nil, nil, errs.NotFound("user tidak ditemukan")
	}

	if user.Verified {
		return nil, nil, errs.BadRequest("user sudah diverifikasi")
	}

	officialMap, err := s.loadOfficialMap([]uint{userID})
	if err != nil {
		log.Printf("error mengambil data official user %d: %v", userID, err)
		return nil, nil, errs.InternalServerError("gagal mengambil data user")
	}

	official := (*migration.Official)(nil)
	if foundOfficial, exists := officialMap[userID]; exists {
		official = &foundOfficial
	}

	if user.Student == nil && official == nil {
		return nil, nil, errs.BadRequest("user bukan mahasiswa atau official")
	}

	return user, official, nil
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
