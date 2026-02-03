package migration

import "time"

var Models = []any{
	User{},
}

type User struct {
	ID        int       `gorm:"column:id;primaryKey;autoIncrement;not null;<-create"`
	Name      string    `gorm:"column:name;not null"`
	Email     string    `gorm:"column:email;uniqueIndex;not null"`
	Password  string    `gorm:"column:password;not null"`
	IsActive  bool      `gorm:"column:IsActive;not null;default:true"`
	CreatedAt time.Time `gorm:"column:created_at;autoCreateTime"`
	UpdatedAt time.Time `gorm:"column:updated_at;autoUpdateTime"`

	Roles   []Role   `gorm:"many2many:user_roles;"`
	Student *Student `gorm:"constraint:OnUpdate:CASCADE,OnDelete:SET NULL;"`
}
type Role struct {
	ID   uint   `gorm:"primaryKey"`
	Code string `gorm:"size:50;uniqueIndex;not null"`
	Name string `gorm:"size:100;not null"`
}

type Student struct {
	ID           uint   `gorm:"primaryKey"`
	UserID       uint   `gorm:"uniqueIndex;not null"`
	NPM          string `gorm:"size:20;uniqueIndex;not null"`
	ProgramStudi string `gorm:"size:100;not null"`
	Angkatan     int

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

	Subject string `gorm:"size:150;not null"`
	Content string `gorm:"type:text;not null"`

	Status string `gorm:"size:30;not null"`
	// draft, submitted, verified, signed, rejected

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

	LetterID   uint `gorm:"not null"`
	RoleID     uint `gorm:"not null"`
	ApproverID uint `gorm:"not null"`

	Status string `gorm:"size:30;not null"`
	// pending, approved, rejected

	Notes      string `gorm:"type:text"`
	ApprovedAt *time.Time

	Letter   Letter
	Role     Role
	Approver User
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
