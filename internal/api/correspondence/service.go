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
	"slices"
	"strings"
	"time"

	"github.com/reyimanuel/letter-administration/internal/api/letters"
	"github.com/reyimanuel/letter-administration/internal/constants"
	"github.com/reyimanuel/letter-administration/internal/infrastructures/pkg/errs"
	"github.com/reyimanuel/letter-administration/internal/infrastructures/pkg/helpers"
	"github.com/reyimanuel/letter-administration/internal/infrastructures/pkg/policy"
	"github.com/reyimanuel/letter-administration/internal/infrastructures/pkg/push"
	user "github.com/reyimanuel/letter-administration/internal/api/users"
	"github.com/reyimanuel/letter-administration/internal/migration"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

const (
	statusDraft     = "draft"
	statusSubmitted = "submitted"
	statusForwarded = "forwarded"
	statusApproved  = "approved"
	statusRejected  = "rejected"

	approvalPending  = "pending"
	approvalApproved = "approved"
	approvalRejected = "rejected"

	historySubmitted = "SUBMITTED"
	historyForwarded = "FORWARDED"
	historyApproved  = "APPROVED"
	historyRejected  = "REJECTED"
)

var blockedPayloadFields = map[string]struct{}{
	"mahasiswa":     {},
	"nim":           {},
	"program_studi": {},
	"angkatan":      {},
	"tanggal":       {},
	"tahun_ajaran":  {},
	"nomor_surat":   {},
	"official":      {},
	"nip":           {},
	"pangkat":       {},
	"jabatan":       {},
}

var studentTemplateFields = map[string]struct{}{
	"mahasiswa":     {},
	"nim":           {},
	"program_studi": {},
	"angkatan":      {},
}

type Service struct {
	Repo        *Repository
	LettersRepo *letters.Repository
	UsersRepo   *user.Repository
}

func NewService(repo *Repository, lettersRepo *letters.Repository, usersRepo *user.Repository) *Service {
	return &Service{Repo: repo, LettersRepo: lettersRepo, UsersRepo: usersRepo}
}

func (s *Service) PreviewLetter(letterID uint, userID uint, isAdmin bool, isOfficial bool) (string, string, error) {
	var filePath string

	err := s.Repo.WithTx(func(tx *gorm.DB) error {
		letter, err := s.LettersRepo.GetLetterByID(tx, letterID)
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
	if status != statusForwarded && status != statusApproved && status != statusRejected {
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
				LetterType: LetterTypeSummary{ID: l.LetterType.ID, Code: l.LetterType.Code, Name: l.LetterType.Name},
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

	err := s.Repo.WithTx(func(tx *gorm.DB) error {
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
			Subject:      req.Subject,
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
			subject := strings.TrimSpace(*req.Subject)
			if subject == "" {
				return errs.BadRequest("subject tidak boleh kosong")
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
		letter, err := s.LettersRepo.GetLetterByID(tx, letterID)
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

		payloadMap["tanggal"] = helpers.FormatIndonesianDate(now)
		payloadMap["tahun_ajaran"] = helpers.GetCurrentAcademicYear()

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
		missingPayload := helpers.MissingPayloadKeys(payloadMap, ph.RequiredPayloadKeys)
		if len(missingPayload) > 0 {
			return errs.BadRequestWithData("Data surat belum lengkap", missingTemplateFieldsData{Missing: missingPayload})
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

func (s *Service) ApproveLetter(letterID uint, userID uint, req ApproveLetterRequest) (*Response, error) {
	historyAction := historyApproved
	message := "Surat berhasil disetujui"
	switch req.Action {
	case "reject":
		historyAction = historyRejected
		message = "Surat berhasil ditolak"
	case "forward":
		historyAction = historyForwarded
		message = "Surat berhasil diteruskan"
	}

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
		isOfficialRole := slices.Contains(roles, "DEKAN") || slices.Contains(roles, "WAKIL_DEKAN")

		letter, err := s.LettersRepo.GetLetterByID(tx, letterID)
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
			if !isAdmin {
				return errs.Forbidden("Hanya admin yang dapat meneruskan surat")
			}
			if letter.Status != statusSubmitted {
				return errs.BadRequest("Surat tidak dalam status yang dapat diteruskan")
			}
		case "approve", "reject":
			switch letter.Status {
			case statusSubmitted:
				if !isAdmin {
					return errs.Forbidden("Hanya admin yang dapat memproses surat pada tahap ini")
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
			letter.Status = statusRejected
			approval.Status = approvalRejected
			approval.ApproverID = &userID
			approval.Notes = req.Notes
			approval.ApprovedAt = &now
			notificationNotes = req.Notes

		case "forward":
			official, err := s.resolveOfficial(tx, req.SignedByRole)
			if err != nil {
				return err
			}

			// Move approval stage to selected official role (pending).
			normalized := strings.ToLower(strings.TrimSpace(req.SignedByRole))
			normalized = strings.ReplaceAll(normalized, "_", " ")
			normalized = strings.Join(strings.Fields(normalized), " ")
			roleCode := ""
			switch normalized {
			case "dekan":
				roleCode = "DEKAN"
			case "wakil dekan":
				roleCode = "WAKIL_DEKAN"
			}
			if roleCode == "" {
				return errs.BadRequest("Penandatangan tidak valid")
			}
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
			nomorSurat := strings.TrimSpace(req.LetterNumber)
			if nomorSurat == "" {
				return errs.BadRequest("Nomor surat wajib diisi saat approve")
			}

			used, err := s.Repo.IsLetterNumberUsed(tx, nomorSurat, letter.ID)
			if err != nil {
				return errs.InternalServerError("Gagal memvalidasi nomor surat")
			}
			if used {
				return errs.BadRequest("Nomor surat sudah digunakan")
			}

			var official *migration.Official
			if letter.Status == statusForwarded {
				officialActor, err := s.UsersRepo.GetActiveOfficialByUserID(tx, userID)
				if err != nil {
					return errs.Forbidden("Data pejabat tidak ditemukan")
				}
				official = officialActor
			} else {
				o, err := s.resolveOfficial(tx, req.SignedByRole)
				if err != nil {
					return err
				}
				official = o
			}

			template, err := s.Repo.GetTemplateByLetterType(tx, letter.LetterTypeID)
			if err != nil {
				log.Printf("template not found: letter_type_id=%d err=%v", letter.LetterTypeID, err)
				return errs.NotFound("Template surat tidak ditemukan")
			}

			payloadMap, err := unmarshalPayload(letter.Payload)
			if err != nil {
				log.Printf("error unmarshaling payload: %v", err)
				return errs.InternalServerError("Terjadi kesalahan dalam membaca data surat")
			}

			payloadMap = buildApprovedPayload(payloadMap, now, nomorSurat, official)
			payloadJSON, err := marshalPayload(payloadMap)
			if err != nil {
				log.Printf("error marshaling approved payload: %v", err)
				return errs.InternalServerError("Terjadi kesalahan dalam memperbarui data surat")
			}

			output := fmt.Sprintf("public/generated/final_%d.docx", letter.ID)
			if err := s.generateLetterDocument(template.FilePath, output, buildTemplateData(student, payloadMap)); err != nil {
				return err
			}

			letter.Status = statusApproved
			letter.FilePath = output
			letter.Payload = payloadJSON
			letter.LetterNumber = &nomorSurat
			letter.SignedByID = &official.ID
			letter.SignedAt = &now

			approval.Status = approvalApproved
			approval.ApproverID = &userID
			approval.Notes = req.Notes
			approval.ApprovedAt = &now
			notificationNotes = req.Notes
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
			title = "Surat Disetujui"
			body = fmt.Sprintf("Surat '%s' telah disetujui", subject)
			nType = "letter_approved"
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
		letter, err := s.LettersRepo.GetLetterByID(tx, letterID)
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
			if letter.Status == statusForwarded || letter.Status == statusApproved {
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
			Subject:      letter.Subject,
			Status:       letter.Status,
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
				LetterType: LetterTypeSummary{ID: l.LetterType.ID, Code: l.LetterType.Code, Name: l.LetterType.Name},
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

func (s *Service) resolveOfficial(tx *gorm.DB, signedByRole string) (*migration.Official, error) {
	if signedByRole == "" {
		return nil, errs.BadRequest("Penandatangan wajib dipilih")
	}

	// Only allow letter signers to be Dean / Vice Dean.
	// We accept a few client variants (case-insensitive, underscores/spaces).
	normalized := strings.ToLower(strings.TrimSpace(signedByRole))
	normalized = strings.ReplaceAll(normalized, "_", " ")
	normalized = strings.Join(strings.Fields(normalized), " ")
	if normalized != "dekan" && normalized != "wakil dekan" {
		return nil, errs.BadRequest("Penandatangan hanya boleh Dekan atau Wakil Dekan")
	}

	official, err := s.UsersRepo.GetActiveOfficialByRole(tx, normalized)
	if err != nil {
		log.Printf("official not found: jabatan=%q err=%v", normalized, err)
		return nil, errs.NotFound("Pejabat dengan jabatan '" + normalized + "' tidak ditemukan atau tidak aktif")
	}

	if err := policy.CanOfficialAct(&official.User, official); err != nil {
		return nil, err
	}

	return official, nil
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

func buildApprovedPayload(payload map[string]any, approvedAt time.Time, letterNumber string, official *migration.Official) map[string]any {
	enriched := copyPayload(payload, nil)

	if raw, ok := enriched["tahun_ajaran"]; !ok || strings.TrimSpace(fmt.Sprint(raw)) == "" {
		enriched["tahun_ajaran"] = helpers.GetCurrentAcademicYear()
	}

	enriched["tanggal"] = helpers.FormatIndonesianDate(approvedAt)
	enriched["nomor_surat"] = letterNumber
	enriched["official"] = official.User.Name
	enriched["nip"] = official.NIP
	enriched["pangkat"] = official.Pangkat
	enriched["jabatan"] = official.Jabatan

	// Optional placeholders (template-dependent).
	// For templates using {{tanda_tangan}}, FillTemplate will embed the signature image into the DOCX.
	enriched["ttd"] = official.User.Name
	enriched["tanda_tangan"] = helpers.DocxImage(official.Signature)
	enriched["signature"] = helpers.ToAbsoluteURL(official.Signature)
	return enriched
}

func buildTemplateData(student *migration.Student, payload map[string]any) map[string]string {
	data := map[string]string{
		"mahasiswa":     student.User.Name,
		"nim":           student.NIM,
		"program_studi": student.ProgramStudi,
		"angkatan":      fmt.Sprintf("%d/%d", student.Angkatan, student.Angkatan+1),
	}

	for key, value := range payload {
		if _, blocked := studentTemplateFields[key]; blocked {
			continue
		}
		if value == nil {
			data[key] = ""
			continue
		}
		data[key] = fmt.Sprint(value)
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
