package user

import "time"

type LoginRequest struct {
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required"`
}

type RefreshTokenRequest struct {
	RefreshToken string `json:"refresh_token" binding:"required"`
}

type LogoutRequest struct {
	RefreshToken string `json:"refresh_token"`
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
	SemesterMasukKuliah     string     `json:"semester_masuk_kuliah"`
	Kredensial              string     `json:"kredensial,omitempty"`
	AdminVerificationStatus string     `json:"admin_verification_status"`
	AdminVerifiedAt         *time.Time `json:"admin_verified_at,omitempty"`
	RejectionReason         string     `json:"rejection_reason,omitempty"`
	EmailVerifiedAt         *time.Time `json:"email_verified_at,omitempty"`
	CreatedAt               time.Time  `json:"created_at"`
}

type PendingStudentListData struct {
	Items []PendingStudentResponse `json:"items"`
	Meta  PaginationMeta           `json:"meta"`
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

type UserListData struct {
	Items []UserListResponse `json:"items"`
	Meta  PaginationMeta     `json:"meta"`
}

type PaginationMeta struct {
	Page     int   `json:"page"`
	PageSize int   `json:"page_size"`
	Total    int64 `json:"total"`
}

// GetUsersQuery represents query parameters for user list endpoints
type GetUsersQuery struct {
	Page     int `form:"page" default:"1"`
	PageSize int `form:"page_size" default:"20"`
}

type RejectStudentRequest struct {
	Reason string `json:"reason" binding:"required"`
}

// ApproveStudentRequest optionally carries corrected student data.
// If a field is omitted/empty, it will not be updated.
type ApproveStudentRequest struct {
	Name                string `json:"name,omitempty"`
	NIM                 string `json:"nim,omitempty"`
	ProgramStudi        string `json:"program_studi,omitempty"`
	Angkatan            *int   `json:"angkatan,omitempty"`
	SemesterMasukKuliah string `json:"semester_masuk_kuliah,omitempty"`
}

type CreateStudentInvitationRequest struct {
	Name                string `json:"name" binding:"required,safehtml"`
	NIM                 string `json:"nim" binding:"required"`
	Email               string `json:"email" binding:"required,email"`
	ProgramStudi        string `json:"program_studi" binding:"required,safehtml"`
	Angkatan            int    `json:"angkatan" binding:"required"`
	SemesterMasukKuliah string `json:"semester_masuk_kuliah,omitempty"`
}

type BulkStudentInvitationRowResult struct {
	Row                 int    `json:"row"`
	Name                string `json:"name,omitempty"`
	NIM                 string `json:"nim,omitempty"`
	Email               string `json:"email,omitempty"`
	ProgramStudi        string `json:"program_studi,omitempty"`
	Angkatan            int    `json:"angkatan,omitempty"`
	SemesterMasukKuliah string `json:"semester_masuk_kuliah,omitempty"`
	Status              string `json:"status"`
	Error               string `json:"error,omitempty"`
}

type BulkStudentInvitationImportData struct {
	TotalCount   int                              `json:"total_count"`
	SuccessCount int                              `json:"success_count"`
	FailedCount  int                              `json:"failed_count"`
	Items        []BulkStudentInvitationRowResult `json:"items"`
}

type CompleteStudentInvitationRequest struct {
	Token    string `json:"token" binding:"required"`
	Password string `json:"password" binding:"required,strongpassword"`
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
	SemesterMasukKuliah     string     `json:"semester_masuk_kuliah,omitempty"`
	KredensialPath          string     `json:"kredensial_path,omitempty"`
	AdminVerificationStatus string     `json:"admin_verification_status,omitempty"`
	AdminVerifiedAt         *time.Time `json:"admin_verified_at,omitempty"`
	RejectionReason         string     `json:"rejection_reason,omitempty"`

	// Atasan fields (only for atasan roles)
	AtasanID  *uint  `json:"atasan_id,omitempty"`
	NIP       string `json:"nip,omitempty"`
	Pangkat   string `json:"pangkat,omitempty"`
	Jabatan   string `json:"jabatan,omitempty"`
	Signature string `json:"signature,omitempty"`
	IsOnDuty  *bool  `json:"is_on_duty,omitempty"`
}

type CreateStaffRequest struct {
	Name     string `form:"name" binding:"required,safehtml"`
	Email    string `form:"email" binding:"required,email"`
	RoleCode string `form:"role_code" binding:"required,oneof=ADMIN ATASAN"`
	Jabatan  string `form:"jabatan"`
}

type CompleteStaffInvitationRequest struct {
	Token    string `form:"token" binding:"required"`
	Password string `form:"password" binding:"required,strongpassword"`

	NIP     string `form:"nip"`
	Pangkat string `form:"pangkat"`
	Jabatan string `form:"jabatan"`
}

// AdminUpdateUserRequest is used by admin to update a user account.
// All fields are optional; if omitted, the field will not be updated.
type AdminUpdateUserRequest struct {
	Name         *string `json:"name,omitempty"`
	Email        *string `json:"email,omitempty" binding:"omitempty,email"`
	IsActive     *bool   `json:"is_active,omitempty"`
	ProfilePhoto *string `json:"profile_photo,omitempty"`
}

// UpdateMyProfileRequest allows a user to update their own profile.
// Only name and profile_photo can be updated by the user themselves.
type UpdateMyProfileRequest struct {
	Name         *string `json:"name,omitempty"`
	ProfilePhoto *string `json:"profile_photo,omitempty"`
}

// UpsertFCMTokenRequest registers or updates an FCM registration token for the current user.
// FE (Next.js PWA / wrapped APK) should call this after login and whenever the token changes.
type UpsertFCMTokenRequest struct {
	Token    string `json:"token" binding:"required"`
	Platform string `json:"platform" binding:"omitempty"` // e.g. "web", "android", "ios"
}

type DeleteFCMTokenRequest struct {
	Token string `json:"token" binding:"required"`
}
