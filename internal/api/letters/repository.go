package letters

import (
	"errors"

	"github.com/reyimanuel/letter-administration/internal/migration"
	"gorm.io/datatypes"
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
		"file_path":    t.FilePath,
		"file_type":    t.FileType,
		"placeholders": t.Placeholders,
		"created_by":   t.CreatedBy,
	}).Error
}

func (r *Repository) GetLetterTypeByID(tx *gorm.DB, id uint) (*migration.LetterType, error) {
	var letterType migration.LetterType
	if err := tx.First(&letterType, id).Error; err != nil {
		return nil, err
	}
	return &letterType, nil
}

func (r *Repository) GetLetterTypeByCode(tx *gorm.DB, code string) (*migration.LetterType, error) {
	var letterType migration.LetterType
	if err := tx.Where("code = ?", code).First(&letterType).Error; err != nil {
		return nil, err
	}
	return &letterType, nil
}

func (r *Repository) CreateLetterType(tx *gorm.DB, t *migration.LetterType) error {
	return tx.Create(t).Error
}

func (r *Repository) UpdateLetterTypeAttachmentRequirements(tx *gorm.DB, letterTypeID uint, reqs datatypes.JSON) error {
	return tx.Model(&migration.LetterType{}).
		Where("id = ?", letterTypeID).
		Update("attachment_requirements", reqs).Error
}

func (r *Repository) UpdateLetterType(tx *gorm.DB, id uint, code, name, description, workCode, classificationCode string) error {
	updates := map[string]interface{}{
		"name":             name,
		"description":      description,
		"kode_kerja":       workCode,
		"kode_klasifikasi": classificationCode,
	}
	if code != "" {
		updates["code"] = code
	}
	return tx.Model(&migration.LetterType{}).
		Where("id = ?", id).
		Updates(updates).Error
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

func (r *Repository) DeleteTemplateByLetterTypeID(tx *gorm.DB, letterTypeID uint) error {
	return tx.Where("letter_type_id = ?", letterTypeID).Delete(&migration.LetterTemplate{}).Error
}

func (r *Repository) ListTemplates(tx *gorm.DB) ([]migration.LetterTemplate, error) {
	var templates []migration.LetterTemplate
	err := tx.Preload("LetterType").Preload("Creator").Order("updated_at desc").Find(&templates).Error
	return templates, err
}

func (r *Repository) ListLetterTypes(tx *gorm.DB) ([]migration.LetterType, error) {
	var letterTypes []migration.LetterType
	err := tx.Order("name asc").Find(&letterTypes).Error
	return letterTypes, err
}
