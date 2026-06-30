package user

import (
	"errors"
	"time"

	"github.com/reyimanuel/letter-administration/internal/constants"
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

func (r *Repository) GetActiveAtasanByUserID(tx *gorm.DB, userID uint) (*migration.Atasan, error) {
	var atasan migration.Atasan
	err := tx.Preload("User").Preload("User.Roles").
		Where("user_id = ? AND is_active = ?", userID, true).
		First(&atasan).Error
	if err != nil {
		return nil, err
	}
	return &atasan, nil
}

func (r *Repository) GetActiveAtasanByID(tx *gorm.DB, atasanID uint) (*migration.Atasan, error) {
	var atasan migration.Atasan
	err := tx.Preload("User").Preload("User.Roles").
		Where("id = ? AND is_active = ?", atasanID, true).
		First(&atasan).Error
	if err != nil {
		return nil, err
	}
	return &atasan, nil
}

func (r *Repository) ListActiveAtasan(tx *gorm.DB) ([]migration.Atasan, error) {
	var atasan []migration.Atasan
	err := tx.Model(&migration.Atasan{}).
		Preload("User").
		Preload("User.Roles").
		Joins("JOIN users ON users.id = atasan.user_id").
		Joins("JOIN user_roles ON user_roles.user_id = users.id").
		Joins("JOIN roles ON roles.id = user_roles.role_id").
		Where("atasan.is_active = ? AND users.is_active = ? AND roles.code IN ?", true, true, constants.AtasanRoleCodes).
		Order("atasan.jabatan ASC, users.name ASC").
		Find(&atasan).Error
	return atasan, err
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

func baseUserQuery(tx *gorm.DB) *gorm.DB {
	return tx.Model(&migration.User{}).
		Preload("Roles").
		Preload("Student", "admin_verification_status = ?", "approved")
}

func (r *Repository) GetPendingStudents(tx *gorm.DB, page, pageSize int) ([]migration.Student, int64, error) {
	var students []migration.Student
	var total int64

	query := tx.Model(&migration.Student{}).Where("admin_verification_status = ?", "pending")

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	err := query.Preload("User").
		Order("created_at DESC").
		Limit(pageSize).
		Offset((page - 1) * pageSize).
		Find(&students).Error
	return students, total, err
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
	return tx.Transaction(func(txTx *gorm.DB) error {
		// Clean up internal associations
		if err := txTx.Where("user_id = ?", userID).Delete(&migration.UserRole{}).Error; err != nil {
			return err
		}
		if err := txTx.Where("user_id = ?", userID).Delete(&migration.UserDeviceToken{}).Error; err != nil {
			return err
		}
		if err := txTx.Where("user_id = ?", userID).Delete(&migration.UserNotification{}).Error; err != nil {
			return err
		}

		// Try deleting Atasan and Student profiles.
		// If these have related data (like Letters), it will fail with 23503, which is expected.
		if err := txTx.Where("user_id = ?", userID).Delete(&migration.Atasan{}).Error; err != nil {
			return err
		}
		if err := txTx.Where("user_id = ?", userID).Delete(&migration.Student{}).Error; err != nil {
			return err
		}

		return txTx.Delete(&migration.User{}, userID).Error
	})
}

func (r *Repository) GetAllUsers(tx *gorm.DB, page, pageSize int) ([]migration.User, int64, error) {
	var users []migration.User
	var total int64

	query := baseUserQuery(tx)

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	err := query.
		Order("users.created_at DESC").
		Limit(pageSize).
		Offset((page - 1) * pageSize).
		Find(&users).Error
	return users, total, err
}

func (r *Repository) GetAtasanByUserID(tx *gorm.DB, userID uint) (*migration.Atasan, error) {
	var atasan migration.Atasan
	if err := tx.Preload("User").Where("user_id = ?", userID).First(&atasan).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}

	return &atasan, nil
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
		Where("(revoked_at = ? OR revoked_at IS NULL)", false).
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
	return tx.Model(&migration.UserDeviceToken{}).
		Where("token IN ?", tokens).
		Updates(map[string]any{"revoked_at": true, "updated_at": time.Now()}).Error
}

// CountAdminsWithRole returns the count of users with the specified role
func (r *Repository) CountAdminsWithRole(role string) (int64, error) {
	var count int64
	err := r.DB.Model(&migration.User{}).
		Joins("JOIN user_roles ON user_roles.user_id = users.id").
		Joins("JOIN roles ON roles.id = user_roles.role_id").
		Where("roles.code = ?", role).
		Count(&count).Error
	return count, err
}
