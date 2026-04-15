package user

import (
	"errors"
	"strings"
	"time"

	"github.com/reyimanuel/letter-administration/internal/migration"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type Repository struct {
	DB *gorm.DB
}

func NewRepository(db *gorm.DB) *Repository {
	return &Repository{DB: db}
}

func (r *Repository) GetByEmail(email string) (*migration.User, error) {
	var user migration.User
	if err := r.DB.Preload("Roles").Preload("Student").Where("email = ?", email).First(&user).Error; err != nil {
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

func (r *Repository) GetActiveOfficialByUserID(tx *gorm.DB, userID uint) (*migration.Official, error) {
	var official migration.Official
	err := tx.Preload("User").
		Where("user_id = ? AND is_active = ?", userID, true).
		First(&official).Error
	if err != nil {
		return nil, err
	}
	return &official, nil
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

func (r *Repository) CreateOfficialWithUser(tx *gorm.DB, user *migration.User, official *migration.Official) error {
	if err := tx.Create(user).Error; err != nil {
		return err
	}

	official.UserID = user.ID
	return tx.Create(official).Error
}

func baseUserQuery(tx *gorm.DB) *gorm.DB {
	return tx.Model(&migration.User{}).
		Preload("Roles").
		Preload("Student", "admin_verification_status = ?", "approved")
}

func (r *Repository) GetPendingStudents(tx *gorm.DB) ([]migration.Student, error) {
	var students []migration.Student
	err := tx.Preload("User").
		Where("admin_verification_status = ?", "pending").
		Order("created_at DESC").
		Find(&students).Error
	return students, err
}

func (r *Repository) UpdateStudentAdminVerification(tx *gorm.DB, studentID uint, status string, adminID *uint, reason string) error {
	updates := map[string]any{
		"admin_verification_status": status,
		"admin_verified_by":         adminID,
		"admin_verified_at":         time.Now(),
		"rejection_reason":          reason,
	}

	return tx.Model(&migration.Student{}).Where("id = ?", studentID).Updates(updates).Error
}

func (r *Repository) SetUserEmailVerified(tx *gorm.DB, userID uint, verifiedAt time.Time) error {
	return tx.Model(&migration.User{}).Where("id = ?", userID).Update("email_verified_at", verifiedAt).Error
}

func (r *Repository) ClearStudentKredensial(tx *gorm.DB, studentID uint) error {
	return tx.Model(&migration.Student{}).Where("id = ?", studentID).Update("kredensial_path", "").Error
}

func (r *Repository) UpdateStudentFields(tx *gorm.DB, studentID uint, updates map[string]any) error {
	if len(updates) == 0 {
		return nil
	}
	return tx.Model(&migration.Student{}).Where("id = ?", studentID).Updates(updates).Error
}

func (r *Repository) UpdateUserFields(tx *gorm.DB, userID uint, updates map[string]any) error {
	if len(updates) == 0 {
		return nil
	}
	return tx.Model(&migration.User{}).Where("id = ?", userID).Updates(updates).Error
}

func (r *Repository) DeleteUser(tx *gorm.DB, userID uint) error {
	return tx.Delete(&migration.User{}, userID).Error
}

func (r *Repository) GetAllUsers(tx *gorm.DB) ([]migration.User, error) {
	var users []migration.User
	err := baseUserQuery(tx).
		Joins("LEFT JOIN students ON students.user_id = users.id").
		Where("students.id IS NULL OR students.admin_verification_status = ?", "approved").
		Order("users.created_at DESC").
		Find(&users).Error
	return users, err
}

func (r *Repository) GetOfficialByUserID(tx *gorm.DB, userID uint) (*migration.Official, error) {
	var official migration.Official
	if err := tx.Preload("User").Where("user_id = ?", userID).First(&official).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}

	return &official, nil
}

func (r *Repository) UpsertUserDeviceToken(tx *gorm.DB, userID uint, token string, platform string, lastSeenAt time.Time) error {
	item := &migration.UserDeviceToken{
		UserID:     userID,
		Token:      token,
		Platform:   platform,
		RevokedAt:  false,
		LastSentAt: &lastSeenAt,
	}

	// Upsert by unique token. If token already exists, re-attach it to this user
	// (e.g. user re-login) and update platform + timestamps.
	return tx.Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "token"}},
		DoUpdates: clause.Assignments(map[string]any{
			"user_id":      userID,
			"platform":     platform,
			"revoked_at":   false,
			"last_sent_at": lastSeenAt,
			"updated_at":   lastSeenAt,
		}),
	}).Create(item).Error
}

func (r *Repository) DeleteUserDeviceToken(tx *gorm.DB, userID uint, token string) error {
	return tx.Where("user_id = ? AND token = ?", userID, token).
		Delete(&migration.UserDeviceToken{}).Error
}

func (r *Repository) ListActiveDeviceTokensByUserID(tx *gorm.DB, userID uint) ([]string, error) {
	var tokens []string
	err := tx.Model(&migration.UserDeviceToken{}).
		Where("user_id = ?", userID).
		Where("revoked_at = ?", false).
		Where("token <> ''").
		Pluck("token", &tokens).Error
	if err != nil {
		return nil, err
	}
	return tokens, nil
}

func (r *Repository) UpdateDeviceTokensLastSentAt(tx *gorm.DB, tokens []string, sentAt time.Time) error {
	if len(tokens) == 0 {
		return nil
	}
	return tx.Model(&migration.UserDeviceToken{}).
		Where("token IN ?", tokens).
		Updates(map[string]any{"last_sent_at": sentAt, "updated_at": sentAt}).Error
}

func (r *Repository) RevokeDeviceTokens(tx *gorm.DB, tokens []string) error {
	if len(tokens) == 0 {
		return nil
	}
	return tx.Model(&migration.UserDeviceToken{}).
		Where("token IN ?", tokens).
		Updates(map[string]any{"revoked_at": true, "updated_at": time.Now()}).Error
}
