package user

import (
	"log"
	"net/http"

	"github.com/reyimanuel/letter-administration/internal/infrastructures/pkg/errs"
	"github.com/reyimanuel/letter-administration/internal/infrastructures/pkg/helpers"
	"github.com/reyimanuel/letter-administration/internal/infrastructures/pkg/token"
)

type Service struct {
	Repo *repository
}

func NewService(repo *repository) *Service {
	return &Service{Repo: repo}
}

func (s *Service) Login(payload *LoginRequest) (*Response, error) {
	user, err := s.Repo.GetByEmail(payload.Email)
	if err != nil {
		return nil, errs.Unauthorized("Email atau Password Salah")
	}

	if !helpers.CheckPasswordHash(payload.Password, user.Password) {
		return nil, errs.Unauthorized("Email atau Password Salah")
	}

	access, err := token.GenerateToken(user.ID, user.Email, user.RoleSlice())
	if err != nil {
		log.Printf("error saat membuat token: %v", err)
		return nil, errs.InternalServerError("Gagal membuat akses token")
	}

	refresh, err := token.GenerateRefreshToken(user.ID)
	if err != nil {
		log.Printf("error saat membuat refresh token: %v", err)
		return nil, errs.InternalServerError("Gagal membuat refresh token")
	}

	return &Response{
		StatusCode: http.StatusOK,
		Message:    "Login Berhasil",
		Data: TokenResponse{
			AccessToken:  access,
			RefreshToken: refresh,
		},
	}, nil
}
