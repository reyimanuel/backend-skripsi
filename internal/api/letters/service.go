package letters

import (
	"fmt"
	"mime/multipart"
	"net/http"
	"path/filepath"

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
	if filepath.Ext(file.Filename) != ".docx" {
		return nil, errs.BadRequest("hanya file .docx yang diperbolehkan")
	}

	mime, err := helpers.DetectMimeType(file)
	if err != nil {
		return nil, errs.InternalServerError("gagal membaca file")
	}

	allowedMimes := map[string]bool{
		"application/vnd.openxmlformats-officedocument.wordprocessingml.document": true,
		"application/zip": true,
	}
	if !allowedMimes[mime] {
		fmt.Printf("mime type tidak valid: %s\n", mime)
		return nil, errs.BadRequest("file docx tidak valid")
	}

	newFileName := helpers.GenerateUniqueFileName(file.Filename)
	newPath := filepath.Join("public", "letter-template", newFileName)

	var oldPath string

	err = s.Repo.WithTx(func(tx *gorm.DB) error {

		if _, err := s.Repo.GetLetterTypeByID(tx, letterTypeID); err != nil {
			return errs.NotFound("Jenis surat tidak ditemukan")
		}

		existing, _ := s.Repo.GetTemplateByLetterTypeID(tx, letterTypeID)
		if existing != nil {
			oldPath = existing.FilePath
		}

		if err := helpers.SaveUploadedFile(file, newPath); err != nil {
			return err
		}

		return s.Repo.UpsertTemplate(tx, &migration.LetterTemplate{
			LetterTypeID: letterTypeID,
			FilePath:     newPath,
			FileType:     "docx",
			CreatedBy:    adminID,
		})
	})

	if err != nil {
		return nil, err
	}

	helpers.RemoveOldFile(oldPath, newPath)

	return &Response{
		StatusCode: http.StatusCreated,
		Message:    "Template surat berhasil diupload",
	}, nil
}
