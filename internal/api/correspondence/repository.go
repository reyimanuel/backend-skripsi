package correspondence

import (
	"strings"
	"time"

	"github.com/reyimanuel/letter-administration/internal/migration"
	"gorm.io/gorm"
)

type ListLettersParams struct {
	StudentID   *uint
	Query       string
	Status      string
	LetterType  *uint
	CreatedFrom *time.Time
	CreatedTo   *time.Time
	Sort        string
	Page        int
	PageSize    int
}

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

func (r *Repository) CreateAttachment(tx *gorm.DB, att *migration.LetterAttachment) error {
	return tx.Create(att).Error
}

func (r *Repository) GetLetterWithTypeByID(tx *gorm.DB, letterID uint) (*migration.Letter, error) {
	var letter migration.Letter
	if err := tx.Preload("LetterType").Where("id = ?", letterID).First(&letter).Error; err != nil {
		return nil, err
	}
	return &letter, nil
}

func (r *Repository) ListAttachmentKeysByLetterID(tx *gorm.DB, letterID uint) ([]string, error) {
	var keys []string
	if err := tx.Model(&migration.LetterAttachment{}).
		Where("letter_id = ?", letterID).
		Where("requirement_key <> ''").
		Distinct().
		Pluck("requirement_key", &keys).Error; err != nil {
		return nil, err
	}
	return keys, nil
}

func (r *Repository) SaveLetter(tx *gorm.DB, letter *migration.Letter) error {
	return tx.Save(letter).Error
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

func (r *Repository) ListHistoriesByLetterID(tx *gorm.DB, letterID uint) ([]migration.LetterHistory, error) {
	var histories []migration.LetterHistory
	err := tx.
		Preload("Actor").
		Where("letter_id = ?", letterID).
		Order("created_at asc").
		Find(&histories).Error
	return histories, err
}

func (r *Repository) ListLetters(tx *gorm.DB, p ListLettersParams) ([]migration.Letter, int64, error) {
	query := tx.Model(&migration.Letter{}).
		Preload("LetterType").
		Preload("Student").
		Preload("Student.User").
		Joins("LEFT JOIN students ON students.id = letters.student_id").
		Joins("LEFT JOIN users ON users.id = students.user_id").
		Joins("LEFT JOIN letter_types ON letter_types.id = letters.letter_type_id")

	if p.StudentID != nil {
		query = query.Where("letters.student_id = ?", *p.StudentID)
	}
	if strings.TrimSpace(p.Status) != "" {
		query = query.Where("letters.status = ?", p.Status)
	}
	if p.LetterType != nil {
		query = query.Where("letters.letter_type_id = ?", *p.LetterType)
	}
	if p.CreatedFrom != nil {
		query = query.Where("letters.created_at >= ?", *p.CreatedFrom)
	}
	if p.CreatedTo != nil {
		query = query.Where("letters.created_at <= ?", *p.CreatedTo)
	}

	if q := strings.TrimSpace(p.Query); q != "" {
		like := "%" + strings.ToLower(q) + "%"
		query = query.Where(
			"LOWER(letters.subject) LIKE ? OR LOWER(COALESCE(letters.letter_number, '')) LIKE ? OR LOWER(users.name) LIKE ? OR LOWER(COALESCE(students.nim, '')) LIKE ?",
			like, like, like, like,
		)
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	sort := strings.TrimSpace(p.Sort)
	if sort == "created_at_asc" {
		query = query.Order("letters.created_at asc")
	} else {
		query = query.Order("letters.created_at desc")
	}

	page := p.Page
	if page <= 0 {
		page = 1
	}
	pageSize := p.PageSize
	if pageSize <= 0 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}
	offset := (page - 1) * pageSize

	var letters []migration.Letter
	if err := query.Limit(pageSize).Offset(offset).Find(&letters).Error; err != nil {
		return nil, 0, err
	}

	return letters, total, nil
}
