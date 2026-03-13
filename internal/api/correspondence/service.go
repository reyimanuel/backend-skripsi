package correspondence

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"path/filepath"
	"time"

	"github.com/reyimanuel/letter-administration/internal/api/letters"
	user "github.com/reyimanuel/letter-administration/internal/api/users"
	"github.com/reyimanuel/letter-administration/internal/infrastructures/pkg/errs"
	"github.com/reyimanuel/letter-administration/internal/infrastructures/pkg/helpers"
	"github.com/reyimanuel/letter-administration/internal/infrastructures/pkg/policy"
	"github.com/reyimanuel/letter-administration/internal/migration"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

const (
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

func (s *Service) PreviewLetter(letterID uint, userID uint, isAdmin bool) (string, string, error) {
	var filePath string

	err := s.Repo.WithTx(func(tx *gorm.DB) error {
		letter, err := s.LettersRepo.GetLetterByID(tx, letterID)
		if err != nil {
			log.Printf("letter not found: id=%d err=%v", letterID, err)
			return errs.NotFound("Surat tidak ditemukan")
		}

		if !isAdmin {
			student, err := s.UsersRepo.GetStudentByUserID(tx, userID)
			if err != nil {
				log.Printf("student not found for preview: user_id=%d err=%v", userID, err)
				return errs.Forbidden("Hanya pemilik surat yang dapat melihat preview")
			}

			if letter.StudentID != student.ID {
				return errs.Forbidden("Anda tidak memiliki akses ke surat ini")
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

func (s *Service) CreateSubmitLetter(userID uint, req SubmitLetterRequest) (*Response, error) {
	var letter *migration.Letter
	var payloadMap map[string]any
	var outputDocx string

	err := s.Repo.WithTx(func(tx *gorm.DB) error {
		now := time.Now()

		student, err := s.UsersRepo.GetStudentByUserID(tx, userID)
		if err != nil {
			return errs.Forbidden("Hanya mahasiswa yang dapat mengajukan surat")
		}
		if err := policy.CanStudentSubmitLetter(&student.User, student); err != nil {
			return err
		}

		template, err := s.Repo.GetTemplateByLetterType(tx, req.LetterTypeID)
		if err != nil {
			return errs.NotFound("Template surat tidak ditemukan")
		}

		payloadMap = buildSubmitPayload(req.Payload, now, helpers.GetCurrentAcademicYear())
		data := buildTemplateData(student, payloadMap)

		outputDocx = fmt.Sprintf("public/generated/letter_%d.docx", now.UnixNano())

		if err := s.generateLetterDocument(template.FilePath, outputDocx, data); err != nil {
			return err
		}

		payloadJSON, err := marshalPayload(payloadMap)
		if err != nil {
			log.Printf("error marshaling payload: %v", err)
			return errs.InternalServerError("Terjadi kesalahan dalam membuat surat")
		}

		letter = &migration.Letter{
			StudentID:    student.ID,
			LetterTypeID: req.LetterTypeID,
			Subject:      req.Subject,
			Payload:      payloadJSON,
			Status:       statusSubmitted,
			FilePath:     outputDocx,
		}

		if err := s.Repo.CreateLetter(tx, letter); err != nil {
			log.Printf("error creating letter submission: %v", err)
			return errs.InternalServerError("Terjadi kesalahan dalam membuat submisi surat")
		}

		if err := s.Repo.CreateHistory(tx, &migration.LetterHistory{LetterID: letter.ID, ActorID: userID, Action: historySubmitted}); err != nil {
			log.Printf("error creating letter history: %v", err)
			return errs.InternalServerError("Terjadi kesalahan dalam membuat surat")
		}

		adminRole, err := s.UsersRepo.GetRoleByCode(tx, "ADMIN")
		if err != nil {
			log.Printf("error getting admin role: %v", err)
			return errs.InternalServerError("Gagal mendapatkan role admin")
		}

		if err := s.Repo.CreateApproval(tx, &migration.LetterApproval{LetterID: letter.ID, RoleID: adminRole.ID, Status: approvalPending}); err != nil {
			log.Printf("error creating letter approval: %v", err)
			return errs.InternalServerError("Terjadi kesalahan dalam membuat surat")
		}

		return nil
	})

	if err != nil {
		log.Printf("error creating letter submission: %v", err)
		return nil, err
	}

	return &Response{
		StatusCode: http.StatusOK,
		Message:    "Surat berhasil dibuat",
		Data: Data{
			ID:           letter.ID,
			LetterTypeID: letter.LetterTypeID,
			Subject:      letter.Subject,
			Status:       letter.Status,
			Payload:      payloadMap,
			FilePath:     "/" + outputDocx,
			PreviewURL:   fmt.Sprintf("/api/correspondence/preview/%d", letter.ID),
			CreatedAt:    letter.CreatedAt,
		},
	}, nil
}

func (s *Service) ApproveLetter(letterID uint, userID uint, req ApproveLetterRequest) (*Response, error) {
	_, historyAction, message := approvalResult(req.Action)

	err := s.Repo.WithTx(func(tx *gorm.DB) error {
		now := time.Now()

		letter, err := s.LettersRepo.GetLetterByID(tx, letterID)
		if err != nil {
			log.Printf("letter not found: id=%d err=%v", letterID, err)
			return errs.NotFound("Surat tidak ditemukan")
		}

		if letter.Status != statusSubmitted && letter.Status != statusForwarded {
			return errs.BadRequest("Surat tidak dalam status yang dapat disetujui")
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

		case "forward":
			official, err := s.resolveOfficial(tx, req.SignedByRole)
			if err != nil {
				return err
			}

			letter.Status = statusForwarded
			letter.SignedByID = &official.ID
			approval.Status = approvalApproved
			approval.ApproverID = &userID
			approval.Notes = req.Notes
			approval.ApprovedAt = &now

		case "approve":
			official, err := s.resolveOfficial(tx, req.SignedByRole)
			if err != nil {
				return err
			}

			sequence, err := s.Repo.CountApprovedThisYear(tx)
			if err != nil {
				return errs.InternalServerError("Gagal generate nomor surat")
			}

			nomorSurat := helpers.GenerateLetterNumber(sequence + 1)

			student, err := s.UsersRepo.GetStudentByID(tx, letter.StudentID)
			if err != nil {
				log.Printf("student not found: id=%d err=%v", letter.StudentID, err)
				return errs.NotFound("Data mahasiswa tidak ditemukan")
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

		return nil
	})

	if err != nil {
		log.Printf("error approving letter: %v", err)
		return nil, err
	}

	return &Response{
		StatusCode: http.StatusOK,
		Message:    message,
		Data: PreviewResponse{
			ID:         letterID,
			PreviewURL: fmt.Sprintf("/api/correspondence/preview/%d", letterID),
		},
	}, nil
}

// private helpers

func (s *Service) resolveOfficial(tx *gorm.DB, signedByRole string) (*migration.Official, error) {
	if signedByRole == "" {
		return nil, errs.BadRequest("Penandatangan wajib dipilih")
	}

	official, err := s.UsersRepo.GetActiveOfficialByRole(tx, signedByRole)
	if err != nil {
		log.Printf("official not found: jabatan=%q err=%v", signedByRole, err)
		return nil, errs.NotFound("Pejabat dengan jabatan '" + signedByRole + "' tidak ditemukan atau tidak aktif")
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

	if err := helpers.ConvertToPDF(outputPath); err != nil {
		log.Printf("convert pdf failed: path=%q err=%v", outputPath, err)
		return errs.InternalServerError("Gagal membuat preview surat")
	}

	return nil
}

func sanitizePayload(payload map[string]any) map[string]any {
	clean := make(map[string]any, len(payload))
	for key, value := range payload {
		if _, blocked := blockedPayloadFields[key]; blocked {
			continue
		}
		clean[key] = value
	}
	return clean
}

func clonePayload(payload map[string]any) map[string]any {
	cloned := make(map[string]any, len(payload))
	for key, value := range payload {
		cloned[key] = value
	}
	return cloned
}

func buildSubmitPayload(payload map[string]any, generatedAt time.Time, academicYear string) map[string]any {
	enriched := clonePayload(sanitizePayload(payload))
	enriched["tanggal"] = helpers.FormatIndonesianDate(generatedAt)
	enriched["tahun_ajaran"] = academicYear
	return enriched
}

func buildApprovedPayload(payload map[string]any, approvedAt time.Time, letterNumber string, official *migration.Official) map[string]any {
	enriched := clonePayload(payload)
	enriched["tanggal"] = helpers.FormatIndonesianDate(approvedAt)
	enriched["nomor_surat"] = letterNumber
	enriched["official"] = official.User.Name
	enriched["nip"] = official.NIP
	enriched["pangkat"] = official.Pangkat
	enriched["jabatan"] = official.Jabatan
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
		data[key] = stringifyTemplateValue(value)
	}

	return data
}

func stringifyTemplateValue(value any) string {
	if value == nil {
		return ""
	}
	return fmt.Sprint(value)
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

func approvalResult(action string) (status string, historyAction string, message string) {
	switch action {
	case "reject":
		return statusRejected, historyRejected, "Surat berhasil ditolak"
	case "forward":
		return statusForwarded, historyForwarded, "Surat berhasil diteruskan"
	default:
		return statusApproved, historyApproved, "Surat berhasil disetujui"
	}
}
