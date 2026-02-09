package letters

import (
	"github.com/reyimanuel/letter-administration/internal/migration"
	"gorm.io/gorm"
)

type repository struct {
	DB *gorm.DB
}

func NewRepository(db *gorm.DB) *repository {
	return &repository{DB: db}
}

func (r *repository) WithTx(fn func(tx *gorm.DB) error) error {
	return r.DB.Transaction(fn)
}

func (r *repository) GetStudentByUserID(tx *gorm.DB, userID uint) (*migration.Student, error) {
	var student migration.Student
	if err := tx.Where("user_id = ?", userID).First(&student).Error; err != nil {
		return nil, err
	}
	return &student, nil
}

func (r *repository) GetRoleByCode(tx *gorm.DB, code string) (*migration.Role, error) {
	var role migration.Role
	if err := tx.Where("code = ?", code).First(&role).Error; err != nil {
		return nil, err
	}
	return &role, nil
}
