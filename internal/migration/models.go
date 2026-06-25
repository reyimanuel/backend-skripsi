package migration

import (
	"time"

	"gorm.io/datatypes"
)

var Models = []any{
	User{},
	UserDeviceToken{},
	UserNotification{},
	Atasan{},
	Role{},
	UserRole{},
	Student{},
	LetterType{},
	Letter{},
	LetterApproval{},
	LetterHistory{},
	LetterAttachment{},
	LetterTemplate{},
}

type User struct {
	ID              uint       `gorm:"column:id;primaryKey;autoIncrement;not null;<-create"`
	Name            string     `gorm:"column:name;not null"`
	Email           string     `gorm:"column:email;uniqueIndex;not null"`
	Password        string     `gorm:"column:password;not null"`
	ProfilePhoto    *string    `gorm:"column:profile_photo;size:255"`
	CreatedAt       time.Time  `gorm:"column:created_at;autoCreateTime"`
	UpdatedAt       time.Time  `gorm:"column:updated_at;autoUpdateTime"`
	EmailVerifiedAt *time.Time `gorm:"column:email_verified_at;index"`
	IsActive        bool       `gorm:"column:is_active;not null;default:true"`

	Roles   []Role   `gorm:"many2many:user_roles;"`
	Student *Student `gorm:"constraint:OnUpdate:CASCADE,OnDelete:SET NULL;"`
}

type UserDeviceToken struct {
	ID         uint       `gorm:"primaryKey"`
	UserID     uint       `gorm:"not null;index"`
	Token      string     `gorm:"size:512;uniqueIndex;not null"`
	Platform   string     `gorm:"size:30;not null;default:web"` // e.g., "web", "android", "ios"
	CreatedAt  time.Time  `gorm:"autoCreateTime"`
	UpdatedAt  time.Time  `gorm:"autoUpdateTime"`
	RevokedAt  bool       `gorm:"not null;default:false"` // kept for backward compatibility (acts as revoked flag)
	LastSentAt *time.Time `gorm:"index"`                  // last seen / last updated timestamp

	User User `gorm:"constraint:OnDelete:CASCADE;"`
}

// UserNotification stores an in-app notification record so FE can display
// notification history (e.g. last 7 days), even when push delivery fails.
type UserNotification struct {
	ID        uint           `gorm:"primaryKey"`
	UserID    uint           `gorm:"not null;index"`
	Title     string         `gorm:"size:150;not null"`
	Body      string         `gorm:"type:text;not null"`
	Data      datatypes.JSON `gorm:"type:jsonb;not null;default:'{}'"`
	ReadAt    *time.Time     `gorm:"index"`
	CreatedAt time.Time      `gorm:"autoCreateTime;index"`

	User User `gorm:"constraint:OnDelete:CASCADE;"`
}

type Atasan struct {
	ID        uint   `gorm:"primaryKey"`
	UserID    uint   `gorm:"not null"`
	NIP       string `gorm:"size:50"`
	Pangkat   string `gorm:"size:100"`
	Jabatan   string `gorm:"size:100"`                      // Dean, Vice Dean, etc.
	Signature string `gorm:"size:255"`                      // path to signature image
	IsOnDuty  bool   `gorm:"column:is_active;default:true"` // if the atasan is still active on duty

	User User `gorm:"constraint:OnDelete:CASCADE;"`
}

func (Atasan) TableName() string {
	return "atasan"
}

type UserRole struct {
	UserID uint `gorm:"primaryKey"`
	RoleID uint `gorm:"primaryKey"`

	User User `gorm:"foreignKey:UserID;constraint:OnDelete:CASCADE;"`
	Role Role `gorm:"foreignKey:RoleID;constraint:OnDelete:CASCADE;"`
}

type Role struct {
	ID   uint   `gorm:"primaryKey"`
	Code string `gorm:"size:50;uniqueIndex;not null"`
	Name string `gorm:"size:100;not null"`
}

type Student struct {
	ID                      uint   `gorm:"primaryKey"`
	UserID                  uint   `gorm:"uniqueIndex;not null"`
	NIM                     string `gorm:"size:20;uniqueIndex;not null"`
	ProgramStudi            string `gorm:"size:100;not null"`
	Angkatan                int
	SemesterMasukKuliah     string `gorm:"column:semester_masuk_kuliah;size:10"`
	KredensialPath          string `gorm:"size:255"` // path to uploaded credential (KTM/KRS)
	AdminVerificationStatus string `gorm:"size:20;not null;default:pending;index"`
	AdminVerifiedBy         *uint  `gorm:"index"`
	AdminVerifier           *User  `gorm:"foreignKey:AdminVerifiedBy;references:ID;constraint:OnUpdate:CASCADE,OnDelete:SET NULL;"`
	AdminVerifiedAt         *time.Time
	RejectionReason         string `gorm:"type:text"`

	User    User
	Letters []Letter

	CreatedAt time.Time
	UpdatedAt time.Time
}

type LetterType struct {
	ID                 uint   `gorm:"primaryKey"`
	Code               string `gorm:"size:50;uniqueIndex;not null"`
	Name               string `gorm:"size:100;not null"`
	Description        string `gorm:"type:text"`
	WorkCode           string `gorm:"column:kode_kerja;size:50"`
	ClassificationCode string `gorm:"column:kode_klasifikasi;size:50"`

	AttachmentRequirements datatypes.JSON `gorm:"type:jsonb;not null;default:'[]'"`

	Letters []Letter
}

type Letter struct {
	ID uint `gorm:"primaryKey"`

	StudentID    uint `gorm:"not null"`
	LetterTypeID uint `gorm:"not null"`

	Subject string         `gorm:"size:150;not null"`
	Payload datatypes.JSON `gorm:"type:jsonb;not null"`

	Status string `gorm:"size:30;not null"`
	// draft, submitted, forwarded, approved, signed, rejected

	SignedByID *uint
	SignedBy   *Atasan `gorm:"foreignKey:SignedByID"`
	SignedAt   *time.Time

	FilePath string `gorm:"size:255"`

	LetterNumber *string `gorm:"size:100"`

	Student    Student
	LetterType LetterType

	Approvals   []LetterApproval
	Histories   []LetterHistory
	Attachments []LetterAttachment

	CreatedAt time.Time
	UpdatedAt time.Time
}

type LetterApproval struct {
	ID uint `gorm:"primaryKey"`

	LetterID   uint  `gorm:"not null"`
	RoleID     uint  `gorm:"not null"`
	ApproverID *uint `gorm:"default:null"`

	Status string `gorm:"size:30;not null"`
	// pending, approved, rejected

	Notes      string `gorm:"type:text"`
	ApprovedAt *time.Time

	Letter   Letter
	Role     Role
	Approver *User
}

type LetterHistory struct {
	ID uint `gorm:"primaryKey"`

	LetterID uint `gorm:"not null"`
	ActorID  uint `gorm:"not null"`

	Action string `gorm:"size:50;not null"`
	Notes  string `gorm:"type:text"`

	Letter Letter
	Actor  User

	CreatedAt time.Time
}

type LetterAttachment struct {
	ID uint `gorm:"primaryKey"`

	LetterID uint `gorm:"not null"`

	RequirementKey string `gorm:"column:requirement_key;size:50;not null;default:'';index"`

	FilePath string `gorm:"size:255;not null"`
	FileType string `gorm:"size:50"`

	Letter Letter

	UploadedAt time.Time
}

type LetterTemplate struct {
	ID uint `gorm:"primaryKey"`

	LetterTypeID uint `gorm:"uniqueIndex;not null"`

	FilePath string `gorm:"size:255;not null"`
	FileType string `gorm:"size:20;not null"`

	// Placeholders contains the list of detected {{key}} placeholders inside the docx.
	// Stored for introspection (e.g. FE form generation / validation).
	Placeholders datatypes.JSON `gorm:"type:jsonb;not null;default:'[]'"`

	CreatedBy uint `gorm:"not null"`

	CreatedAt time.Time
	UpdatedAt time.Time

	LetterType LetterType `gorm:"foreignKey:LetterTypeID;constraint:OnUpdate:CASCADE,OnDelete:CASCADE;"`

	Creator User `gorm:"foreignKey:CreatedBy;references:ID;constraint:OnUpdate:CASCADE,OnDelete:SET NULL;"`
}

func (u *User) RoleSlice() []string {
	out := make([]string, 0, len(u.Roles))
	for _, r := range u.Roles {
		out = append(out, r.Code)
	}
	return out
}
