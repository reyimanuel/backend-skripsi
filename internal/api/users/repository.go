package user

import (
	"errors"

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
	if err := tx.Where("user_id = ?", userID).First(&student).Error; err != nil {
		return nil, err
	}
	return &student, nil
}

func (r *Repository) GetStudentByID(tx *gorm.DB, id uint) (*migration.Student, error) {
	var student migration.Student
	if err := tx.Where("id = ?", id).First(&student).Error; err != nil {
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

	err := tx.Preload("User").
		Where("jabatan = ? AND is_active = ?", role, true).
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
