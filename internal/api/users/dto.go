package user

import "time"

type LoginRequest struct {
	Email    string `json:"email" binding:"required"`
	Password string `json:"password" binding:"required"`
}

// RegisterStudentRequest is received as multipart/form-data.
// kredensial (bukti gambar KTM/KRS) is handled separately via ctx.FormFile("kredensial").
type RegisterStudentRequest struct {
	Name         string `form:"name"          binding:"required"`
	NIM          string `form:"nim"           binding:"required"`
	Email        string `form:"email"         binding:"required,email"`
	Password     string `form:"password"      binding:"required,min=6"`
	ProgramStudi string `form:"program_studi" binding:"required"`
	Angkatan     int    `form:"angkatan"      binding:"required"`
}

type Response struct {
	StatusCode int    `json:"status_code"`
	Message    string `json:"message"`
	Data       any    `json:"data,omitempty"`
}

type TokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
}

type PendingStudentResponse struct {
	StudentID               uint       `json:"student_id"`
	UserID                  uint       `json:"user_id"`
	Name                    string     `json:"name"`
	Email                   string     `json:"email"`
	NIM                     string     `json:"nim"`
	ProgramStudi            string     `json:"program_studi"`
	Angkatan                int        `json:"angkatan"`
	Kredensial              string     `json:"kredensial,omitempty"`
	AdminVerificationStatus string     `json:"admin_verification_status"`
	AdminVerifiedAt         *time.Time `json:"admin_verified_at,omitempty"`
	RejectionReason         string     `json:"rejection_reason,omitempty"`
	EmailVerifiedAt         *time.Time `json:"email_verified_at,omitempty"`
	CreatedAt               time.Time  `json:"created_at"`
}

// RegisterWithKRSRequest is received as multipart/form-data.
// The KRS image is handled separately via ctx.FormFile("krs").
type RegisterWithKRSRequest struct {
	Email    string `form:"email"    binding:"required,email"`
	Password string `form:"password" binding:"required,min=6"`
}

// KRSPreviewResponse is returned after a successful KRS-based registration
// so the student can confirm which data was extracted from their document.
type KRSPreviewResponse struct {
	UserID       uint   `json:"user_id"`
	Name         string `json:"name"`
	NIM          string `json:"nim"`
	ProgramStudi string `json:"program_studi"`
	Angkatan     int    `json:"angkatan"`
}

type UserListResponse struct {
	UserID                  uint       `json:"user_id"`
	Name                    string     `json:"name"`
	Email                   string     `json:"email"`
	Roles                   []string   `json:"roles"`
	IsActive                bool       `json:"is_active"`
	EmailVerifiedAt         *time.Time `json:"email_verified_at"`
	StudentID               *uint      `json:"student_id,omitempty"`
	AdminVerificationStatus string     `json:"admin_verification_status,omitempty"`
	AdminVerifiedAt         *time.Time `json:"admin_verified_at,omitempty"`
	RejectionReason         string     `json:"rejection_reason,omitempty"`
	CreatedAt               time.Time  `json:"created_at"`
}

type RejectStudentRequest struct {
	Reason string `json:"reason" binding:"required"`
}

// ApproveStudentRequest optionally carries corrected student data
// (e.g., to fix OCR ambiguities based on the uploaded KRS/KTM).
// If a field is omitted/empty, it will not be updated.
type ApproveStudentRequest struct {
	Name         string `json:"name,omitempty"`
	NIM          string `json:"nim,omitempty"`
	ProgramStudi string `json:"program_studi,omitempty"`
	Angkatan     *int   `json:"angkatan,omitempty"`
}

type VerifyEmailRequest struct {
	Token string `json:"token" binding:"required"`
}

type ResendVerificationRequest struct {
	Email string `json:"email" binding:"required,email"`
}

type MeResponse struct {
	UserID          uint       `json:"user_id"`
	Name            string     `json:"name"`
	Email           string     `json:"email"`
	ProfilePhoto    *string    `json:"profile_photo,omitempty"`
	Roles           []string   `json:"roles"`
	IsActive        bool       `json:"is_active"`
	EmailVerifiedAt *time.Time `json:"email_verified_at"`
	CreatedAt       time.Time  `json:"created_at"`

	// Student fields (only for MAHASISWA)
	StudentID               *uint      `json:"student_id,omitempty"`
	NIM                     string     `json:"nim,omitempty"`
	ProgramStudi            string     `json:"program_studi,omitempty"`
	Angkatan                int        `json:"angkatan,omitempty"`
	KredensialPath          string     `json:"kredensial_path,omitempty"`
	AdminVerificationStatus string     `json:"admin_verification_status,omitempty"`
	AdminVerifiedAt         *time.Time `json:"admin_verified_at,omitempty"`
	RejectionReason         string     `json:"rejection_reason,omitempty"`

	// Official fields (only for DEKAN/WAKIL_DEKAN)
	OfficialID *uint  `json:"official_id,omitempty"`
	NIP        string `json:"nip,omitempty"`
	Pangkat    string `json:"pangkat,omitempty"`
	Jabatan    string `json:"jabatan,omitempty"`
	Signature  string `json:"signature,omitempty"`
	IsOnDuty   *bool  `json:"is_on_duty,omitempty"`
}

type CreateOfficialRequest struct {
	Name      string `json:"name" binding:"required"`
	Email     string `json:"email" binding:"required,email"`
	Password  string `json:"password" binding:"required,min=6"`
	RoleCode  string `json:"role_code" binding:"required,oneof=DEKAN WAKIL_DEKAN"`
	NIP       string `json:"nip"`
	Pangkat   string `json:"pangkat"`
	Jabatan   string `json:"jabatan" binding:"required"`
	Signature string `json:"signature"`
}

// AdminUpdateUserRequest is used by admin to update a user account.
// All fields are optional; if omitted, the field will not be updated.
type AdminUpdateUserRequest struct {
	Name         *string `json:"name,omitempty"`
	Email        *string `json:"email,omitempty" binding:"omitempty,email"`
	IsActive     *bool   `json:"is_active,omitempty"`
	ProfilePhoto *string `json:"profile_photo,omitempty"`
}
