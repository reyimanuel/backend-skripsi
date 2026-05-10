package letters

import (
	"encoding/json"
	"errors"
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
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

type Service struct {
	Repo *Repository
}

func NewService(repo *Repository) *Service {
	return &Service{Repo: repo}
}

func (s *Service) GetAttachmentRequirements(letterTypeID uint) (*Response, error) {
	var out LetterTypeRequirementsResponse
	err := s.Repo.WithTx(func(tx *gorm.DB) error {
		lt, err := s.Repo.GetLetterTypeByID(tx, letterTypeID)
		if err != nil {
			return errs.NotFound("Jenis surat tidak ditemukan")
		}

		out = LetterTypeRequirementsResponse{LetterTypeID: lt.ID, Code: lt.Code, Name: lt.Name}
		if len(lt.AttachmentRequirements) == 0 {
			out.Requirements = []AttachmentRequirement{}
			return nil
		}

		var reqs []AttachmentRequirement
		if err := json.Unmarshal(lt.AttachmentRequirements, &reqs); err != nil {
			log.Printf("invalid attachment requirements JSON for letter_type_id=%d: %v", lt.ID, err)
			out.Requirements = []AttachmentRequirement{}
			return nil
		}
		out.Requirements = reqs
		return nil
	})
	if err != nil {
		return nil, err
	}

	return &Response{StatusCode: http.StatusOK, Message: "OK", Data: out}, nil
}

func (s *Service) UpdateAttachmentRequirements(letterTypeID uint, req UpdateAttachmentRequirementsRequest) (*Response, error) {
	// minimal validation to keep schema predictable
	seen := make(map[string]struct{}, len(req.Requirements))
	for _, r := range req.Requirements {
		key := strings.TrimSpace(r.Key)
		label := strings.TrimSpace(r.Label)
		if key == "" || label == "" {
			return nil, errs.BadRequest("setiap requirement wajib punya key dan label")
		}
		if _, ok := seen[key]; ok {
			return nil, errs.BadRequest("key requirement tidak boleh duplikat")
		}
		seen[key] = struct{}{}
	}

	payload, err := json.Marshal(req.Requirements)
	if err != nil {
		return nil, errs.InternalServerError("gagal menyimpan requirements")
	}

	err = s.Repo.WithTx(func(tx *gorm.DB) error {
		if _, err := s.Repo.GetLetterTypeByID(tx, letterTypeID); err != nil {
			return errs.NotFound("Jenis surat tidak ditemukan")
		}
		return s.Repo.UpdateLetterTypeAttachmentRequirements(tx, letterTypeID, datatypes.JSON(payload))
	})
	if err != nil {
		return nil, err
	}

	return &Response{StatusCode: http.StatusOK, Message: "Requirements berhasil diupdate"}, nil
}

// UploadTemplateV2 uploads a .docx template, detects {{placeholders}} inside the DOCX,
// persists them, and returns the required payload keys to help clients build forms.
func (s *Service) UploadTemplateV2(adminID uint, req UploadTemplateV2Request, file *multipart.FileHeader) (*Response, error) {
	if file == nil {
		return nil, errs.BadRequest("file tidak ditemukan")
	}
	if filepath.Ext(file.Filename) != ".docx" {
		return nil, errs.BadRequest("hanya file .docx yang diperbolehkan")
	}

	newFileName := helpers.GenerateUniqueFileName(file.Filename)
	newPath := filepath.Join("public", "letter-template", newFileName)

	if err := helpers.SaveUploadedFile(file, newPath); err != nil {
		log.Printf("failed saving uploaded template file: path=%q err=%v", newPath, err)
		return nil, errs.InternalServerError("gagal menyimpan file")
	}

	mime, err := helpers.DetectMimeTypeFromPath(newPath)
	if err != nil {
		_ = os.Remove(newPath)
		return nil, errs.InternalServerError("gagal membaca file")
	}

	allowedMimes := map[string]bool{
		"application/vnd.openxmlformats-officedocument.wordprocessingml.document": true,
		"application/zip": true,
	}
	if !allowedMimes[mime] {
		log.Printf("invalid mime type for uploaded template: mime=%q", mime)
		_ = os.Remove(newPath)
		return nil, errs.BadRequest("file docx tidak valid")
	}

	analysis, err := helpers.AnalyzeDocxTemplatePlaceholders(newPath)
	if err != nil {
		_ = os.Remove(newPath)
		log.Printf("failed analyzing template placeholders: path=%q err=%v", newPath, err)
		return nil, errs.BadRequest("gagal membaca placeholder pada template")
	}
	placeholdersJSON, err := json.Marshal(analysis.Placeholders)
	if err != nil {
		_ = os.Remove(newPath)
		return nil, errs.InternalServerError("gagal memproses placeholder template")
	}

	var oldPath string
	var resolvedLetterTypeID uint
	var resolvedLetterType migration.LetterType

	err = s.Repo.WithTx(func(tx *gorm.DB) error {
		letterTypeIDStr := strings.TrimSpace(req.LetterTypeID)
		if letterTypeIDStr != "" {
			parsed, err := strconv.ParseUint(letterTypeIDStr, 10, 64)
			if err != nil {
				return errs.BadRequest("letter_type_id tidak valid")
			}
			resolvedLetterTypeID = uint(parsed)
			lt, err := s.Repo.GetLetterTypeByID(tx, resolvedLetterTypeID)
			if err != nil {
				return errs.NotFound("Jenis surat tidak ditemukan")
			}
			resolvedLetterType = *lt
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
			resolvedLetterType = *lt
		}

		existing, _ := s.Repo.GetTemplateByLetterTypeID(tx, resolvedLetterTypeID)
		if existing != nil {
			oldPath = existing.FilePath
		}

		if err := s.Repo.UpsertTemplate(tx, &migration.LetterTemplate{
			LetterTypeID: resolvedLetterTypeID,
			FilePath:     newPath,
			FileType:     "docx",
			Placeholders: datatypes.JSON(placeholdersJSON),
			CreatedBy:    adminID,
		}); err != nil {
			return err
		}

		// Remove old file inside the transaction to prevent race conditions
		if oldPath != "" {
			helpers.RemoveOldFile(oldPath, newPath)
		}

		return nil
	})
	if err != nil {
		_ = os.Remove(newPath)
		return nil, err
	}

	// Ensure slices aren't nil in JSON.
	if analysis.Placeholders == nil {
		analysis.Placeholders = []string{}
	}
	if analysis.AutoFilledKeys == nil {
		analysis.AutoFilledKeys = []string{}
	}
	if analysis.RequiredPayloadKeys == nil {
		analysis.RequiredPayloadKeys = []string{}
	}

	return &Response{
		StatusCode: http.StatusCreated,
		Message:    "Template surat berhasil diupload",
		Data: TemplateUploadV2Data{
			LetterTypeID:        resolvedLetterTypeID,
			Code:                resolvedLetterType.Code,
			Name:                resolvedLetterType.Name,
			Description:         resolvedLetterType.Description,
			FilePath:            helpers.ToAbsoluteURL(newPath),
			Placeholders:        analysis.Placeholders,
			AutoFilledKeys:      analysis.AutoFilledKeys,
			RequiredPayloadKeys: analysis.RequiredPayloadKeys,
		},
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
		log.Printf("failed preparing template pdf preview: letter_type_id=%d path=%q err=%v", letterTypeID, docxPath, err)
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
			var placeholders []string
			if len(t.Placeholders) > 0 {
				_ = json.Unmarshal(t.Placeholders, &placeholders)
			}
			analysis := helpers.ClassifyTemplatePlaceholders(placeholders)

			items = append(items, TemplateListItem{
				ID:                  t.ID,
				LetterTypeID:        t.LetterTypeID,
				Code:                t.LetterType.Code,
				Name:                t.LetterType.Name,
				Description:         t.LetterType.Description,
				FilePath:            helpers.ToAbsoluteURL(t.FilePath),
				FileType:            t.FileType,
				RequiredPayloadKeys: analysis.RequiredPayloadKeys,
				CreatedBy:           t.CreatedBy,
				CreatedAt:           t.CreatedAt,
				UpdatedAt:           t.UpdatedAt,
			})
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	return &Response{StatusCode: http.StatusOK, Message: "Berhasil mengambil data template", Data: items}, nil
}
