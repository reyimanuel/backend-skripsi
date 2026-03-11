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

func (r *Repository) GetPendingStudents(tx *gorm.DB) ([]migration.Student, error) {
	var students []migration.Student
	err := tx.Preload("User").
		Joins("JOIN users ON users.id = students.user_id").
		Where("users.verified = ?", false).
		Find(&students).Error
	return students, err
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
