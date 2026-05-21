package push

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"firebase.google.com/go/v4/messaging"
	"github.com/reyimanuel/letter-administration/internal/infrastructures/pkg/fcm"
	"github.com/reyimanuel/letter-administration/internal/migration"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

const maxMulticastTokens = 500

type SendResult struct {
	Tokens  int
	Success int
	Failure int
	Revoked int
}

func IsReady() bool {
	return fcm.Client() != nil
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

	// Best-effort: store notification for in-app history.
	if err := storeNotificationsForUsers(db, []uint{userID}, title, body, data); err != nil {
		log.Printf("push: store notification failed: user_id=%d err=%v", userID, err)
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

	userIDs, err := listUserIDsByRoleCode(db, roleCode)
	if err != nil {
		return nil, err
	}
	if len(userIDs) == 0 {
		return &SendResult{Tokens: 0}, nil
	}

	// Best-effort: store notification for in-app history (one per recipient user).
	if err := storeNotificationsForUsers(db, userIDs, title, body, data); err != nil {
		log.Printf("push: store role notification failed: role=%q err=%v", roleCode, err)
	}

	tokens, err := listActiveTokensByUserIDs(db, userIDs)
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

		batch, err := sendMulticast(ctx, client, chunk, title, body, data)
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
			log.Printf("push: token delivery failed: token=%s err=%v", redactToken(chunk[idx]), r.Error)
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
	return listActiveTokensByUserIDs(db, []uint{userID})
}

func listUserIDsByRoleCode(db *gorm.DB, roleCode string) ([]uint, error) {
	var userIDs []uint
	if err := db.Model(&migration.UserRole{}).
		Joins("JOIN roles ON roles.id = user_roles.role_id").
		Where("roles.code = ?", roleCode).
		Pluck("user_roles.user_id", &userIDs).Error; err != nil {
		return nil, err
	}
	return uniqueUints(userIDs), nil
}

func listActiveTokensByUserIDs(db *gorm.DB, userIDs []uint) ([]string, error) {
	userIDs = uniqueUints(userIDs)
	if len(userIDs) == 0 {
		return []string{}, nil
	}
	var tokens []string
	if err := db.Model(&migration.UserDeviceToken{}).
		Where("user_id IN ?", userIDs).
		Where("(revoked_at = ? OR revoked_at IS NULL)", false).
		Where("token <> ''").
		Distinct().
		Pluck("token", &tokens).Error; err != nil {
		return nil, err
	}
	return uniqueStrings(tokens), nil
}

func storeNotificationsForUsers(db *gorm.DB, userIDs []uint, title string, body string, data map[string]string) error {
	userIDs = uniqueUints(userIDs)
	if len(userIDs) == 0 {
		return nil
	}

	dataJSON := datatypes.JSON([]byte("{}"))
	if data != nil {
		if b, err := json.Marshal(data); err == nil {
			dataJSON = datatypes.JSON(b)
		}
	}

	records := make([]migration.UserNotification, 0, len(userIDs))
	for _, uid := range userIDs {
		records = append(records, migration.UserNotification{
			UserID: uid,
			Title:  title,
			Body:   body,
			Data:   dataJSON,
		})
	}

	// Insert in batches for efficiency.
	return db.CreateInBatches(records, 200).Error
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

func sendMulticast(ctx context.Context, client *messaging.Client, tokens []string, title string, body string, data map[string]string) (*messaging.BatchResponse, error) {
	tag := "sitara-notification"
	if data != nil {
		if typ := strings.TrimSpace(data["type"]); typ != "" {
			tag = "sitara-" + typ
		}
		if letterID := strings.TrimSpace(data["letter_id"]); letterID != "" {
			tag += "-" + letterID
		}
		if studentUserID := strings.TrimSpace(data["student_user_id"]); studentUserID != "" {
			tag += "-" + studentUserID
		}
	}

	payloadData := make(map[string]string, len(data)+3)
	for key, value := range data {
		payloadData[key] = value
	}
	if strings.TrimSpace(payloadData["title"]) == "" {
		payloadData["title"] = title
	}
	if strings.TrimSpace(payloadData["body"]) == "" {
		payloadData["body"] = body
	}
	if strings.TrimSpace(payloadData["tag"]) == "" {
		payloadData["tag"] = tag
	}

	// IMPORTANT:
	// - Data-only messages won't show native Android popups automatically.
	// - Adding the Notification field allows the OS (and FCM SDKs) to display a notification
	//   when the app is backgrounded, without requiring custom client-side handling.
	msg := &messaging.MulticastMessage{
		Tokens: tokens,
		Data:   payloadData,
		Notification: &messaging.Notification{
			Title: title,
			Body:  body,
		},
		Android: &messaging.AndroidConfig{
			Priority: "high",
			Notification: &messaging.AndroidNotification{
				Title: title,
				Body:  body,
				Tag:   tag,
			},
		},
		Webpush: &messaging.WebpushConfig{
			Headers: map[string]string{
				"TTL":     "4500",
				"Urgency": "high",
			},
			Notification: &messaging.WebpushNotification{
				Title: title,
				Body:  body,
				Tag:   tag,
			},
			Data: payloadData,
		},
	}
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
		strings.Contains(msg, "invalid registration") ||
		strings.Contains(msg, "registration token is not a valid")
}

func redactToken(token string) string {
	token = strings.TrimSpace(token)
	if len(token) <= 12 {
		return "***"
	}
	return token[:6] + "..." + token[len(token)-6:]
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

func uniqueUints(in []uint) []uint {
	seen := make(map[uint]struct{}, len(in))
	out := make([]uint, 0, len(in))
	for _, v := range in {
		if v == 0 {
			continue
		}
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
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
