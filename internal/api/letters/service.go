package letters

import (
	"fmt"
	"mime/multipart"
	"net/http"
	"strings"

	"github.com/reyimanuel/letter-administration/internal/infrastructures/pkg/errs"
	"github.com/reyimanuel/letter-administration/internal/infrastructures/pkg/helpers"
	"github.com/reyimanuel/letter-administration/internal/migration"
	"gorm.io/gorm"
)

type Service struct {
	Repo *Repository
}

func NewService(repo *Repository) *Service {
	return &Service{Repo: repo}
}

func (s *Service) UploadTemplate(adminID uint, letterTypeID uint, file *multipart.FileHeader) (*Response, error) {

	if !strings.HasSuffix(file.Filename, ".docx") {
		return nil, errs.BadRequest("Hanya file .docx yang diperbolehkan")
	}

	path := fmt.Sprintf(
		"storage/templates/%d_%s",
		letterTypeID,
		file.Filename,
	)

	err := s.Repo.WithTx(func(tx *gorm.DB) error {

		// pastikan letter type ada
		if _, err := s.Repo.GetLetterTypeByID(tx, letterTypeID); err != nil {
			return errs.NotFound("Jenis surat tidak ditemukan")
		}

		// simpan file
		if err := helpers.SaveUploadedFile(file, path); err != nil {
			return err
		}

		// upsert template
		return s.Repo.UpsertTemplate(tx, &migration.LetterTemplate{
			LetterTypeID: letterTypeID,
			FilePath:     path,
			FileType:     "docx",
			CreatedBy:    adminID,
		})
	})

	if err != nil {
		return nil, err
	}

	return &Response{
		StatusCode: http.StatusOK,
		Message:    "Template surat berhasil diupload",
	}, nil
}
