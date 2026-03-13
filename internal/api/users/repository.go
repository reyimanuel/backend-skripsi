package user

import (
	"errors"
	"strings"
	"time"

	"github.com/reyimanuel/letter-administration/internal/migration"
	"gorm.io/gorm"
)

type Repository struct {
	DB *gorm.DB
}

func NewRepository(db *gorm.DB) *Repository {
	return &Repository{DB: db}
}

func (r *Repository) GetByEmail(email string) (*migration.User, error) {
	var user migration.User
	if err := r.DB.Preload("Roles").Preload("Student").Where("email = ?", email).First(&user).Error; err != nil {
		return nil, err
	}
	return &user, nil
}

func (r *Repository) GetUserByID(tx *gorm.DB, userID uint) (*migration.User, error) {
	var user migration.User
	if err := tx.Preload("Roles").Preload("Student").Where("id = ?", userID).First(&user).Error; err != nil {
		return nil, err
	}
	return &user, nil
}

func (r *Repository) GetStudentByUserID(tx *gorm.DB, userID uint) (*migration.Student, error) {
	var student migration.Student
	if err := tx.Preload("User").Where("user_id = ?", userID).First(&student).Error; err != nil {
		return nil, err
	}
	return &student, nil
}

func (r *Repository) GetStudentByID(tx *gorm.DB, id uint) (*migration.Student, error) {
	var student migration.Student
	if err := tx.Preload("User").Where("id = ?", id).First(&student).Error; err != nil {
		return nil, err
	}
	return &student, nil
}

func (r *Repository) GetRoleByCode(tx *gorm.DB, code string) (*migration.Role, error) {
	var role migration.Role
	if err := tx.Where("code = ?", code).First(&role).Error; err != nil {
		return nil, err
	}
	return &role, nil
}

func (r *Repository) GetActiveOfficialByRole(tx *gorm.DB, role string) (*migration.Official, error) {
	var officials []migration.Official

	// Normalize: replace underscores with spaces so "WAKIL_DEKAN" matches "Wakil Dekan"
	normalized := strings.ReplaceAll(role, "_", " ")

	err := tx.Preload("User").
		Where("LOWER(jabatan) = LOWER(?) AND is_active = ?", normalized, true).
		Find(&officials).Error

	if err != nil {
		return nil, err
	}

	if len(officials) == 0 {
		return nil, errors.New("pejabat aktif tidak ditemukan")
	}

	if len(officials) > 1 {
		return nil, errors.New("data pejabat aktif duplikat untuk jabatan ini")
	}

	return &officials[0], nil
}

func (r *Repository) GetByNIM(nim string) (*migration.Student, error) {
	var student migration.Student
	if err := r.DB.Where("nim = ?", nim).First(&student).Error; err != nil {
		return nil, err
	}
	return &student, nil
}

func (r *Repository) CreateStudentWithUser(tx *gorm.DB, user *migration.User, student *migration.Student) error {
	if err := tx.Create(user).Error; err != nil {
		return err
	}
	student.UserID = user.ID
	return tx.Create(student).Error
}

func (r *Repository) CreateOfficialWithUser(tx *gorm.DB, user *migration.User, official *migration.Official) error {
	if err := tx.Create(user).Error; err != nil {
		return err
	}

	official.UserID = user.ID
	return tx.Create(official).Error
}

func baseUserQuery(tx *gorm.DB) *gorm.DB {
	return tx.Model(&migration.User{}).
		Preload("Roles").
		Preload("Student")
}

func (r *Repository) GetPendingStudents(tx *gorm.DB) ([]migration.Student, error) {
	var students []migration.Student
	err := tx.Preload("User").
		Where("admin_verification_status = ?", "pending").
		Order("created_at DESC").
		Find(&students).Error
	return students, err
}

func (r *Repository) UpdateStudentAdminVerification(tx *gorm.DB, studentID uint, status string, adminID *uint, reason string) error {
	updates := map[string]any{
		"admin_verification_status": status,
		"admin_verified_by":         adminID,
		"admin_verified_at":         time.Now(),
		"rejection_reason":          reason,
	}

	return tx.Model(&migration.Student{}).Where("id = ?", studentID).Updates(updates).Error
}

func (r *Repository) SetUserEmailVerified(tx *gorm.DB, userID uint, verifiedAt time.Time) error {
	return tx.Model(&migration.User{}).Where("id = ?", userID).Update("email_verified_at", verifiedAt).Error
}

func (r *Repository) ClearStudentKredensial(tx *gorm.DB, studentID uint) error {
	return tx.Model(&migration.Student{}).Where("id = ?", studentID).Update("kredensial_path", "").Error
}

func (r *Repository) DeleteUser(tx *gorm.DB, userID uint) error {
	return tx.Delete(&migration.User{}, userID).Error
}

func (r *Repository) GetAllUsers(tx *gorm.DB) ([]migration.User, error) {
	var users []migration.User
	err := baseUserQuery(tx).
		Order("users.created_at DESC").
		Find(&users).Error
	return users, err
}

func (r *Repository) GetOfficialByUserID(tx *gorm.DB, userID uint) (*migration.Official, error) {
	var official migration.Official
	if err := tx.Preload("User").Where("user_id = ?", userID).First(&official).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}

	return &official, nil
}
