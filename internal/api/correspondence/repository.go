package correspondence

import (
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

func (r *Repository) WithTx(fn func(tx *gorm.DB) error) error {
	return r.DB.Transaction(fn)
}

func (r *Repository) CreateLetter(tx *gorm.DB, letter *migration.Letter) error {
	if err := tx.Create(letter).Error; err != nil {
		return err
	}
	return nil
}

func (r *Repository) CreateHistory(tx *gorm.DB, history *migration.LetterHistory) error {
	if err := tx.Create(history).Error; err != nil {
		return err
	}
	return nil
}

func (r *Repository) CreateApproval(tx *gorm.DB, approval *migration.LetterApproval) error {
	if err := tx.Create(approval).Error; err != nil {
		return err
	}
	return nil
}

func (r *Repository) GetApprovalByLetterID(tx *gorm.DB, letterID uint) (*migration.LetterApproval, error) {
	var approval migration.LetterApproval
	if err := tx.Where("letter_id = ?", letterID).First(&approval).Error; err != nil {
		return nil, err
	}
	return &approval, nil
}

func (r *Repository) SaveApproval(tx *gorm.DB, approval *migration.LetterApproval) error {
	return tx.Save(approval).Error
}

func (r *Repository) GetTemplateByLetterType(tx *gorm.DB, letterTypeID uint) (*migration.LetterTemplate, error) {
	var t migration.LetterTemplate

	if err := tx.
		Where("letter_type_id = ?", letterTypeID).
		First(&t).Error; err != nil {
		return nil, err
	}

	return &t, nil
}

func (r *Repository) CountApprovedThisYear(tx *gorm.DB) (int64, error) {
	var count int64

	now := time.Now()
	startOfYear := time.Date(now.Year(), 1, 1, 0, 0, 0, 0, time.UTC)
	endOfYear := startOfYear.AddDate(1, 0, 0)

	err := tx.Model(&migration.Letter{}).
		Where("status = ?", "approved").
		Where("signed_at >= ? AND signed_at < ?", startOfYear, endOfYear).
		Count(&count).Error

	return count, err
}

func (r *Repository) GetAttachmentsByLetterID(tx *gorm.DB, letterID uint) ([]migration.LetterAttachment, error) {
	var atts []migration.LetterAttachment
	err := tx.Where("letter_id = ?", letterID).Find(&atts).Error
	return atts, err
}

func (r *Repository) DeleteAttachmentsByLetterID(tx *gorm.DB, letterID uint) error {
	return tx.Where("letter_id = ?", letterID).Delete(&migration.LetterAttachment{}).Error
}

func (r *Repository) DeleteApprovalsByLetterID(tx *gorm.DB, letterID uint) error {
	return tx.Where("letter_id = ?", letterID).Delete(&migration.LetterApproval{}).Error
}

func (r *Repository) DeleteHistoriesByLetterID(tx *gorm.DB, letterID uint) error {
	return tx.Where("letter_id = ?", letterID).Delete(&migration.LetterHistory{}).Error
}

func (r *Repository) DeleteLetterByID(tx *gorm.DB, letterID uint) error {
	return tx.Delete(&migration.Letter{}, letterID).Error
}
