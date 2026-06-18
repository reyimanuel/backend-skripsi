package correspondence

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/reyimanuel/letter-administration/internal/api/letters"
	user "github.com/reyimanuel/letter-administration/internal/api/users"
	"github.com/reyimanuel/letter-administration/internal/constants"
	"github.com/reyimanuel/letter-administration/internal/infrastructures/pkg/errs"
	"github.com/reyimanuel/letter-administration/internal/infrastructures/pkg/helpers"
	"github.com/reyimanuel/letter-administration/internal/infrastructures/pkg/policy"
	"github.com/reyimanuel/letter-administration/internal/infrastructures/pkg/push"
	"github.com/reyimanuel/letter-administration/internal/migration"
	"github.com/reyimanuel/letter-administration/internal/realtime"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

const (
	statusDraft     = "draft"
	statusSubmitted = "submitted"
	statusForwarded = "forwarded"
	statusApproved  = "approved"
	statusSigned    = "signed"
	statusRejected  = "rejected"

	approvalPending  = "pending"
	approvalApproved = "approved"
	approvalRejected = "rejected"

	historySubmitted = "SUBMITTED"
	historyForwarded = "FORWARDED"
	historyApproved  = "APPROVED"
	historyNumbered  = "NUMBERED"
	historyRejected  = "REJECTED"

	maxLetterSubjectRunes      = 150
	maxLetterPayloadKeys       = 60
	maxLetterPayloadValueRunes = 1000
	maxAttachmentBytes         = 5 * 1024 * 1024
	maxActionNotesRunes        = 500
)

var (
	payloadKeyPattern           = regexp.MustCompile(`^[a-z][a-z0-9_]{1,49}$`)
	attachmentKeyPattern        = regexp.MustCompile(`^[a-z][a-z0-9_]{1,49}$`)
	letterNumberSeqPattern      = regexp.MustCompile(`^[0-9]{1,6}$`)
	additionalStudentNIMPattern = regexp.MustCompile(`^[0-9]{6,20}$`)
)

var blockedPayloadFields = map[string]struct{}{
	"mahasiswa":             {},
	"tabel_data_mahasiswa":  {},
	"nim":                   {},
	"program_studi":         {},
	"angkatan":              {},
	"semester_masuk_kuliah": {},
	"tahun_ajaran":          {},
	"hari":                  {},
	"tanggal":               {},
	"bulan":                 {},
	"tahun":                 {},
	"nomor_surat":           {},
	"official":              {},
	"nip":                   {},
	"pangkat":               {},
	"jabatan":               {},
	"ttd":                   {},
	"tanda_tangan":          {},
	"signature":             {},
}

var studentTemplateFields = map[string]struct{}{
	"mahasiswa":             {},
	"tabel_data_mahasiswa":  {},
	"nim":                   {},
	"program_studi":         {},
	"angkatan":              {},
	"semester_masuk_kuliah": {},
	"hari":                  {},
	"tanggal":               {},
	"bulan":                 {},
	"tahun":                 {},
}

func hasOfficialRole(roles []string) bool {
	for _, role := range roles {
		if constants.IsOfficialRoleCode(role) {
			return true
		}
	}
	return false
}

type Service struct {
	Repo        *Repository
	LettersRepo *letters.Repository
	UsersRepo   *user.Repository
}

func NewService(repo *Repository, lettersRepo *letters.Repository, usersRepo *user.Repository) *Service {
	return &Service{Repo: repo, LettersRepo: lettersRepo, UsersRepo: usersRepo}
}

func (s *Service) ListActiveOfficials() (*Response, error) {
	var items []OfficialTargetItem
	err := s.Repo.WithTx(func(tx *gorm.DB) error {
		officials, err := s.UsersRepo.ListActiveOfficials(tx)
		if err != nil {
			log.Printf("error listing active officials: %v", err)
			return errs.InternalServerError("Gagal mengambil daftar pejabat")
		}

		items = make([]OfficialTargetItem, 0, len(officials))
		for _, official := range officials {
			roleCode := officialRoleCode(official)
			if roleCode == "" {
				continue
			}
			items = append(items, OfficialTargetItem{
				ID:       official.ID,
				UserID:   official.UserID,
				Name:     official.User.Name,
				RoleCode: roleCode,
				Jabatan:  strings.TrimSpace(official.Jabatan),
				NIP:      strings.TrimSpace(official.NIP),
			})
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	return &Response{StatusCode: http.StatusOK, Message: "Daftar pejabat berhasil diambil", Data: items}, nil
}

func (s *Service) PreviewLetter(letterID uint, userID uint, isAdmin bool, isOfficial bool) (string, string, error) {
	var filePath string

	err := s.Repo.WithTx(func(tx *gorm.DB) error {
		letter, err := s.Repo.GetLetterWithTypeByID(tx, letterID)
		if err != nil {
			log.Printf("letter not found: id=%d err=%v", letterID, err)
			return errs.NotFound("Surat tidak ditemukan")
		}

		if !isAdmin {
			if isOfficial {
				official, err := s.UsersRepo.GetActiveOfficialByUserID(tx, userID)
				if err != nil {
					log.Printf("official not found for preview: user_id=%d err=%v", userID, err)
					return errs.Forbidden("Hanya pejabat yang dapat melihat preview surat ini")
				}
				if letter.SignedByID == nil || *letter.SignedByID != official.ID {
					return errs.Forbidden("Anda tidak memiliki akses ke surat ini")
				}
			} else {
				student, err := s.UsersRepo.GetStudentByUserID(tx, userID)
				if err != nil {
					log.Printf("student not found for preview: user_id=%d err=%v", userID, err)
					return errs.Forbidden("Hanya pemilik surat yang dapat melihat preview")
				}

				if letter.StudentID != student.ID {
					return errs.Forbidden("Anda tidak memiliki akses ke surat ini")
				}
			}
		}

		if letter.FilePath == "" {
			return errs.NotFound("File surat belum tersedia")
		}

		filePath = letter.FilePath
		return nil
	})
	if err != nil {
		return "", "", err
	}

	pdfPath, err := helpers.EnsurePDFPreview(filePath)
	if err != nil {
		log.Printf("failed preparing preview for letter=%d path=%q err=%v", letterID, filePath, err)
		return "", "", errs.InternalServerError("Gagal menyiapkan preview surat")
	}

	fileName := fmt.Sprintf("preview_%d.pdf", letterID)
	if base := filepath.Base(pdfPath); base != "" {
		fileName = base
	}

	return pdfPath, fileName, nil
}

func (s *Service) ListForwardedLetters(userID uint, q ListLettersQuery) (*Response, error) {
	// Filter by requested status, but restrict to valid official statuses.
	status := strings.TrimSpace(strings.ToLower(q.Status))
	if status != statusForwarded && status != statusApproved && status != statusSigned && status != statusRejected {
		// Default to forwarded if invalid or empty.
		status = statusForwarded
	}

	sort := strings.TrimSpace(q.Sort)
	if sort != "" && sort != "created_at_desc" && sort != "created_at_asc" {
		return nil, errs.BadRequest("sort tidak valid")
	}
	if sort == "" {
		sort = "created_at_desc"
	}

	page := q.Page
	if page <= 0 {
		page = 1
	}
	pageSize := q.PageSize
	if pageSize <= 0 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}

	var createdFrom *time.Time
	if strings.TrimSpace(q.CreatedFrom) != "" {
		tm, err := parseTimeOrDate(strings.TrimSpace(q.CreatedFrom))
		if err != nil {
			return nil, errs.BadRequest("created_from tidak valid")
		}
		createdFrom = &tm
	}
	var createdTo *time.Time
	if strings.TrimSpace(q.CreatedTo) != "" {
		tm, err := parseTimeOrDate(strings.TrimSpace(q.CreatedTo))
		if err != nil {
			return nil, errs.BadRequest("created_to tidak valid")
		}
		createdTo = &tm
	}
	if createdFrom != nil && createdTo != nil && createdFrom.After(*createdTo) {
		return nil, errs.BadRequest("range tanggal tidak valid")
	}

	var letterTypeID *uint
	if q.LetterType != 0 {
		letterTypeID = &q.LetterType
	}

	var items []LetterListItem
	var total int64

	err := s.Repo.WithTx(func(tx *gorm.DB) error {
		official, err := s.UsersRepo.GetActiveOfficialByUserID(tx, userID)
		if err != nil {
			return errs.Forbidden("Hanya pejabat yang dapat melihat surat forwarded")
		}

		signedByID := official.ID
		letters, count, err := s.Repo.ListLetters(tx, ListLettersParams{
			SignedByID:  &signedByID,
			Query:       q.Q,
			Status:      status,
			LetterType:  letterTypeID,
			CreatedFrom: createdFrom,
			CreatedTo:   createdTo,
			Sort:        sort,
			Page:        page,
			PageSize:    pageSize,
		})
		if err != nil {
			log.Printf("error listing forwarded letters: %v", err)
			return errs.InternalServerError("Gagal mengambil daftar surat")
		}
		total = count

		items = make([]LetterListItem, 0, len(letters))
		for _, l := range letters {
			previewURL := helpers.ToAbsoluteURL(fmt.Sprintf("/api/correspondence/preview/%d", l.ID))
			historyURL := helpers.ToAbsoluteURL(fmt.Sprintf("/api/correspondence/history/%d", l.ID))

			student := &StudentSummary{
				StudentID: l.Student.ID,
				UserID:    l.Student.User.ID,
				Name:      l.Student.User.Name,
				NIM:       l.Student.NIM,
			}

			items = append(items, LetterListItem{
				ID:      l.ID,
				Subject: l.Subject,
				Status:  l.Status,
				LetterNo: func() *string {
					if l.LetterNumber == nil || strings.TrimSpace(*l.LetterNumber) == "" {
						return nil
					}
					return l.LetterNumber
				}(),
				LetterType: LetterTypeSummary{
					ID:                 l.LetterType.ID,
					Code:               l.LetterType.Code,
					Name:               l.LetterType.Name,
					WorkCode:           l.LetterType.WorkCode,
					ClassificationCode: l.LetterType.ClassificationCode,
				},
				Student:    student,
				PreviewURL: previewURL,
				HistoryURL: historyURL,
				CreatedAt:  l.CreatedAt,
				UpdatedAt:  l.UpdatedAt,
			})
		}

		return nil
	})
	if err != nil {
		return nil, err
	}

	return &Response{
		StatusCode: http.StatusOK,
		Message:    "Daftar surat forwarded berhasil diambil",
		Data: LetterListData{
			Items: items,
			Meta:  PaginationMeta{Page: page, PageSize: pageSize, Total: total},
		},
	}, nil
}

func (s *Service) CreateDraftLetter(userID uint, req CreateDraftRequest) (*Response, error) {
	var letter *migration.Letter
	var payloadMap map[string]any
	subject, err := validateLetterSubject(req.Subject)
	if err != nil {
		return nil, err
	}
	if err := validateSubmitPayload(req.Payload); err != nil {
		return nil, err
	}

	err = s.Repo.WithTx(func(tx *gorm.DB) error {
		student, err := s.UsersRepo.GetStudentByUserID(tx, userID)
		if err != nil {
			return errs.Forbidden("Hanya mahasiswa yang dapat membuat draft surat")
		}
		if err := policy.CanStudentSubmitLetter(&student.User, student); err != nil {
			return err
		}

		// Ensure template exists for the type (so we don't allow drafting unusable letters)
		template, err := s.Repo.GetTemplateByLetterType(tx, req.LetterTypeID)
		if err != nil {
			return errs.NotFound("Template surat tidak ditemukan")
		}

		payloadMap = buildSubmitPayload(req.Payload)
		if missing := missingTemplateKeysForPayload(template, req.LetterTypeID, payloadMap); len(missing) > 0 {
			return errs.BadRequestWithData("Data draft belum lengkap", missingTemplateFieldsData{Missing: missing})
		}

		payloadJSON, err := marshalPayload(payloadMap)
		if err != nil {
			return errs.InternalServerError("Terjadi kesalahan dalam membuat draft")
		}

		letter = &migration.Letter{
			StudentID:    student.ID,
			LetterTypeID: req.LetterTypeID,
			Subject:      subject,
			Payload:      payloadJSON,
			Status:       statusDraft,
			FilePath:     "",
		}
		if err := s.Repo.CreateLetter(tx, letter); err != nil {
			log.Printf("error creating draft letter: %v", err)
			return errs.InternalServerError("Terjadi kesalahan dalam membuat draft")
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	realtime.Publish("letters", "letter-draft-created", letter.ID)

	return &Response{
		StatusCode: http.StatusCreated,
		Message:    "Draft surat berhasil dibuat",
		Data: Data{
			ID:           letter.ID,
			LetterTypeID: letter.LetterTypeID,
			Subject:      letter.Subject,
			Status:       letter.Status,
			Payload:      payloadMap,
			FilePath:     "",
			PreviewURL:   "",
			CreatedAt:    letter.CreatedAt,
		},
	}, nil
}

func (s *Service) UpdateDraftLetter(letterID uint, userID uint, req UpdateDraftRequest) (*Response, error) {
	var payloadOut map[string]any
	var letterOut *migration.Letter

	err := s.Repo.WithTx(func(tx *gorm.DB) error {
		student, err := s.UsersRepo.GetStudentByUserID(tx, userID)
		if err != nil {
			return errs.Forbidden("Hanya mahasiswa yang dapat mengubah draft surat")
		}
		if err := policy.CanStudentSubmitLetter(&student.User, student); err != nil {
			return err
		}

		letter, err := s.Repo.GetLetterWithTypeByID(tx, letterID)
		if err != nil {
			return errs.NotFound("Surat tidak ditemukan")
		}
		if letter.StudentID != student.ID {
			return errs.Forbidden("Anda tidak memiliki akses ke surat ini")
		}
		if letter.Status != statusDraft {
			return errs.BadRequest("Surat bukan draft")
		}

		if req.Subject != nil {
			subject, err := validateLetterSubject(*req.Subject)
			if err != nil {
				return err
			}
			letter.Subject = subject
		}

		payloadMap, err := unmarshalPayload(letter.Payload)
		if err != nil {
			return errs.InternalServerError("Terjadi kesalahan dalam membaca data draft")
		}
		if payloadMap == nil {
			payloadMap = map[string]any{}
		}

		if req.Payload != nil {
			if err := validateSubmitPayload(req.Payload); err != nil {
				return err
			}
			incoming := buildSubmitPayload(req.Payload)
			for k, v := range incoming {
				payloadMap[k] = v
			}

			template, err := s.Repo.GetTemplateByLetterType(tx, letter.LetterTypeID)
			if err != nil {
				return errs.NotFound("Template surat tidak ditemukan")
			}
			if missing := missingTemplateKeysForPayload(template, letter.LetterTypeID, payloadMap); len(missing) > 0 {
				return errs.BadRequestWithData("Data draft belum lengkap", missingTemplateFieldsData{Missing: missing})
			}
		}

		payloadJSON, err := marshalPayload(payloadMap)
		if err != nil {
			return errs.InternalServerError("Terjadi kesalahan dalam memproses draft")
		}
		letter.Payload = payloadJSON

		if err := s.Repo.SaveLetter(tx, letter); err != nil {
			return errs.InternalServerError("Gagal menyimpan draft")
		}

		payloadOut = payloadMap
		letterOut = letter
		return nil
	})
	if err != nil {
		return nil, err
	}

	realtime.Publish("letters", "letter-draft-updated", letterOut.ID)

	return &Response{
		StatusCode: http.StatusOK,
		Message:    "Draft surat berhasil diperbarui",
		Data: Data{
			ID:           letterOut.ID,
			LetterTypeID: letterOut.LetterTypeID,
			Subject:      letterOut.Subject,
			Status:       letterOut.Status,
			Payload:      payloadOut,
			FilePath:     letterOut.FilePath,
			PreviewURL:   "",
			CreatedAt:    letterOut.CreatedAt,
		},
	}, nil
}

func (s *Service) UploadAttachments(letterID uint, userID uint, isAdmin bool, files []*multipart.FileHeader, keys []string, defaultKey string) (*Response, error) {
	if len(files) == 0 {
		return nil, errs.BadRequest("file tidak ditemukan")
	}
	if len(keys) > 0 && len(keys) != len(files) {
		return nil, errs.BadRequest("jumlah keys harus sama dengan jumlah files")
	}

	items := make([]AttachmentItem, 0, len(files))
	err := s.Repo.WithTx(func(tx *gorm.DB) error {
		letter, err := s.Repo.GetLetterWithTypeByID(tx, letterID)
		if err != nil {
			return errs.NotFound("Surat tidak ditemukan")
		}

		if !isAdmin {
			student, err := s.UsersRepo.GetStudentByUserID(tx, userID)
			if err != nil {
				return errs.Forbidden("Hanya pemilik surat yang dapat mengupload berkas")
			}
			if letter.StudentID != student.ID {
				return errs.Forbidden("Anda tidak memiliki akses ke surat ini")
			}
		}

		for idx, f := range files {
			if f == nil {
				continue
			}
			key := strings.TrimSpace(defaultKey)
			if len(keys) > 0 {
				key = strings.TrimSpace(keys[idx])
			}
			if key == "" {
				return errs.BadRequest("key berkas wajib diisi")
			}
			if err := validateAttachmentInput(key, f); err != nil {
				return err
			}
			newName := helpers.GenerateUniqueFileName(f.Filename)
			newPath := filepath.Join("public", "generated", "attachments", fmt.Sprintf("letter_%d", letterID), newName)
			if err := helpers.SaveUploadedFile(f, newPath); err != nil {
				log.Printf("failed saving attachment: letter_id=%d filename=%q err=%v", letterID, f.Filename, err)
				return errs.InternalServerError("gagal menyimpan berkas")
			}

			mime, err := helpers.DetectMimeTypeFromPath(newPath)
			if err != nil {
				_ = os.Remove(newPath)
				return errs.InternalServerError("gagal membaca berkas")
			}
			if helpers.IsTemplateImagePlaceholderKey(key) && mime != "image/png" && mime != "image/jpeg" {
				_ = os.Remove(newPath)
				return errs.BadRequest("berkas " + key + " harus berupa gambar PNG atau JPG")
			}

			att := &migration.LetterAttachment{
				LetterID:       letterID,
				RequirementKey: key,
				FilePath:       filepath.ToSlash(newPath),
				FileType:       mime,
				UploadedAt:     time.Now(),
			}
			if err := s.Repo.CreateAttachment(tx, att); err != nil {
				_ = os.Remove(newPath)
				return errs.InternalServerError("gagal menyimpan data berkas")
			}

			items = append(items, AttachmentItem{ID: att.ID, Key: att.RequirementKey, FilePath: helpers.ToAbsoluteURL(att.FilePath), FileType: att.FileType})
		}

		return nil
	})
	if err != nil {
		return nil, err
	}

	realtime.Publish("letters", "letter-attachments-uploaded", letterID)

	return &Response{StatusCode: http.StatusCreated, Message: "Berkas berhasil diupload", Data: UploadAttachmentsResponse{LetterID: letterID, Attachments: items}}, nil
}

type missingAttachmentsData struct {
	Missing []string `json:"missing"`
}

type missingTemplateFieldsData struct {
	Missing []string `json:"missing"`
}

func extractTemplatePlaceholdersForLetterType(template *migration.LetterTemplate, letterTypeID uint) []string {
	if template == nil {
		return nil
	}

	var templatePlaceholders []string
	if len(template.Placeholders) > 0 {
		if err := json.Unmarshal(template.Placeholders, &templatePlaceholders); err != nil {
			log.Printf("invalid placeholders JSON on template: letter_type_id=%d err=%v", letterTypeID, err)
			templatePlaceholders = nil
		}
	}

	if len(templatePlaceholders) == 0 {
		// Backward compatibility for templates uploaded before v2.
		if analysis, err := helpers.AnalyzeDocxTemplatePlaceholders(template.FilePath); err == nil {
			templatePlaceholders = analysis.Placeholders
		} else {
			log.Printf("placeholder analysis skipped: template=%q err=%v", template.FilePath, err)
		}
	}

	return templatePlaceholders
}

func missingTemplateKeysForPayload(template *migration.LetterTemplate, letterTypeID uint, payload map[string]any) []string {
	placeholders := extractTemplatePlaceholdersForLetterType(template, letterTypeID)
	ph := helpers.ClassifyTemplatePlaceholders(placeholders)

	return helpers.MissingPayloadKeys(payload, ph.RequiredPayloadKeys)
}

func (s *Service) SubmitDraftLetter(letterID uint, userID uint) (*Response, error) {
	var previewURL string
	var studentName string
	var letterSubject string
	err := s.Repo.WithTx(func(tx *gorm.DB) error {
		now := time.Now()
		student, err := s.UsersRepo.GetStudentByUserID(tx, userID)
		if err != nil {
			return errs.Forbidden("Hanya mahasiswa yang dapat submit surat")
		}
		studentName = student.User.Name
		if err := policy.CanStudentSubmitLetter(&student.User, student); err != nil {
			return err
		}

		letter, err := s.Repo.GetLetterWithTypeByID(tx, letterID)
		if err != nil {
			return errs.NotFound("Surat tidak ditemukan")
		}
		if letter.StudentID != student.ID {
			return errs.Forbidden("Anda tidak memiliki akses ke surat ini")
		}
		if letter.Status != statusDraft {
			return errs.BadRequest("Surat bukan draft")
		}
		letterSubject = letter.Subject

		// parse required keys from letter type
		reqs, err := parseRequiredAttachmentKeys(letter.LetterType.AttachmentRequirements)
		if err != nil {
			log.Printf("invalid attachment requirements: letter_type_id=%d err=%v", letter.LetterTypeID, err)
			return errs.InternalServerError("Konfigurasi requirements surat tidak valid")
		}

		if len(reqs) > 0 {
			haveKeys, err := s.Repo.ListAttachmentKeysByLetterID(tx, letterID)
			if err != nil {
				return errs.InternalServerError("Gagal memeriksa berkas")
			}
			have := make(map[string]struct{}, len(haveKeys))
			for _, k := range haveKeys {
				have[strings.TrimSpace(k)] = struct{}{}
			}
			missing := make([]string, 0)
			for _, k := range reqs {
				if _, ok := have[k]; !ok {
					missing = append(missing, k)
				}
			}
			if len(missing) > 0 {
				return errs.BadRequestWithData("Berkas wajib belum lengkap", missingAttachmentsData{Missing: missing})
			}
		}

		template, err := s.Repo.GetTemplateByLetterType(tx, letter.LetterTypeID)
		if err != nil {
			return errs.NotFound("Template surat tidak ditemukan")
		}

		payloadMap, err := unmarshalPayload(letter.Payload)
		if err != nil {
			return errs.InternalServerError("Terjadi kesalahan dalam membaca data surat")
		}

		// Validate template placeholders: ensure all required payload keys exist
		// so the generated document won't keep any {{...}} tokens.
		var templatePlaceholders []string
		if len(template.Placeholders) > 0 {
			if err := json.Unmarshal(template.Placeholders, &templatePlaceholders); err != nil {
				log.Printf("invalid placeholders JSON on template: letter_type_id=%d err=%v", letter.LetterTypeID, err)
				templatePlaceholders = nil
			}
		}
		if len(templatePlaceholders) == 0 {
			// Backward compatibility for templates uploaded before v2.
			if analysis, err := helpers.AnalyzeDocxTemplatePlaceholders(template.FilePath); err == nil {
				templatePlaceholders = analysis.Placeholders
			} else {
				log.Printf("placeholder analysis skipped: template=%q err=%v", template.FilePath, err)
			}
		}

		ph := helpers.ClassifyTemplatePlaceholders(templatePlaceholders)
		ensureLetterSystemPayload(payloadMap)

		missingPayload := helpers.MissingPayloadKeys(payloadMap, ph.RequiredPayloadKeys)
		if len(missingPayload) > 0 {
			return errs.BadRequestWithData("Data surat belum lengkap", missingTemplateFieldsData{Missing: missingPayload})
		}

		imageKeys := templateImagePlaceholderKeys(templatePlaceholders)
		if err := s.ensureAttachmentKeysPresent(tx, letterID, imageKeys); err != nil {
			return err
		}

		// Persist the enriched payload so later steps (approve/history) can
		// reuse the same computed fields.
		payloadJSON, err := marshalPayload(payloadMap)
		if err != nil {
			log.Printf("error marshaling submit payload: %v", err)
			return errs.InternalServerError("Terjadi kesalahan dalam memproses data surat")
		}
		letter.Payload = payloadJSON

		// Build a payload view for templating that includes blank defaults for
		// auto-filled keys that may only be available later (e.g. nomor_surat).
		payloadForTemplate := make(map[string]any, len(payloadMap)+len(ph.AutoFilledKeys))
		for k, v := range payloadMap {
			payloadForTemplate[k] = v
		}
		for _, k := range ph.AutoFilledKeys {
			if _, ok := payloadForTemplate[k]; !ok {
				payloadForTemplate[k] = ""
			}
		}

		data := buildTemplateData(student, payloadForTemplate)
		if len(imageKeys) > 0 {
			atts, err := s.Repo.GetAttachmentsByLetterID(tx, letterID)
			if err != nil {
				return errs.InternalServerError("Gagal mengambil data berkas")
			}
			addTemplateImageAttachments(data, atts, imageKeys)
		}

		outputDocx := fmt.Sprintf("public/generated/letter_%d.docx", now.UnixNano())
		if err := s.generateLetterDocument(template.FilePath, outputDocx, data); err != nil {
			return err
		}

		letter.Status = statusSubmitted
		letter.FilePath = outputDocx
		if err := s.Repo.SaveLetter(tx, letter); err != nil {
			return errs.InternalServerError("Gagal menyimpan surat")
		}

		if err := s.Repo.CreateHistory(tx, &migration.LetterHistory{LetterID: letter.ID, ActorID: userID, Action: historySubmitted}); err != nil {
			return errs.InternalServerError("Terjadi kesalahan dalam membuat surat")
		}

		adminRole, err := s.UsersRepo.GetRoleByCode(tx, "ADMIN")
		if err != nil {
			return errs.InternalServerError("Gagal mendapatkan role admin")
		}
		if err := s.Repo.CreateApproval(tx, &migration.LetterApproval{LetterID: letter.ID, RoleID: adminRole.ID, Status: approvalPending}); err != nil {
			return errs.InternalServerError("Terjadi kesalahan dalam membuat surat")
		}

		previewURL = helpers.ToAbsoluteURL(fmt.Sprintf("/api/correspondence/preview/%d", letter.ID))
		return nil
	})
	if err != nil {
		return nil, err
	}

	// Best-effort: notify admins about new submitted letter.
	ctx, cancel := context.WithTimeout(context.Background(), constants.ExternalServiceTimeout)
	defer cancel()
	if _, err := push.SendToRole(ctx, s.Repo.DB, "ADMIN", "Surat Baru Disubmit", push.FormatAdminLetterBody(studentName, letterSubject), map[string]string{
		"type":      "letter_submitted",
		"letter_id": fmt.Sprint(letterID),
	}); err != nil {
		log.Printf("push admin notify (letter_submitted) failed: letter_id=%d err=%v", letterID, err)
	}

	realtime.PublishTopics([]string{"letters", "letter-approvals"}, "letter-submitted", letterID)

	return &Response{StatusCode: http.StatusOK, Message: "Surat berhasil disubmit", Data: PreviewResponse{ID: letterID, PreviewURL: previewURL}}, nil
}

func parseRequiredAttachmentKeys(raw datatypes.JSON) ([]string, error) {
	if len(raw) == 0 {
		return []string{}, nil
	}
	var items []struct {
		Key      string `json:"key"`
		Required bool   `json:"required"`
	}
	if err := json.Unmarshal(raw, &items); err != nil {
		return nil, err
	}
	out := make([]string, 0)
	seen := make(map[string]struct{})
	for _, it := range items {
		k := strings.TrimSpace(it.Key)
		if !it.Required || k == "" {
			continue
		}
		if _, ok := seen[k]; ok {
			continue
		}
		seen[k] = struct{}{}
		out = append(out, k)
	}
	return out, nil
}

func (s *Service) ReviewLetter(letterID uint, userID uint, req ApproveLetterRequest) (*Response, error) {
	if err := validateReviewLetterRequest(req); err != nil {
		return nil, err
	}

	historyAction := historyApproved
	message := "Surat berhasil disetujui"

	var studentUserID uint
	var subject string
	var resultingStatus string
	var notificationNotes string
	err := s.Repo.WithTx(func(tx *gorm.DB) error {
		now := time.Now()

		actor, err := s.UsersRepo.GetUserByID(tx, userID)
		if err != nil {
			return errs.Unauthorized("user tidak terautentikasi")
		}
		roles := actor.RoleSlice()
		isAdmin := slices.Contains(roles, "ADMIN")
		isOfficialRole := hasOfficialRole(roles)

		letter, err := s.Repo.GetLetterWithTypeByID(tx, letterID)
		if err != nil {
			log.Printf("letter not found: id=%d err=%v", letterID, err)
			return errs.NotFound("Surat tidak ditemukan")
		}
		subject = letter.Subject

		student, err := s.UsersRepo.GetStudentByID(tx, letter.StudentID)
		if err != nil {
			log.Printf("student not found for approval notify: id=%d err=%v", letter.StudentID, err)
			return errs.NotFound("Data mahasiswa tidak ditemukan")
		}
		studentUserID = student.UserID

		// Action gating by role + status.
		switch req.Action {
		case "forward":
			switch letter.Status {
			case statusSubmitted:
				if !isAdmin {
					return errs.Forbidden("Hanya admin yang dapat meneruskan surat pada tahap ini")
				}
			case statusForwarded:
				if !isOfficialRole {
					return errs.Forbidden("Hanya pejabat yang dapat meneruskan surat forwarded")
				}
				officialActor, err := s.UsersRepo.GetActiveOfficialByUserID(tx, userID)
				if err != nil {
					return errs.Forbidden("Data pejabat tidak ditemukan")
				}
				if letter.SignedByID == nil || *letter.SignedByID != officialActor.ID {
					return errs.Forbidden("Surat ini tidak ditugaskan kepada Anda")
				}
				if err := policy.CanOfficialAct(&officialActor.User, officialActor); err != nil {
					return err
				}
			default:
				return errs.BadRequest("Surat tidak dalam status yang dapat diteruskan")
			}
		case "approve", "reject":
			switch letter.Status {
			case statusSubmitted:
				if !isAdmin {
					return errs.Forbidden("Hanya admin yang dapat memproses surat pada tahap ini")
				}
				if req.Action != "reject" {
					return errs.BadRequest("Admin hanya dapat meneruskan atau menolak surat pada tahap ini")
				}
			case statusForwarded:
				if !isOfficialRole {
					return errs.Forbidden("Hanya pejabat yang dapat memproses surat forwarded")
				}
				// Ensure the forwarded letter is assigned to this official.
				officialActor, err := s.UsersRepo.GetActiveOfficialByUserID(tx, userID)
				if err != nil {
					return errs.Forbidden("Data pejabat tidak ditemukan")
				}
				if letter.SignedByID == nil || *letter.SignedByID != officialActor.ID {
					return errs.Forbidden("Surat ini tidak ditugaskan kepada Anda")
				}
				if err := policy.CanOfficialAct(&officialActor.User, officialActor); err != nil {
					return err
				}
			case statusApproved:
				if !isAdmin || req.Action != "approve" {
					return errs.Forbidden("Hanya admin yang dapat mengisi nomor surat")
				}
			default:
				return errs.BadRequest("Surat tidak dalam status yang dapat diproses")
			}
		default:
			return errs.BadRequest("aksi tidak valid")
		}

		approval, err := s.Repo.GetApprovalByLetterID(tx, letter.ID)
		if err != nil {
			log.Printf("approval not found: letter_id=%d err=%v", letter.ID, err)
			return errs.InternalServerError("Data approval surat tidak ditemukan")
		}

		switch req.Action {
		case "reject":
			historyAction = historyRejected
			message = "Surat berhasil ditolak"
			letter.Status = statusRejected
			approval.Status = approvalRejected
			approval.ApproverID = &userID
			approval.Notes = req.Notes
			approval.ApprovedAt = &now
			notificationNotes = req.Notes

		case "forward":
			historyAction = historyForwarded
			message = "Surat berhasil diteruskan"
			official, roleCode, err := s.resolveForwardTarget(tx, req)
			if err != nil {
				return err
			}
			if letter.Status == statusForwarded && letter.SignedByID != nil && *letter.SignedByID == official.ID {
				return errs.BadRequest("Pejabat tujuan harus berbeda dari pejabat saat ini")
			}

			// Move approval stage to selected official role (pending).
			role, err := s.UsersRepo.GetRoleByCode(tx, roleCode)
			if err != nil {
				return errs.InternalServerError("Gagal memvalidasi role penandatangan")
			}

			letter.Status = statusForwarded
			letter.SignedByID = &official.ID
			approval.RoleID = role.ID
			approval.Status = approvalPending
			approval.ApproverID = nil
			approval.Notes = req.Notes
			approval.ApprovedAt = nil
			notificationNotes = req.Notes

		case "approve":
			if letter.Status == statusForwarded {
				officialActor, err := s.UsersRepo.GetActiveOfficialByUserID(tx, userID)
				if err != nil {
					return errs.Forbidden("Data pejabat tidak ditemukan")
				}

				letter.Status = statusApproved
				letter.SignedByID = &officialActor.ID
				letter.SignedAt = nil
				approval.Status = approvalApproved
				approval.ApproverID = &userID
				approval.Notes = req.Notes
				approval.ApprovedAt = &now
				notificationNotes = req.Notes
				message = "Surat berhasil disetujui dan menunggu nomor surat"
				break
			}

			nomorSurat, err := buildLetterNumber(req.LetterNumber, letter.LetterType, now)
			if err != nil {
				return err
			}

			used, err := s.Repo.IsLetterNumberUsed(tx, nomorSurat, letter.ID)
			if err != nil {
				return errs.InternalServerError("Gagal memvalidasi nomor surat")
			}
			if used {
				return errs.BadRequest("Nomor surat sudah digunakan")
			}
			if letter.SignedByID == nil {
				return errs.BadRequest("Pejabat penandatangan belum tersedia")
			}
			official, err := s.Repo.GetOfficialByID(tx, *letter.SignedByID)
			if err != nil {
				log.Printf("approved official not found: official_id=%d err=%v", *letter.SignedByID, err)
				return errs.NotFound("Pejabat penandatangan tidak ditemukan")
			}

			template, err := s.Repo.GetTemplateByLetterType(tx, letter.LetterTypeID)
			if err != nil {
				log.Printf("template not found: letter_type_id=%d err=%v", letter.LetterTypeID, err)
				return errs.NotFound("Template surat tidak ditemukan")
			}
			templatePlaceholders := extractTemplatePlaceholdersForLetterType(template, letter.LetterTypeID)

			payloadMap, err := unmarshalPayload(letter.Payload)
			if err != nil {
				log.Printf("error unmarshaling payload: %v", err)
				return errs.InternalServerError("Terjadi kesalahan dalam membaca data surat")
			}

			payloadMap = buildApprovedPayload(payloadMap, nomorSurat, official)
			payloadJSON, err := marshalPayload(payloadMap)
			if err != nil {
				log.Printf("error marshaling approved payload: %v", err)
				return errs.InternalServerError("Terjadi kesalahan dalam memperbarui data surat")
			}

			output := fmt.Sprintf("public/generated/final_%d.docx", letter.ID)
			data := buildTemplateData(student, payloadMap)
			addOfficialTemplateData(data, official)
			imageKeys := templateImagePlaceholderKeys(templatePlaceholders)
			if len(imageKeys) > 0 {
				atts, err := s.Repo.GetAttachmentsByLetterID(tx, letterID)
				if err != nil {
					return errs.InternalServerError("Gagal mengambil data berkas")
				}
				addTemplateImageAttachments(data, atts, imageKeys)
			}
			if err := s.generateLetterDocument(template.FilePath, output, data); err != nil {
				return err
			}

			letter.Status = statusSigned
			letter.FilePath = output
			letter.Payload = payloadJSON
			letter.LetterNumber = &nomorSurat
			letter.SignedByID = &official.ID
			letter.SignedAt = &now

			notificationNotes = req.Notes
			historyAction = historyNumbered
			message = "Nomor surat berhasil disimpan"
		}

		if err := tx.Save(letter).Error; err != nil {
			return err
		}

		if err := s.Repo.SaveApproval(tx, approval); err != nil {
			log.Printf("error updating approval: %v", err)
			return errs.InternalServerError("Terjadi kesalahan dalam memperbarui approval surat")
		}

		if err := s.Repo.CreateHistory(tx, &migration.LetterHistory{LetterID: letter.ID, ActorID: userID, Action: historyAction, Notes: req.Notes}); err != nil {
			log.Printf("error creating letter history: %v", err)
			return errs.InternalServerError("Terjadi kesalahan dalam mencatat riwayat surat")
		}

		resultingStatus = letter.Status

		return nil
	})

	if err != nil {
		log.Printf("error approving letter: %v", err)
		return nil, err
	}

	// Best-effort: notify the letter owner about status changes.
	if studentUserID != 0 {
		title := "Status Surat Berubah"
		body := fmt.Sprintf("Status surat '%s' berubah menjadi %s", subject, resultingStatus)
		nType := "letter_status_changed"
		switch req.Action {
		case "approve":
			if resultingStatus == statusSigned {
				title = "Surat Selesai"
				body = fmt.Sprintf("Surat '%s' telah diberi nomor dan ditandatangani", subject)
				nType = "letter_signed"
			} else {
				title = "Surat Disetujui Pejabat"
				body = fmt.Sprintf("Surat '%s' telah disetujui dan menunggu nomor surat", subject)
				nType = "letter_approved"
			}
		case "reject":
			title = "Surat Ditolak"
			body = fmt.Sprintf("Surat '%s' ditolak", subject)
			if strings.TrimSpace(notificationNotes) != "" {
				body = body + ": " + strings.TrimSpace(notificationNotes)
			}
			nType = "letter_rejected"
		case "forward":
			title = "Surat Diteruskan"
			body = fmt.Sprintf("Surat '%s' telah diteruskan", subject)
			nType = "letter_forwarded"
		}

		ctx, cancel := context.WithTimeout(context.Background(), constants.ExternalServiceTimeout)
		defer cancel()
		if _, err := push.SendToUser(ctx, s.Repo.DB, studentUserID, title, body, map[string]string{
			"type":      nType,
			"letter_id": fmt.Sprint(letterID),
			"status":    resultingStatus,
		}); err != nil {
			log.Printf("push student notify failed: letter_id=%d student_user_id=%d err=%v", letterID, studentUserID, err)
		}
	}

	realtime.PublishTopics([]string{"letters", "letter-approvals"}, "letter-"+req.Action, letterID)

	return &Response{
		StatusCode: http.StatusOK,
		Message:    message,
		Data: PreviewResponse{
			ID:         letterID,
			PreviewURL: helpers.ToAbsoluteURL(fmt.Sprintf("/api/correspondence/preview/%d", letterID)),
		},
	}, nil
}

func (s *Service) DeleteLetter(letterID uint, userID uint, isAdmin bool) (*Response, error) {
	var docxPath string
	var attachmentPaths []string

	err := s.Repo.WithTx(func(tx *gorm.DB) error {
		letter, err := s.Repo.GetLetterWithTypeByID(tx, letterID)
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return errs.NotFound("Surat tidak ditemukan")
			}
			return err
		}

		if !isAdmin {
			student, err := s.UsersRepo.GetStudentByUserID(tx, userID)
			if err != nil {
				return errs.Forbidden("Hanya mahasiswa yang dapat menghapus surat")
			}
			if letter.StudentID != student.ID {
				return errs.Forbidden("Anda tidak memiliki akses ke surat ini")
			}
			if letter.Status == statusForwarded || letter.Status == statusApproved || letter.Status == statusSigned {
				return errs.BadRequest("Surat tidak dapat dihapus pada status saat ini")
			}
		}

		docxPath = letter.FilePath

		atts, err := s.Repo.GetAttachmentsByLetterID(tx, letterID)
		if err != nil {
			log.Printf("error fetching attachments for delete: letter_id=%d err=%v", letterID, err)
			return errs.InternalServerError("Gagal menghapus surat")
		}
		attachmentPaths = make([]string, 0, len(atts))
		for _, a := range atts {
			if a.FilePath != "" {
				attachmentPaths = append(attachmentPaths, a.FilePath)
			}
		}

		if err := s.Repo.DeleteAttachmentsByLetterID(tx, letterID); err != nil {
			log.Printf("error deleting attachments: letter_id=%d err=%v", letterID, err)
			return errs.InternalServerError("Gagal menghapus surat")
		}
		if err := s.Repo.DeleteHistoriesByLetterID(tx, letterID); err != nil {
			log.Printf("error deleting histories: letter_id=%d err=%v", letterID, err)
			return errs.InternalServerError("Gagal menghapus surat")
		}
		if err := s.Repo.DeleteApprovalsByLetterID(tx, letterID); err != nil {
			log.Printf("error deleting approvals: letter_id=%d err=%v", letterID, err)
			return errs.InternalServerError("Gagal menghapus surat")
		}
		if err := s.Repo.DeleteLetterByID(tx, letterID); err != nil {
			log.Printf("error deleting letter: id=%d err=%v", letterID, err)
			return errs.InternalServerError("Gagal menghapus surat")
		}

		return nil
	})
	if err != nil {
		return nil, err
	}

	// Best-effort file cleanup after transaction commit.
	pathsToRemove := make([]string, 0, 2+len(attachmentPaths))
	if strings.TrimSpace(docxPath) != "" {
		pathsToRemove = append(pathsToRemove, docxPath)
		pdfPath := strings.TrimSuffix(docxPath, filepath.Ext(docxPath)) + ".pdf"
		pathsToRemove = append(pathsToRemove, pdfPath)
	}
	pathsToRemove = append(pathsToRemove, attachmentPaths...)

	for _, p := range pathsToRemove {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if err := os.Remove(p); err != nil && !os.IsNotExist(err) {
			log.Printf("failed removing file %q: %v", p, err)
		}
	}

	realtime.PublishTopics([]string{"letters", "letter-approvals"}, "letter-deleted", letterID)

	return &Response{StatusCode: http.StatusOK, Message: "Surat berhasil dihapus"}, nil
}

func (s *Service) GetHistoryAndDetail(letterID uint, userID uint, isAdmin bool, isOfficial bool) (*Response, error) {
	var out []LetterHistoryItem
	var detail LetterHistoryDetail

	err := s.Repo.WithTx(func(tx *gorm.DB) error {
		letter, err := s.LettersRepo.GetLetterByID(tx, letterID)
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return errs.NotFound("Surat tidak ditemukan")
			}
			return err
		}

		if !isAdmin {
			if isOfficial {
				official, err := s.UsersRepo.GetActiveOfficialByUserID(tx, userID)
				if err != nil {
					return errs.Forbidden("Hanya pejabat yang dapat melihat riwayat surat ini")
				}
				if letter.SignedByID == nil || *letter.SignedByID != official.ID {
					return errs.Forbidden("Anda tidak memiliki akses ke surat ini")
				}
			} else {
				student, err := s.UsersRepo.GetStudentByUserID(tx, userID)
				if err != nil {
					return errs.Forbidden("Hanya pemilik surat yang dapat melihat riwayat")
				}
				if letter.StudentID != student.ID {
					return errs.Forbidden("Anda tidak memiliki akses ke surat ini")
				}
			}
		}

		student, err := s.UsersRepo.GetStudentByID(tx, letter.StudentID)
		if err != nil {
			log.Printf("error fetching student: letter_id=%d student_id=%d err=%v", letterID, letter.StudentID, err)
			return errs.InternalServerError("Gagal mengambil data mahasiswa pemohon")
		}

		histories, err := s.Repo.ListHistoriesByLetterID(tx, letterID)
		if err != nil {
			log.Printf("error fetching histories: letter_id=%d err=%v", letterID, err)
			return errs.InternalServerError("Gagal mengambil riwayat surat")
		}

		atts, err := s.Repo.GetAttachmentsByLetterID(tx, letterID)
		if err != nil {
			log.Printf("error fetching attachments: letter_id=%d err=%v", letterID, err)
			return errs.InternalServerError("Gagal mengambil data berkas")
		}
		items := make([]AttachmentItem, 0, len(atts))
		for _, a := range atts {
			items = append(items, AttachmentItem{
				ID:       a.ID,
				Key:      strings.TrimSpace(a.RequirementKey),
				FilePath: helpers.ToAbsoluteURL(a.FilePath),
				FileType: a.FileType,
			})
		}

		payloadMap, err := unmarshalPayload(letter.Payload)
		if err != nil {
			return errs.InternalServerError("Terjadi kesalahan dalam membaca data surat")
		}

		detail = LetterHistoryDetail{
			ID:           letter.ID,
			LetterTypeID: letter.LetterTypeID,
			LetterType: &LetterTypeSummary{
				ID:                 letter.LetterType.ID,
				Code:               letter.LetterType.Code,
				Name:               letter.LetterType.Name,
				WorkCode:           letter.LetterType.WorkCode,
				ClassificationCode: letter.LetterType.ClassificationCode,
			},
			Subject:      letter.Subject,
			Status:       letter.Status,
			LetterNumber: letter.LetterNumber,
			Payload:      payloadMap,
			Attachments:  items,
			Student: &StudentSummary{
				StudentID: student.ID,
				UserID:    student.UserID,
				Name:      student.User.Name,
				NIM:       student.NIM,
			},
			PreviewURL: helpers.ToAbsoluteURL(fmt.Sprintf("/api/correspondence/preview/%d", letter.ID)),
			CreatedAt:  letter.CreatedAt,
			UpdatedAt:  letter.UpdatedAt,
		}

		out = make([]LetterHistoryItem, 0, len(histories))
		for _, h := range histories {
			var actor *HistoryActor
			if h.Actor.ID != 0 {
				actor = &HistoryActor{UserID: h.Actor.ID, Name: h.Actor.Name}
			}
			out = append(out, LetterHistoryItem{
				ID:        h.ID,
				Action:    h.Action,
				Notes:     h.Notes,
				Actor:     actor,
				CreatedAt: h.CreatedAt,
			})
		}

		return nil
	})
	if err != nil {
		return nil, err
	}

	return &Response{StatusCode: http.StatusOK, Message: "Riwayat surat berhasil diambil", Data: LetterHistoryAndDetailData{Letter: detail, Histories: out}}, nil
}

func (s *Service) ListLetters(userID uint, isAdmin bool, q ListLettersQuery) (*Response, error) {
	allowedStatus := map[string]struct{}{
		"":              {},
		statusSubmitted: {},
		statusForwarded: {},
		statusApproved:  {},
		statusSigned:    {},
		statusRejected:  {},
	}
	if _, ok := allowedStatus[strings.TrimSpace(q.Status)]; !ok {
		return nil, errs.BadRequest("status tidak valid")
	}

	sort := strings.TrimSpace(q.Sort)
	if sort != "" && sort != "created_at_desc" && sort != "created_at_asc" {
		return nil, errs.BadRequest("sort tidak valid")
	}
	if sort == "" {
		sort = "created_at_desc"
	}

	page := q.Page
	if page <= 0 {
		page = 1
	}
	pageSize := q.PageSize
	if pageSize <= 0 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}

	var createdFrom *time.Time
	if strings.TrimSpace(q.CreatedFrom) != "" {
		tm, err := parseTimeOrDate(strings.TrimSpace(q.CreatedFrom))
		if err != nil {
			return nil, errs.BadRequest("created_from tidak valid")
		}
		createdFrom = &tm
	}
	var createdTo *time.Time
	if strings.TrimSpace(q.CreatedTo) != "" {
		tm, err := parseTimeOrDate(strings.TrimSpace(q.CreatedTo))
		if err != nil {
			return nil, errs.BadRequest("created_to tidak valid")
		}
		createdTo = &tm
	}
	if createdFrom != nil && createdTo != nil && createdFrom.After(*createdTo) {
		return nil, errs.BadRequest("range tanggal tidak valid")
	}

	var letterTypeID *uint
	if q.LetterType != 0 {
		letterTypeID = &q.LetterType
	}

	var items []LetterListItem
	var total int64

	err := s.Repo.WithTx(func(tx *gorm.DB) error {
		var studentID *uint
		if !isAdmin {
			student, err := s.UsersRepo.GetStudentByUserID(tx, userID)
			if err != nil {
				return errs.Forbidden("Hanya mahasiswa yang dapat melihat surat miliknya")
			}
			studentID = &student.ID
		}

		letters, count, err := s.Repo.ListLetters(tx, ListLettersParams{
			StudentID:   studentID,
			Query:       q.Q,
			Status:      strings.TrimSpace(q.Status),
			LetterType:  letterTypeID,
			CreatedFrom: createdFrom,
			CreatedTo:   createdTo,
			Sort:        sort,
			Page:        page,
			PageSize:    pageSize,
		})
		if err != nil {
			log.Printf("error listing letters: %v", err)
			return errs.InternalServerError("Gagal mengambil daftar surat")
		}
		total = count

		items = make([]LetterListItem, 0, len(letters))
		for _, l := range letters {
			previewURL := helpers.ToAbsoluteURL(fmt.Sprintf("/api/correspondence/preview/%d", l.ID))
			historyURL := helpers.ToAbsoluteURL(fmt.Sprintf("/api/correspondence/history/%d", l.ID))

			var student *StudentSummary
			if isAdmin {
				student = &StudentSummary{
					StudentID: l.Student.ID,
					UserID:    l.Student.User.ID,
					Name:      l.Student.User.Name,
					NIM:       l.Student.NIM,
				}
			}

			items = append(items, LetterListItem{
				ID:      l.ID,
				Subject: l.Subject,
				Status:  l.Status,
				LetterNo: func() *string {
					if l.LetterNumber == nil || strings.TrimSpace(*l.LetterNumber) == "" {
						return nil
					}
					return l.LetterNumber
				}(),
				LetterType: LetterTypeSummary{
					ID:                 l.LetterType.ID,
					Code:               l.LetterType.Code,
					Name:               l.LetterType.Name,
					WorkCode:           l.LetterType.WorkCode,
					ClassificationCode: l.LetterType.ClassificationCode,
				},
				Student:    student,
				PreviewURL: previewURL,
				HistoryURL: historyURL,
				CreatedAt:  l.CreatedAt,
				UpdatedAt:  l.UpdatedAt,
			})
		}

		return nil
	})
	if err != nil {
		return nil, err
	}

	return &Response{
		StatusCode: http.StatusOK,
		Message:    "Daftar surat berhasil diambil",
		Data: LetterListData{
			Items: items,
			Meta:  PaginationMeta{Page: page, PageSize: pageSize, Total: total},
		},
	}, nil
}

func parseTimeOrDate(value string) (time.Time, error) {
	if value == "" {
		return time.Time{}, fmt.Errorf("empty time")
	}
	if tm, err := time.Parse(time.RFC3339, value); err == nil {
		return tm, nil
	}
	// Fallback: yyyy-mm-dd
	return time.ParseInLocation("2006-01-02", value, time.UTC)
}

// private helpers

func (s *Service) resolveForwardTarget(tx *gorm.DB, req ApproveLetterRequest) (*migration.Official, string, error) {
	if req.TargetOfficialID != 0 {
		official, err := s.UsersRepo.GetActiveOfficialByID(tx, req.TargetOfficialID)
		if err != nil {
			return nil, "", errs.NotFound("Pejabat tujuan tidak ditemukan atau tidak aktif")
		}
		if err := policy.CanOfficialAct(&official.User, official); err != nil {
			return nil, "", err
		}
		roleCode := officialRoleCode(*official)
		if roleCode == "" {
			return nil, "", errs.BadRequest("Pejabat tujuan tidak memiliki role pejabat yang valid")
		}
		return official, roleCode, nil
	}

	return nil, "", errs.BadRequest("Atasan tujuan wajib dipilih")
}

func officialRoleCode(official migration.Official) string {
	for _, role := range official.User.Roles {
		code := strings.ToUpper(strings.TrimSpace(role.Code))
		if constants.IsOfficialRoleCode(code) {
			return code
		}
	}
	return ""
}

func validateLetterSubject(subject string) (string, error) {
	trimmed := strings.TrimSpace(subject)
	if trimmed == "" {
		return "", errs.BadRequest("Perihal surat wajib diisi")
	}
	length := len([]rune(trimmed))
	if length < 3 || length > maxLetterSubjectRunes {
		return "", errs.BadRequest("Perihal surat harus 3-150 karakter")
	}
	if !helpers.IsSafeHTML(trimmed) {
		return "", errs.BadRequest("Perihal surat mengandung karakter tidak aman")
	}
	return trimmed, nil
}

func validateSubmitPayload(payload map[string]any) error {
	if payload == nil {
		return errs.BadRequest("Data surat wajib diisi")
	}
	if len(payload) > maxLetterPayloadKeys {
		return errs.BadRequest("Data surat terlalu banyak")
	}

	for key, value := range payload {
		normalizedKey := strings.TrimSpace(key)
		if normalizedKey == "" {
			return errs.BadRequest("Key data surat tidak boleh kosong")
		}
		if !payloadKeyPattern.MatchString(normalizedKey) {
			return errs.BadRequest("Key data surat harus 2-50 karakter, diawali huruf kecil, dan hanya boleh berisi huruf kecil, angka, atau underscore")
		}
		if _, blocked := blockedPayloadFields[normalizedKey]; blocked {
			continue
		}
		if err := validatePayloadValue(normalizedKey, value); err != nil {
			return err
		}
	}
	return nil
}

func validatePayloadValue(key string, value any) error {
	switch typed := value.(type) {
	case nil:
		return nil
	case string:
		trimmed := strings.TrimSpace(typed)
		if len([]rune(trimmed)) > maxLetterPayloadValueRunes {
			return errs.BadRequest("Nilai data surat maksimal 1000 karakter")
		}
		if !helpers.IsSafeHTML(trimmed) {
			return errs.BadRequest("Nilai data surat mengandung karakter tidak aman")
		}
	case []any:
		if key != "mahasiswa_lain" {
			return errs.BadRequest("Data list hanya diperbolehkan untuk mahasiswa_lain")
		}
		if len(typed) > 20 {
			return errs.BadRequest("Mahasiswa tambahan maksimal 20 orang")
		}
		for _, item := range typed {
			row, ok := item.(map[string]any)
			if !ok {
				return errs.BadRequest("Format mahasiswa tambahan tidak valid")
			}
			name := strings.TrimSpace(fmt.Sprint(row["name"]))
			nim := strings.TrimSpace(fmt.Sprint(row["nim"]))
			if len([]rune(name)) < 3 || len([]rune(name)) > 100 || !helpers.IsSafeHTML(name) {
				return errs.BadRequest("Nama mahasiswa tambahan harus 3-100 karakter dan tidak mengandung karakter tidak aman")
			}
			if !additionalStudentNIMPattern.MatchString(nim) {
				return errs.BadRequest("NIM mahasiswa tambahan harus 6-20 digit")
			}
		}
	default:
		return errs.BadRequest("Nilai data surat harus berupa teks")
	}
	return nil
}

func validateAttachmentInput(key string, file *multipart.FileHeader) error {
	if !attachmentKeyPattern.MatchString(key) {
		return errs.BadRequest("Key berkas harus 2-50 karakter, diawali huruf kecil, dan hanya boleh berisi huruf kecil, angka, atau underscore")
	}
	if file == nil {
		return errs.BadRequest("file tidak ditemukan")
	}
	if file.Size <= 0 {
		return errs.BadRequest("file berkas kosong")
	}
	if file.Size > maxAttachmentBytes {
		return errs.BadRequest("ukuran berkas maksimal 5MB")
	}

	ext := strings.ToLower(filepath.Ext(file.Filename))
	if ext != ".pdf" && ext != ".png" && ext != ".jpg" && ext != ".jpeg" {
		return errs.BadRequest("berkas harus berformat PDF, PNG, JPG, atau JPEG")
	}
	return nil
}

func validateReviewLetterRequest(req ApproveLetterRequest) error {
	notes := strings.TrimSpace(req.Notes)
	if len([]rune(notes)) > maxActionNotesRunes {
		return errs.BadRequest("Catatan maksimal 500 karakter")
	}
	if notes != "" && !helpers.IsSafeHTML(notes) {
		return errs.BadRequest("Catatan mengandung karakter tidak aman")
	}
	if req.Action == "reject" && len([]rune(notes)) < 10 {
		return errs.BadRequest("Catatan penolakan minimal 10 karakter")
	}
	if req.Action == "forward" && req.TargetOfficialID == 0 {
		return errs.BadRequest("Atasan tujuan wajib dipilih")
	}
	return nil
}

func (s *Service) generateLetterDocument(templatePath string, outputPath string, data map[string]string) error {
	if err := helpers.FillTemplate(templatePath, outputPath, data); err != nil {
		log.Printf("fill template failed: src=%q dst=%q err=%v", templatePath, outputPath, err)
		return errs.InternalServerError("Gagal mengisi template surat")
	}

	if leftover, err := helpers.ExtractDocxPlaceholders(outputPath); err == nil {
		if len(leftover) > 0 {
			return errs.BadRequestWithData("Data surat belum lengkap", missingTemplateFieldsData{Missing: leftover})
		}
	} else {
		log.Printf("failed validating generated docx placeholders: path=%q err=%v", outputPath, err)
		return errs.InternalServerError("Gagal memvalidasi hasil surat")
	}

	if err := helpers.ConvertToPDF(outputPath); err != nil {
		log.Printf("convert pdf failed: path=%q err=%v", outputPath, err)
		return errs.InternalServerError("Gagal membuat preview surat")
	}

	return nil
}

func copyPayload(payload map[string]any, blocked map[string]struct{}) map[string]any {
	cloned := make(map[string]any, len(payload))
	for key, value := range payload {
		if blocked != nil {
			if _, isBlocked := blocked[key]; isBlocked {
				continue
			}
		}
		cloned[key] = value
	}
	return cloned
}

func buildSubmitPayload(payload map[string]any) map[string]any {
	return copyPayload(payload, blockedPayloadFields)
}

func ensureLetterSystemPayload(payload map[string]any) {
	if raw, ok := payload["tahun_ajaran"]; !ok || strings.TrimSpace(fmt.Sprint(raw)) == "" {
		payload["tahun_ajaran"] = helpers.GetCurrentAcademicYear()
	}
}

func normalizeLetterNumberSegment(value string) string {
	segment := strings.ToUpper(strings.TrimSpace(value))
	segment = strings.ReplaceAll(segment, " ", ".")
	segment = strings.Trim(segment, ".")
	return segment
}

func normalizeLetterNumberWorkCode(value string) string {
	workCode := normalizeLetterNumberSegment(value)
	if workCode == "UN12.2" {
		return ""
	}
	return strings.TrimPrefix(workCode, "UN12.2.")
}

func buildLetterNumber(sequence string, letterType migration.LetterType, now time.Time) (string, error) {
	seq := strings.TrimSpace(sequence)
	if seq == "" {
		return "", errs.BadRequest("Nomor urut surat wajib diisi")
	}
	if strings.Contains(seq, "/") {
		return "", errs.BadRequest("Isi nomor urut saja. Format nomor surat lengkap dibuat otomatis oleh sistem")
	}
	if !letterNumberSeqPattern.MatchString(seq) {
		return "", errs.BadRequest("Nomor urut surat harus 1-6 digit angka")
	}

	workCode := normalizeLetterNumberWorkCode(letterType.WorkCode)
	classificationCode := normalizeLetterNumberSegment(letterType.ClassificationCode)
	if workCode == "" {
		return "", errs.BadRequest("Kode kerja pada template surat belum diisi")
	}
	if classificationCode == "" {
		return "", errs.BadRequest("Kode klasifikasi pada template surat belum diisi")
	}
	if strings.Contains(workCode, "/") || strings.Contains(classificationCode, "/") {
		return "", errs.BadRequest("Kode kerja dan kode klasifikasi tidak boleh mengandung karakter /")
	}

	return fmt.Sprintf("%s/UN12.2.%s/%s/%d", seq, workCode, classificationCode, now.Year()), nil
}

func currentIndonesianDateParts(now time.Time) map[string]string {
	days := []string{
		"Minggu", "Senin", "Selasa", "Rabu", "Kamis", "Jumat", "Sabtu",
	}
	months := []string{
		"", "Januari", "Februari", "Maret", "April", "Mei", "Juni",
		"Juli", "Agustus", "September", "Oktober", "November", "Desember",
	}

	return map[string]string{
		"hari":    days[now.Weekday()],
		"tanggal": fmt.Sprintf("%d", now.Day()),
		"bulan":   months[now.Month()],
		"tahun":   fmt.Sprintf("%d", now.Year()),
	}
}

func templateImagePlaceholderKeys(placeholders []string) []string {
	out := make([]string, 0)
	seen := make(map[string]struct{})
	for _, placeholder := range placeholders {
		key := strings.TrimSpace(placeholder)
		if !helpers.IsTemplateImagePlaceholderKey(key) {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, key)
	}
	return out
}

func addTemplateImageAttachments(data map[string]string, atts []migration.LetterAttachment, imageKeys []string) {
	wanted := make(map[string]struct{}, len(imageKeys))
	for _, key := range imageKeys {
		wanted[key] = struct{}{}
	}
	for _, att := range atts {
		key := strings.TrimSpace(att.RequirementKey)
		if _, ok := wanted[key]; !ok {
			continue
		}
		data[key] = helpers.DocxImage(att.FilePath)
	}
}

func (s *Service) ensureAttachmentKeysPresent(tx *gorm.DB, letterID uint, keys []string) error {
	if len(keys) == 0 {
		return nil
	}
	haveKeys, err := s.Repo.ListAttachmentKeysByLetterID(tx, letterID)
	if err != nil {
		return errs.InternalServerError("Gagal memeriksa berkas")
	}
	have := make(map[string]struct{}, len(haveKeys))
	for _, key := range haveKeys {
		have[strings.TrimSpace(key)] = struct{}{}
	}
	missing := make([]string, 0)
	for _, key := range keys {
		if _, ok := have[key]; !ok {
			missing = append(missing, key)
		}
	}
	if len(missing) > 0 {
		return errs.BadRequestWithData("Berkas wajib belum lengkap", missingAttachmentsData{Missing: missing})
	}
	return nil
}

func buildApprovedPayload(payload map[string]any, letterNumber string, official *migration.Official) map[string]any {
	enriched := copyPayload(payload, nil)

	ensureLetterSystemPayload(enriched)

	enriched["nomor_surat"] = letterNumber
	enriched["official"] = official.User.Name
	enriched["nip"] = official.NIP
	enriched["pangkat"] = official.Pangkat
	enriched["jabatan"] = official.Jabatan

	return enriched
}

func addOfficialTemplateData(data map[string]string, official *migration.Official) {
	if data == nil || official == nil {
		return
	}
	data["official"] = official.User.Name
	data["nip"] = official.NIP
	data["pangkat"] = official.Pangkat
	data["jabatan"] = official.Jabatan
	data["ttd"] = official.User.Name
	data["tanda_tangan"] = helpers.DocxImage(official.Signature)
	data["signature"] = helpers.DocxImage(official.Signature)
}

func additionalStudentRowsFromPayload(payload map[string]any) []helpers.DocxStudentTableRow {
	raw, ok := payload["mahasiswa_lain"]
	if !ok || raw == nil {
		return nil
	}

	rows := make([]helpers.DocxStudentTableRow, 0)
	stringValue := func(value any) string {
		if value == nil {
			return ""
		}
		return strings.TrimSpace(fmt.Sprint(value))
	}
	appendRow := func(nameValue, nimValue any) {
		name := stringValue(nameValue)
		nim := stringValue(nimValue)
		if name == "" && nim == "" {
			return
		}
		rows = append(rows, helpers.DocxStudentTableRow{Name: name, NIM: nim})
	}

	switch value := raw.(type) {
	case []any:
		for _, item := range value {
			if row, ok := item.(map[string]any); ok {
				appendRow(row["name"], row["nim"])
			}
		}
	case []map[string]any:
		for _, row := range value {
			appendRow(row["name"], row["nim"])
		}
	case []helpers.DocxStudentTableRow:
		for _, row := range value {
			appendRow(row.Name, row.NIM)
		}
	}

	return rows
}

func buildTemplateData(student *migration.Student, payload map[string]any) map[string]string {
	now := time.Now()
	data := map[string]string{
		"mahasiswa":             student.User.Name,
		"nim":                   student.NIM,
		"program_studi":         student.ProgramStudi,
		"angkatan":              fmt.Sprintf("%d/%d", student.Angkatan, student.Angkatan+1),
		"semester_masuk_kuliah": student.SemesterMasukKuliah,
		"tabel_data_mahasiswa": helpers.DocxStudentTable(append(
			[]helpers.DocxStudentTableRow{{Name: student.User.Name, NIM: student.NIM}},
			additionalStudentRowsFromPayload(payload)...,
		)),
	}
	for key, value := range currentIndonesianDateParts(now) {
		data[key] = value
	}

	for key, value := range payload {
		if _, blocked := studentTemplateFields[key]; blocked {
			continue
		}
		if value == nil {
			data[key] = ""
			continue
		}
		data[key] = strings.TrimSpace(fmt.Sprint(value))
	}

	return data
}

func marshalPayload(payload map[string]any) (datatypes.JSON, error) {
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	return datatypes.JSON(payloadBytes), nil
}

func unmarshalPayload(payload datatypes.JSON) (map[string]any, error) {
	decoded := make(map[string]any)
	if len(payload) == 0 {
		return decoded, nil
	}
	if err := json.Unmarshal(payload, &decoded); err != nil {
		return nil, err
	}
	return decoded, nil
}
