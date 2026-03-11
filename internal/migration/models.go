package migration

import (
	"time"

	"gorm.io/datatypes"
)

var Models = []any{
	User{},
	Official{},
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
	ID        uint      `gorm:"column:id;primaryKey;autoIncrement;not null;<-create"`
	Name      string    `gorm:"column:name;not null"`
	Email     string    `gorm:"column:email;uniqueIndex;not null"`
	Password  string    `gorm:"column:password;not null"`
	CreatedAt time.Time `gorm:"column:created_at;autoCreateTime"`
	UpdatedAt time.Time `gorm:"column:updated_at;autoUpdateTime"`
	Verified  bool      `gorm:"column:verified;not null;default:false"` // diaktifkan oleh admin setelah verifikasi data

	Roles   []Role   `gorm:"many2many:user_roles;"`
	Student *Student `gorm:"constraint:OnUpdate:CASCADE,OnDelete:SET NULL;"`
}

type Official struct {
	ID            uint   `gorm:"primaryKey"`
	UserID        uint   `gorm:"not null"`
	NIP           string `gorm:"size:50"`
	Pangkat       string `gorm:"size:100"`
	Jabatan       string `gorm:"size:100"`     // Dean, Vice Dean, etc.
	Signature     string `gorm:"size:255"`     // path to signature image
	IsActive      bool   `gorm:"default:true"` // if the official is still active on duty
	EmailVerified bool   `gorm:"default:false"`

	User User `gorm:"constraint:OnDelete:CASCADE;"`
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
	ID             uint   `gorm:"primaryKey"`
	UserID         uint   `gorm:"uniqueIndex;not null"`
	NIM            string `gorm:"size:20;uniqueIndex;not null"`
	ProgramStudi   string `gorm:"size:100;not null"`
	Angkatan       int
	EmailVerified  bool   `gorm:"default:false"`
	KredensialPath string `gorm:"size:255"` // path to uploaded credential (KTM/KRS)

	User    User
	Letters []Letter

	CreatedAt time.Time
	UpdatedAt time.Time
}

type LetterType struct {
	ID          uint   `gorm:"primaryKey"`
	Code        string `gorm:"size:50;uniqueIndex;not null"`
	Name        string `gorm:"size:100;not null"`
	Description string `gorm:"type:text"`

	Letters []Letter
}

type Letter struct {
	ID uint `gorm:"primaryKey"`

	StudentID    uint `gorm:"not null"`
	LetterTypeID uint `gorm:"not null"`

	Subject string         `gorm:"size:150;not null"`
	Payload datatypes.JSON `gorm:"type:jsonb;not null"`

	Status string `gorm:"size:30;not null"`
	// draft, submitted, verified, signed, rejected

	SignedByID *uint
	SignedBy   *Official `gorm:"foreignKey:SignedByID"`
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
