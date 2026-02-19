package letters

import (
	"github.com/reyimanuel/letter-administration/internal/migration"
	"gorm.io/gorm"
)

type Repository struct {
	DB *gorm.DB
}

func NewRepository(db *gorm.DB) *Repository {
	return &Repository{DB: db}
}

func (r *Repository) WithTx(fn func(tx *gorm.DB) error) error {
	return r.DB.Transaction(fn)
}

func (r *Repository) UpsertTemplate(tx *gorm.DB, t *migration.LetterTemplate) error {
	return tx.
		Where("letter_type_id = ?", t.LetterTypeID).
		Assign(t).
		FirstOrCreate(t).
		Error
}

func (r *Repository) GetLetterTypeByID(tx *gorm.DB, id uint) (*migration.LetterType, error) {
	var letterType migration.LetterType
	if err := tx.First(&letterType, id).Error; err != nil {
		return nil, err
	}
	return &letterType, nil
}

func (r *Repository) GetLetterByID(tx *gorm.DB, letterID uint) (*migration.Letter, error) {
	var letter migration.Letter
	if err := tx.Where("id = ?", letterID).First(&letter).Error; err != nil {
		return nil, err
	}
	return &letter, nil
}
