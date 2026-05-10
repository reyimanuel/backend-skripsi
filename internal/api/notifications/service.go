package notifications

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/reyimanuel/letter-administration/internal/constants"
	"github.com/reyimanuel/letter-administration/internal/infrastructures/pkg/errs"
	"github.com/reyimanuel/letter-administration/internal/infrastructures/pkg/push"
	"github.com/reyimanuel/letter-administration/internal/migration"
	"gorm.io/gorm"
)

type Service struct {
	Repo *Repository
}

func NewService(repo *Repository) *Service {
	return &Service{Repo: repo}
}

func (s *Service) SendTest(userID uint, req SendTestNotificationRequest) (*Response, error) {
	title := strings.TrimSpace(req.Title)
	body := strings.TrimSpace(req.Body)
	if title == "" || body == "" {
		return nil, errs.BadRequest("Judul dan isi notifikasi wajib diisi")
	}

	ctx, cancel := context.WithTimeout(context.Background(), constants.EmailTimeout)
	defer cancel()

	res, err := push.SendToUser(ctx, s.Repo.DB, userID, title, body, req.Data)
	if err != nil {
		log.Printf("push send test failed: user_id=%d err=%v", userID, err)
		return nil, errs.InternalServerError("Gagal mengirim notifikasi")
	}
	if res.Tokens == 0 {
		return nil, errs.BadRequest("Token notifikasi belum terdaftar")
	}

	return &Response{
		StatusCode: http.StatusOK,
		Message:    "Notifikasi diproses",
		Data: SendResult{
			Tokens:  res.Tokens,
			Success: res.Success,
			Failure: res.Failure,
			Revoked: res.Revoked,
		},
	}, nil
}

func (s *Service) ListRecent(userID uint, days int) (*Response, error) {
	if days < 1 {
		days = 1
	}
	if days > 30 {
		days = 30
	}

	since := time.Now().Add(-time.Duration(days) * 24 * time.Hour)

	var rows []migration.UserNotification
	if err := s.Repo.DB.
		Where("user_id = ?", userID).
		Where("created_at >= ?", since).
		Order("created_at DESC").
		Limit(200).
		Find(&rows).Error; err != nil {
		log.Printf("list notifications failed: user_id=%d err=%v", userID, err)
		return nil, errs.InternalServerError("Gagal mengambil data notifikasi")
	}

	items := make([]NotificationItem, 0, len(rows))
	for _, r := range rows {
		data := map[string]string{}
		if len(r.Data) > 0 {
			_ = json.Unmarshal(r.Data, &data)
		}
		items = append(items, NotificationItem{
			ID:        r.ID,
			Title:     r.Title,
			Body:      r.Body,
			Data:      data,
			IsRead:    r.ReadAt != nil,
			ReadAt:    r.ReadAt,
			CreatedAt: r.CreatedAt,
		})
	}

	return &Response{
		StatusCode: http.StatusOK,
		Message:    "OK",
		Data:       items,
	}, nil
}

func (s *Service) MarkRead(userID uint, notificationID uint) (*Response, error) {
	now := time.Now()

	res := s.Repo.DB.Model(&migration.UserNotification{}).
		Where("id = ? AND user_id = ?", notificationID, userID).
		Updates(map[string]any{
			// Idempotent: keep the first read timestamp if already read.
			"read_at": gorm.Expr("COALESCE(read_at, ?)", now),
		})
	if res.Error != nil {
		log.Printf("mark notification read failed: user_id=%d notification_id=%d err=%v", userID, notificationID, res.Error)
		return nil, errs.InternalServerError("Gagal memperbarui notifikasi")
	}
	if res.RowsAffected == 0 {
		return nil, errs.NotFound("Notifikasi tidak ditemukan")
	}

	return &Response{StatusCode: http.StatusOK, Message: "Notifikasi ditandai sebagai dibaca"}, nil
}
