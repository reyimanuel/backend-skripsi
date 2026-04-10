package notifications

import (
	"context"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/reyimanuel/letter-administration/internal/infrastructures/pkg/errs"
	"github.com/reyimanuel/letter-administration/internal/infrastructures/pkg/push"
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

	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
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
