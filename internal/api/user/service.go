package user

import (
	"log"
	"net/http"

	"github.com/reyimanuel/letter-administration/internal/infrastructures/pkg/errs"
	"github.com/reyimanuel/letter-administration/internal/infrastructures/pkg/token"
	"golang.org/x/crypto/bcrypt"
)

type Service struct {
	Repo *repository
}

func NewService(repo *repository) *Service {
	return &Service{Repo: repo}
}

func (s *Service) Login(payload *LoginRequest) (*LoginResponse, error) {
	user, err := s.Repo.GetByEmail(payload.Email)
	if err != nil {
		return nil, errs.Unauthorized("Email atau Password Salah")
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(payload.Password)); err != nil {
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

	return &LoginResponse{
		StatusCode:   http.StatusOK,
		AccessToken:  access,
		RefreshToken: refresh,
	}, nil
}
