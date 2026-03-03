package user

import (
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/gocolly/colly"
	"gorm.io/gorm"

	"github.com/reyimanuel/letter-administration/internal/infrastructures/pkg/errs"
	"github.com/reyimanuel/letter-administration/internal/infrastructures/pkg/helpers"
	"github.com/reyimanuel/letter-administration/internal/infrastructures/pkg/token"
	"github.com/reyimanuel/letter-administration/internal/migration"
)

type Service struct {
	Repo *Repository
}

func NewService(repo *Repository) *Service {
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

func (s *Service) RegisterStudent(nim, email, password string) (*Response, error) {
	if _, err := s.Repo.GetByEmail(email); err == nil {
		return nil, errs.BadRequest("email sudah terdaftar")
	}

	if _, err := s.Repo.GetByNIM(nim); err == nil {
		return nil, errs.BadRequest("nim sudah terdaftar")
	}

	name, err := s.getNameByNIM(nim)
	if err != nil {
		return nil, errs.BadRequest("nim tidak ditemukan di pddikti")
	}

	if !helpers.MatchNameWithEmail(name, email) {
		return nil, errs.BadRequest("email tidak sesuai dengan nama mahasiswa")
	}

	err = s.Repo.DB.Transaction(func(tx *gorm.DB) error {

		role, err := s.Repo.GetRoleByCode(tx, "STUDENT")
		if err != nil {
			return err
		}

		user := &migration.User{
			Name:     name,
			Email:    email,
			Password: password,
			Roles:    []migration.Role{*role},
		}

		student := &migration.Student{
			NIM: nim,
		}

		return s.Repo.CreateStudentWithUser(tx, user, student)
	})

	if err != nil {
		return nil, err
	}

	return &Response{
		StatusCode: http.StatusOK,
		Message:    "Registrasi Berhasil",
	}, nil
}

func (s *Service) getNameByNIM(nim string) (string, error) {
	c := colly.NewCollector(
		colly.UserAgent("Mozilla/5.0"),
	)

	c.SetRequestTimeout(10 * time.Second)

	var name string

	c.OnHTML(".nama-mahasiswa", func(e *colly.HTMLElement) {
		name = strings.TrimSpace(e.Text)
	})

	url := fmt.Sprintf("https://pddikti.kemdiktisaintek.go.id/search/%s", nim)

	if err := c.Visit(url); err != nil {
		return "", err
	}

	if name == "" {
		return "", errors.New("data mahasiswa tidak ditemukan")
	}

	return name, nil
}
