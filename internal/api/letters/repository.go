package letters

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

func (r *Repository) WithTx(fn func(tx *gorm.DB) error) error {
	return r.DB.Transaction(fn)
}

func (r *Repository) UpsertTemplate(tx *gorm.DB, t *migration.LetterTemplate) error {
	var existing migration.LetterTemplate

	err := tx.Where("letter_type_id = ?", t.LetterTypeID).
		First(&existing).Error

	if errors.Is(err, gorm.ErrRecordNotFound) {
		return tx.Create(t).Error
	}

	if err != nil {
		return err
	}

	return tx.Model(&existing).Updates(map[string]interface{}{
		"file_path":  t.FilePath,
		"file_type":  t.FileType,
		"created_by": t.CreatedBy,
	}).Error
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

func (r *Repository) GetTemplateByLetterTypeID(
	tx *gorm.DB,
	letterTypeID uint,
) (*migration.LetterTemplate, error) {

	var template migration.LetterTemplate

	err := tx.
		Where("letter_type_id = ?", letterTypeID).
		First(&template).
		Error

	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, err
	}

	return &template, nil
}
