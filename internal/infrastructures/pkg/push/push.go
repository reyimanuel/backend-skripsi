package push

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"firebase.google.com/go/v4/messaging"
	"github.com/reyimanuel/letter-administration/internal/infrastructures/pkg/fcm"
	"github.com/reyimanuel/letter-administration/internal/migration"
	"gorm.io/gorm"
)

const maxMulticastTokens = 500

type SendResult struct {
	Tokens  int
	Success int
	Failure int
	Revoked int
}

// SendToUser sends a notification to all active tokens of a user.
// Best practice: call this outside of your business transaction.
func SendToUser(ctx context.Context, db *gorm.DB, userID uint, title string, body string, data map[string]string) (*SendResult, error) {
	if db == nil {
		return nil, errors.New("db is nil")
	}

	title = strings.TrimSpace(title)
	body = strings.TrimSpace(body)
	if title == "" || body == "" {
		return nil, errors.New("title/body is empty")
	}

	tokens, err := listActiveTokensByUserID(db, userID)
	if err != nil {
		return nil, err
	}
	if len(tokens) == 0 {
		return &SendResult{Tokens: 0}, nil
	}

	return sendToTokens(ctx, db, tokens, title, body, data)
}

// SendToRole sends a notification to all active tokens of users that have the given role code.
func SendToRole(ctx context.Context, db *gorm.DB, roleCode string, title string, body string, data map[string]string) (*SendResult, error) {
	if db == nil {
		return nil, errors.New("db is nil")
	}

	roleCode = strings.TrimSpace(roleCode)
	if roleCode == "" {
		return nil, errors.New("roleCode is empty")
	}

	tokens, err := listActiveTokensByRoleCode(db, roleCode)
	if err != nil {
		return nil, err
	}
	if len(tokens) == 0 {
		return &SendResult{Tokens: 0}, nil
	}

	return sendToTokens(ctx, db, tokens, title, body, data)
}

func sendToTokens(ctx context.Context, db *gorm.DB, tokens []string, title string, body string, data map[string]string) (*SendResult, error) {
	client := fcm.Client()
	if client == nil {
		return nil, errors.New("fcm client is nil (not initialized)")
	}

	if ctx == nil {
		ctx = context.Background()
	}

	sentAt := time.Now()
	successTokens := make([]string, 0)
	revokedTokens := make([]string, 0)
	var successCount, failureCount int

	for start := 0; start < len(tokens); start += maxMulticastTokens {
		end := start + maxMulticastTokens
		if end > len(tokens) {
			end = len(tokens)
		}
		chunk := tokens[start:end]

		batch, err := sendMulticast(ctx, client, chunk, &messaging.Notification{Title: title, Body: body}, data)
		if err != nil {
			return nil, err
		}

		for idx, r := range batch.Responses {
			if r.Success {
				successCount++
				successTokens = append(successTokens, chunk[idx])
				continue
			}
			failureCount++
			if isUnregisteredTokenError(r.Error) {
				revokedTokens = append(revokedTokens, chunk[idx])
			}
		}
	}

	if err := db.Transaction(func(tx *gorm.DB) error {
		if err := updateTokensLastSentAt(tx, successTokens, sentAt); err != nil {
			return err
		}
		if err := revokeTokens(tx, revokedTokens, sentAt); err != nil {
			return err
		}
		return nil
	}); err != nil {
		// Notification already sent; keep it as a soft error.
		log.Printf("push: token status update failed: %v", err)
	}

	return &SendResult{Tokens: len(tokens), Success: successCount, Failure: failureCount, Revoked: len(revokedTokens)}, nil
}

func listActiveTokensByUserID(db *gorm.DB, userID uint) ([]string, error) {
	var tokens []string
	err := db.Model(&migration.UserDeviceToken{}).
		Where("user_id = ?", userID).
		Where("revoked_at = ?", false).
		Where("token <> ''").
		Pluck("token", &tokens).Error
	if err != nil {
		return nil, err
	}
	return uniqueStrings(tokens), nil
}

func listActiveTokensByRoleCode(db *gorm.DB, roleCode string) ([]string, error) {
	var userIDs []uint
	if err := db.Model(&migration.UserRole{}).
		Joins("JOIN roles ON roles.id = user_roles.role_id").
		Where("roles.code = ?", roleCode).
		Pluck("user_roles.user_id", &userIDs).Error; err != nil {
		return nil, err
	}
	if len(userIDs) == 0 {
		return []string{}, nil
	}

	var tokens []string
	if err := db.Model(&migration.UserDeviceToken{}).
		Where("user_id IN ?", userIDs).
		Where("revoked_at = ?", false).
		Where("token <> ''").
		Distinct().
		Pluck("token", &tokens).Error; err != nil {
		return nil, err
	}
	return uniqueStrings(tokens), nil
}

func updateTokensLastSentAt(tx *gorm.DB, tokens []string, sentAt time.Time) error {
	if len(tokens) == 0 {
		return nil
	}
	return tx.Model(&migration.UserDeviceToken{}).
		Where("token IN ?", tokens).
		Updates(map[string]any{"last_sent_at": sentAt, "updated_at": sentAt}).Error
}

func revokeTokens(tx *gorm.DB, tokens []string, revokedAt time.Time) error {
	if len(tokens) == 0 {
		return nil
	}
	return tx.Model(&migration.UserDeviceToken{}).
		Where("token IN ?", tokens).
		Updates(map[string]any{"revoked_at": true, "updated_at": revokedAt}).Error
}

func sendMulticast(ctx context.Context, client *messaging.Client, tokens []string, notification *messaging.Notification, data map[string]string) (*messaging.BatchResponse, error) {
	msg := &messaging.MulticastMessage{Tokens: tokens, Notification: notification, Data: data}
	if resp, err := client.SendEachForMulticast(ctx, msg); err == nil {
		return resp, nil
	}
	return client.SendMulticast(ctx, msg)
}

func isUnregisteredTokenError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "not registered") ||
		strings.Contains(msg, "registration-token-not-registered") ||
		strings.Contains(msg, "unregistered") ||
		strings.Contains(msg, "invalid registration")
}

func uniqueStrings(in []string) []string {
	seen := make(map[string]struct{}, len(in))
	out := make([]string, 0, len(in))
	for _, s := range in {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	return out
}

func FormatAdminLetterBody(studentName string, subject string) string {
	studentName = strings.TrimSpace(studentName)
	subject = strings.TrimSpace(subject)
	if studentName == "" && subject == "" {
		return "Ada surat baru disubmit"
	}
	if studentName == "" {
		return fmt.Sprintf("Surat baru disubmit: %s", subject)
	}
	if subject == "" {
		return fmt.Sprintf("%s mengirim surat baru", studentName)
	}
	return fmt.Sprintf("%s mengirim surat: %s", studentName, subject)
}
