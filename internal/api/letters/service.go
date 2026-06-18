package letters

import (
	"encoding/json"
	"errors"
	"log"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/reyimanuel/letter-administration/internal/infrastructures/pkg/errs"
	"github.com/reyimanuel/letter-administration/internal/infrastructures/pkg/helpers"
	"github.com/reyimanuel/letter-administration/internal/migration"
	"github.com/reyimanuel/letter-administration/internal/realtime"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

type Service struct {
	Repo *Repository
}

const maxTemplateUploadBytes = 5 * 1024 * 1024

var (
	letterTypeCodePattern      = regexp.MustCompile(`^[A-Z0-9_]{2,40}$`)
	numberSegmentPattern       = regexp.MustCompile(`^[A-Z0-9. -]{1,30}$`)
	attachmentRequirementKeyRe = regexp.MustCompile(`^[a-z][a-z0-9_]{1,49}$`)
	attachmentAcceptPattern    = regexp.MustCompile(`^[a-z0-9*.+-]+/[a-z0-9*.+-]+(,[a-z0-9*.+-]+/[a-z0-9*.+-]+)*$`)
)

func NewService(repo *Repository) *Service {
	return &Service{Repo: repo}
}

func normalizeNumberSegment(value string) (string, error) {
	normalized := strings.ToUpper(strings.TrimSpace(value))
	normalized = strings.ReplaceAll(normalized, " ", ".")
	normalized = strings.Trim(normalized, ".")
	if normalized == "" {
		return "", nil
	}
	if strings.Contains(normalized, "/") {
		return "", errs.BadRequest("kode kerja dan kode klasifikasi tidak boleh mengandung karakter /")
	}
	if !numberSegmentPattern.MatchString(normalized) {
		return "", errs.BadRequest("kode kerja dan kode klasifikasi hanya boleh berisi huruf, angka, titik, spasi, dan tanda hubung")
	}
	return normalized, nil
}

func normalizeWorkCodeSegment(value string) (string, error) {
	normalized, err := normalizeNumberSegment(value)
	if err != nil {
		return "", err
	}
	if normalized == "UN12.2" {
		return "", nil
	}
	return strings.TrimPrefix(normalized, "UN12.2."), nil
}

func normalizeLetterTypeCode(value string) string {
	code := strings.TrimSpace(value)
	if code == "" {
		return ""
	}
	return strings.ToUpper(strings.ReplaceAll(code, " ", "_"))
}

func validateLetterTypeCode(code string) error {
	if code == "" {
		return nil
	}
	if !letterTypeCodePattern.MatchString(code) {
		return errs.BadRequest("kode tipe surat harus 2-40 karakter dan hanya boleh berisi huruf kapital, angka, atau underscore")
	}
	return nil
}

func validateLetterTypeName(name string) error {
	if name == "" {
		return errs.BadRequest("name wajib diisi")
	}
	if len([]rune(name)) < 3 || len([]rune(name)) > 120 {
		return errs.BadRequest("nama tipe surat harus 3-120 karakter")
	}
	if !helpers.IsSafeHTML(name) {
		return errs.BadRequest("nama tipe surat mengandung karakter tidak aman")
	}
	return nil
}

func validateDescription(description string) error {
	if len([]rune(description)) > 500 {
		return errs.BadRequest("deskripsi maksimal 500 karakter")
	}
	if !helpers.IsSafeHTML(description) {
		return errs.BadRequest("deskripsi mengandung karakter tidak aman")
	}
	return nil
}

func validateTemplateFile(file *multipart.FileHeader) error {
	if file == nil {
		return errs.BadRequest("file tidak ditemukan")
	}
	if filepath.Ext(file.Filename) != ".docx" {
		return errs.BadRequest("hanya file .docx yang diperbolehkan")
	}
	if file.Size <= 0 {
		return errs.BadRequest("file template kosong")
	}
	if file.Size > maxTemplateUploadBytes {
		return errs.BadRequest("ukuran file template maksimal 5MB")
	}
	return nil
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
		if !attachmentRequirementKeyRe.MatchString(key) {
			return nil, errs.BadRequest("key requirement harus 2-50 karakter, diawali huruf kecil, dan hanya boleh berisi huruf kecil, angka, atau underscore")
		}
		if len([]rune(label)) > 120 {
			return nil, errs.BadRequest("label requirement maksimal 120 karakter")
		}
		if !helpers.IsSafeHTML(label) {
			return nil, errs.BadRequest("label requirement mengandung karakter tidak aman")
		}
		accept := strings.TrimSpace(r.Accept)
		if accept != "" && !attachmentAcceptPattern.MatchString(strings.ToLower(accept)) {
			return nil, errs.BadRequest("format accept requirement tidak valid")
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

	realtime.Publish("letter-templates", "letter-template-requirements-updated", letterTypeID)

	return &Response{StatusCode: http.StatusOK, Message: "Requirements berhasil diupdate"}, nil
}

func (s *Service) UpdateLetterType(letterTypeID uint, req UpdateLetterTypeRequest) (*Response, error) {
	name := strings.TrimSpace(req.Name)
	if err := validateLetterTypeName(name); err != nil {
		return nil, err
	}

	code := normalizeLetterTypeCode(req.Code)
	if err := validateLetterTypeCode(code); err != nil {
		return nil, err
	}
	desc := strings.TrimSpace(req.Description)
	if err := validateDescription(desc); err != nil {
		return nil, err
	}
	workCode, err := normalizeWorkCodeSegment(req.WorkCode)
	if err != nil {
		return nil, err
	}
	classificationCode, err := normalizeNumberSegment(req.ClassificationCode)
	if err != nil {
		return nil, err
	}
	if workCode == "" {
		return nil, errs.BadRequest("kode kerja wajib diisi")
	}
	if classificationCode == "" {
		return nil, errs.BadRequest("kode klasifikasi wajib diisi")
	}

	txErr := s.Repo.WithTx(func(tx *gorm.DB) error {
		lt, err := s.Repo.GetLetterTypeByID(tx, letterTypeID)
		if err != nil {
			return errs.NotFound("Jenis surat tidak ditemukan")
		}

		// Jika code berubah, validasi tidak duplikat
		if code != "" && code != lt.Code {
			existing, _ := s.Repo.GetLetterTypeByCode(tx, code)
			if existing != nil {
				return errs.BadRequest("kode tipe surat sudah terdaftar")
			}
		}

		return s.Repo.UpdateLetterType(tx, letterTypeID, code, name, desc, workCode, classificationCode)
	})
	if txErr != nil {
		return nil, txErr
	}

	realtime.Publish("letter-templates", "letter-type-updated", letterTypeID)

	return &Response{StatusCode: http.StatusOK, Message: "Jenis surat berhasil diupdate"}, nil
}

// UploadTemplateV2 uploads a .docx template, detects {{placeholders}} inside the DOCX,
// persists them, and returns the required payload keys to help clients build forms.
func (s *Service) UploadTemplateV2(adminID uint, req UploadTemplateV2Request, file *multipart.FileHeader) (*Response, error) {
	if err := validateTemplateFile(file); err != nil {
		return nil, err
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
	requestWorkCode, err := normalizeWorkCodeSegment(req.WorkCode)
	if err != nil {
		_ = os.Remove(newPath)
		return nil, err
	}
	requestClassificationCode, err := normalizeNumberSegment(req.ClassificationCode)
	if err != nil {
		_ = os.Remove(newPath)
		return nil, err
	}

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

			// Update metadata jika dikirim
			code := normalizeLetterTypeCode(req.Code)
			name := strings.TrimSpace(req.Name)
			desc := strings.TrimSpace(req.Description)
			if err := validateLetterTypeCode(code); err != nil {
				return err
			}
			if name != "" {
				if err := validateLetterTypeName(name); err != nil {
					return err
				}
			}
			if err := validateDescription(desc); err != nil {
				return err
			}
			updateCode := code
			if updateCode == "" {
				updateCode = lt.Code
			}
			updateName := name
			if updateName == "" {
				updateName = lt.Name
			}
			updateDesc := desc
			if updateDesc == "" {
				updateDesc = lt.Description
			}
			updateWorkCode := requestWorkCode
			if updateWorkCode == "" {
				updateWorkCode = lt.WorkCode
			}
			updateClassificationCode := requestClassificationCode
			if updateClassificationCode == "" {
				updateClassificationCode = lt.ClassificationCode
			}
			if updateWorkCode == "" {
				return errs.BadRequest("kode kerja wajib diisi")
			}
			if updateClassificationCode == "" {
				return errs.BadRequest("kode klasifikasi wajib diisi")
			}

			if code != "" && code != lt.Code {
				existing, _ := s.Repo.GetLetterTypeByCode(tx, code)
				if existing != nil {
					return errs.BadRequest("kode tipe surat sudah terdaftar")
				}
			}

			if updateCode != lt.Code ||
				updateName != lt.Name ||
				updateDesc != lt.Description ||
				updateWorkCode != lt.WorkCode ||
				updateClassificationCode != lt.ClassificationCode {
				// Jika hanya name atau description yang berubah, gunakan yang lama untuk yang tidak dikirim
				if err := s.Repo.UpdateLetterType(tx, resolvedLetterTypeID, updateCode, updateName, updateDesc, updateWorkCode, updateClassificationCode); err != nil {
					return err
				}
				// Update resolvedLetterType untuk response
				resolvedLetterType.Code = updateCode
				resolvedLetterType.Name = updateName
				resolvedLetterType.Description = updateDesc
				resolvedLetterType.WorkCode = updateWorkCode
				resolvedLetterType.ClassificationCode = updateClassificationCode
			}
		} else {
			code := normalizeLetterTypeCode(req.Code)
			name := strings.TrimSpace(req.Name)
			desc := strings.TrimSpace(req.Description)
			if code == "" || name == "" {
				return errs.BadRequest("code dan name wajib diisi jika letter_type_id tidak ada")
			}
			if err := validateLetterTypeCode(code); err != nil {
				return err
			}
			if err := validateLetterTypeName(name); err != nil {
				return err
			}
			if err := validateDescription(desc); err != nil {
				return err
			}
			if requestWorkCode == "" {
				return errs.BadRequest("kode kerja wajib diisi")
			}
			if requestClassificationCode == "" {
				return errs.BadRequest("kode klasifikasi wajib diisi")
			}
			lt := &migration.LetterType{
				Code:               code,
				Name:               name,
				Description:        desc,
				WorkCode:           requestWorkCode,
				ClassificationCode: requestClassificationCode,
			}
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

	realtime.Publish("letter-templates", "letter-template-uploaded", resolvedLetterTypeID)

	return &Response{
		StatusCode: http.StatusCreated,
		Message:    "Template surat berhasil diupload",
		Data: TemplateUploadV2Data{
			LetterTypeID:        resolvedLetterTypeID,
			Code:                resolvedLetterType.Code,
			Name:                resolvedLetterType.Name,
			Description:         resolvedLetterType.Description,
			WorkCode:            resolvedLetterType.WorkCode,
			ClassificationCode:  resolvedLetterType.ClassificationCode,
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

	realtime.Publish("letter-templates", "letter-template-deleted", letterTypeID)

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
			if analysis.Placeholders == nil {
				analysis.Placeholders = []string{}
			}
			if analysis.AutoFilledKeys == nil {
				analysis.AutoFilledKeys = []string{}
			}
			if analysis.RequiredPayloadKeys == nil {
				analysis.RequiredPayloadKeys = []string{}
			}

			items = append(items, TemplateListItem{
				ID:                  t.ID,
				LetterTypeID:        t.LetterTypeID,
				Code:                t.LetterType.Code,
				Name:                t.LetterType.Name,
				Description:         t.LetterType.Description,
				WorkCode:            t.LetterType.WorkCode,
				ClassificationCode:  t.LetterType.ClassificationCode,
				FilePath:            helpers.ToAbsoluteURL(t.FilePath),
				FileType:            t.FileType,
				RequiredPayloadKeys: analysis.RequiredPayloadKeys,
				Placeholders:        analysis.Placeholders,
				AutoFilledKeys:      analysis.AutoFilledKeys,
				CreatedBy:           t.CreatedBy,
				CreatorName:         t.Creator.Name,
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

func (s *Service) GetAllLetterTypes() (*Response, error) {
	var items []LetterTypeResponse

	err := s.Repo.WithTx(func(tx *gorm.DB) error {
		letterTypes, err := s.Repo.ListLetterTypes(tx)
		if err != nil {
			return err
		}

		items = make([]LetterTypeResponse, 0, len(letterTypes))
		for _, lt := range letterTypes {
			items = append(items, LetterTypeResponse{
				ID:                 lt.ID,
				Code:               lt.Code,
				Name:               lt.Name,
				Description:        lt.Description,
				WorkCode:           lt.WorkCode,
				ClassificationCode: lt.ClassificationCode,
			})
		}

		return nil
	})
	if err != nil {
		return nil, err
	}

	return &Response{StatusCode: http.StatusOK, Message: "Berhasil mengambil data jenis surat", Data: items}, nil
}
