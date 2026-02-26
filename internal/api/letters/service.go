package letters

import (
	"fmt"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
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
	if filepath.Ext(file.Filename) != ".docx" {
		return nil, errs.BadRequest("hanya file .docx yang diperbolehkan")
	}

	newFileName := helpers.GenerateUniqueFileName(file.Filename)
	newPath := filepath.Join("public", "letter-template", newFileName)

	if err := helpers.SaveUploadedFile(file, newPath); err != nil {
		fmt.Printf("encountered error when saving file")
		return nil, errs.InternalServerError("gagal menyimpan file")
	}

	mime, err := helpers.DetectMimeTypeFromPath(newPath)
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

	var oldPath string

	fmt.Println("Generated filename:", newFileName)
	fmt.Println("Generated path:", newPath)

	err = s.Repo.WithTx(func(tx *gorm.DB) error {

		if _, err := s.Repo.GetLetterTypeByID(tx, letterTypeID); err != nil {
			return errs.NotFound("Jenis surat tidak ditemukan")
		}

		existing, _ := s.Repo.GetTemplateByLetterTypeID(tx, letterTypeID)
		if existing != nil {
			oldPath = existing.FilePath
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

func (s *Service) PreviewTemplate(letterTypeID uint) (string, error) {

	var docxPath string

	err := s.Repo.WithTx(func(tx *gorm.DB) error {

		template, err := s.Repo.GetTemplateByLetterTypeID(tx, letterTypeID)
		if err != nil {
			return err
		}
		if template == nil {
			return errs.NotFound("Template tidak ditemukan")
		}

		docxPath = template.FilePath
		return nil
	})

	if err != nil {
		return "", err
	}

	pdfPath := strings.TrimSuffix(docxPath, filepath.Ext(docxPath)) + ".pdf"

	// cek apakah pdf sudah ada
	docxStat, err := os.Stat(docxPath)
	if err != nil {
		return "", err
	}

	pdfStat, err := os.Stat(pdfPath)

	// convert jika belum ada atau docx lebih baru
	if os.IsNotExist(err) || docxStat.ModTime().After(pdfStat.ModTime()) {

		convertedPath, err := helpers.ConvertDocxToPDF(docxPath)
		if err != nil {
			fmt.Printf("gagal convert docx ke pdf: %v\n", err)
			return "", errs.InternalServerError("Gagal convert PDF")
		}

		pdfPath = convertedPath
	}

	return pdfPath, nil
}
