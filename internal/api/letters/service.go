package letters

import (
	"errors"
	"fmt"
	"log"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5/pgconn"
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

func (s *Service) UploadTemplateFlexible(adminID uint, req UploadTemplateFlexibleRequest, file *multipart.FileHeader) (*Response, error) {
	if file == nil {
		return nil, errs.BadRequest("file tidak ditemukan")
	}
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
		_ = os.Remove(newPath)
		return nil, errs.BadRequest("file docx tidak valid")
	}

	var oldPath string
	var resolvedLetterTypeID uint

	err = s.Repo.WithTx(func(tx *gorm.DB) error {
		letterTypeIDStr := strings.TrimSpace(req.LetterTypeID)
		if letterTypeIDStr != "" {
			parsed, err := strconv.ParseUint(letterTypeIDStr, 10, 64)
			if err != nil {
				return errs.BadRequest("letter_type_id tidak valid")
			}
			resolvedLetterTypeID = uint(parsed)
			if _, err := s.Repo.GetLetterTypeByID(tx, resolvedLetterTypeID); err != nil {
				return errs.NotFound("Jenis surat tidak ditemukan")
			}
		} else {
			code := strings.TrimSpace(req.Code)
			name := strings.TrimSpace(req.Name)
			desc := strings.TrimSpace(req.Description)
			if code == "" || name == "" {
				return errs.BadRequest("code dan name wajib diisi jika letter_type_id tidak ada")
			}
			code = strings.ToUpper(strings.ReplaceAll(code, " ", "_"))

			lt := &migration.LetterType{Code: code, Name: name, Description: desc}
			if err := s.Repo.CreateLetterType(tx, lt); err != nil {
				var pgErr *pgconn.PgError
				if errors.As(err, &pgErr) && pgErr.Code == "23505" {
					return errs.BadRequest("kode tipe surat sudah terdaftar")
				}
				return err
			}
			resolvedLetterTypeID = lt.ID
		}

		existing, _ := s.Repo.GetTemplateByLetterTypeID(tx, resolvedLetterTypeID)
		if existing != nil {
			oldPath = existing.FilePath
		}

		return s.Repo.UpsertTemplate(tx, &migration.LetterTemplate{
			LetterTypeID: resolvedLetterTypeID,
			FilePath:     newPath,
			FileType:     "docx",
			CreatedBy:    adminID,
		})
	})
	if err != nil {
		_ = os.Remove(newPath)
		return nil, err
	}

	helpers.RemoveOldFile(oldPath, newPath)

	return &Response{
		StatusCode: http.StatusCreated,
		Message:    "Template surat berhasil diupload",
		Data: map[string]any{
			"letter_type_id": resolvedLetterTypeID,
			"file_path":      helpers.ToAbsoluteURL(newPath),
		},
	}, nil
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

	pdfPath, err := helpers.EnsurePDFPreview(docxPath)
	if err != nil {
		fmt.Printf("gagal siapkan preview pdf: %v\n", err)
		return "", errs.InternalServerError("Gagal convert PDF")
	}

	return pdfPath, nil
}

func (s *Service) DeleteTemplate(letterTypeID uint) (*Response, error) {
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
		return s.Repo.DeleteTemplateByLetterTypeID(tx, letterTypeID)
	})
	if err != nil {
		return nil, err
	}

	// Best-effort file cleanup after transaction commit.
	pathsToRemove := make([]string, 0, 2)
	if strings.TrimSpace(docxPath) != "" {
		pathsToRemove = append(pathsToRemove, docxPath)
		pathsToRemove = append(pathsToRemove, strings.TrimSuffix(docxPath, filepath.Ext(docxPath))+".pdf")
	}

	for _, p := range pathsToRemove {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if err := os.Remove(p); err != nil && !os.IsNotExist(err) {
			log.Printf("gagal menghapus file %q: %v", p, err)
		}
	}

	return &Response{StatusCode: http.StatusOK, Message: "Template surat berhasil dihapus"}, nil
}

func (s *Service) GetAllTemplates() (*Response, error) {
	var items []TemplateListItem

	err := s.Repo.WithTx(func(tx *gorm.DB) error {
		templates, err := s.Repo.ListTemplates(tx)
		if err != nil {
			return err
		}

		items = make([]TemplateListItem, 0, len(templates))
		for _, t := range templates {
			items = append(items, TemplateListItem{
				ID:           t.ID,
				LetterTypeID: t.LetterTypeID,
				Code:         t.LetterType.Code,
				Name:         t.LetterType.Name,
				Description:  t.LetterType.Description,
				FilePath:     helpers.ToAbsoluteURL(t.FilePath),
				FileType:     t.FileType,
				CreatedBy:    t.CreatedBy,
				CreatedAt:    t.CreatedAt,
				UpdatedAt:    t.UpdatedAt,
			})
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	return &Response{StatusCode: http.StatusOK, Message: "Berhasil mengambil data template", Data: items}, nil
}
