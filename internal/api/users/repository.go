package user

import (
	"errors"
	"strings"

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
	if err := r.DB.Preload("Roles").Where("email = ?", email).First(&user).Error; err != nil {
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

func baseUserQuery(tx *gorm.DB) *gorm.DB {
	return tx.Model(&migration.User{}).
		Preload("Roles").
		Preload("Student")
}

func (r *Repository) GetPendingUsers(tx *gorm.DB) ([]migration.User, error) {
	var users []migration.User
	err := baseUserQuery(tx).
		Joins("LEFT JOIN students ON students.user_id = users.id").
		Joins("LEFT JOIN officials ON officials.user_id = users.id").
		Where("users.verified = ?", false).
		Where("students.id IS NOT NULL OR officials.id IS NOT NULL").
		Distinct("users.id").
		Order("users.created_at DESC").
		Find(&users).Error

	return users, err
}

func (r *Repository) GetOfficialsByUserIDs(tx *gorm.DB, userIDs []uint) ([]migration.Official, error) {
	if len(userIDs) == 0 {
		return []migration.Official{}, nil
	}

	var officials []migration.Official
	err := tx.Where("user_id IN ?", userIDs).Find(&officials).Error
	return officials, err
}

func (r *Repository) VerifyUser(tx *gorm.DB, userID uint) error {
	return tx.Model(&migration.User{}).Where("id = ?", userID).Update("Verified", true).Error
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
