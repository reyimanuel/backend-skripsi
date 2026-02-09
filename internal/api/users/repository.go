package user

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

func (r *repository) GetByEmail(email string) (*migration.User, error) {
	var user migration.User
	if err := r.DB.Where("email = ?", email).First(&user).Error; err != nil {
		return nil, err
	}
	return &user, nil
}
