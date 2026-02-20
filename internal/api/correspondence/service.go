package correspondence

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/reyimanuel/letter-administration/internal/api/letters"
	user "github.com/reyimanuel/letter-administration/internal/api/users"
	"github.com/reyimanuel/letter-administration/internal/infrastructures/pkg/errs"
	"github.com/reyimanuel/letter-administration/internal/infrastructures/pkg/helpers"
	"github.com/reyimanuel/letter-administration/internal/migration"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

type Service struct {
	Repo        *Repository
	LettersRepo *letters.Repository
	UsersRepo   *user.Repository
}

func NewService(repo *Repository, lettersRepo *letters.Repository, usersRepo *user.Repository) *Service {
	return &Service{Repo: repo, LettersRepo: lettersRepo, UsersRepo: usersRepo}
}

func (s *Service) CreateSubmitLetter(userID uint, req SubmitLetterRequest) (*Response, error) {
	var letter *migration.Letter
	var outputPDF string

	err := s.Repo.WithTx(func(tx *gorm.DB) error {

		student, err := s.UsersRepo.GetStudentByUserID(tx, userID)
		if err != nil {
			return errs.Forbidden("Hanya mahasiswa yang dapat mengajukan surat")
		}

		template, err := s.Repo.GetTemplateByLetterType(tx, req.LetterTypeID)
		if err != nil {
			return errs.NotFound("Template surat tidak ditemukan")
		}

		data := map[string]string{
			"nama":          student.User.Name,
			"nim":           student.NIM,
			"program_studi": student.ProgramStudi,
			"angkatan":      fmt.Sprintf("%d/%d", student.Angkatan, student.Angkatan+1),
			"tanggal":       time.Now().Format("19 Januari 2005"),
			"tahun_ajaran":  helpers.GetCurrentAcademicYear(),
		}

		for k, v := range req.Payload {
			data[k] = fmt.Sprintf("%v", v)
		}

		outputDocx := fmt.Sprintf("storage/generated/letter_%d.docx", time.Now().Unix())
		outputPDF := fmt.Sprintf("storage/generated/letter_%d.pdf", time.Now().Unix())

		if err := helpers.FillTemplate(template.FilePath, outputDocx, data); err != nil {
			return err
		}

		if err := helpers.ConvertToPDF(outputDocx); err != nil {
			return err
		}

		letter.FilePath = outputPDF
		if err := tx.Save(letter).Error; err != nil {
			return err
		}

		payloadBytes, err := json.Marshal(req.Payload)
		if err != nil {
			log.Printf("error marshaling payload: %v", err)
			return errs.InternalServerError("Terjadi kesalahan dalam membuat surat")
		}

		letter = &migration.Letter{
			StudentID:    student.ID,
			LetterTypeID: req.LetterTypeID,
			Subject:      req.Subject,
			Payload:      datatypes.JSON(payloadBytes),
			Status:       "submitted",
			FilePath:     outputPDF,
		}

		if err := s.Repo.CreateLetter(tx, letter); err != nil {
			log.Printf("error creating letter submission: %v", err)
			return errs.InternalServerError("Terjadi kesalahan dalam membuat submisi surat")
		}

		if err := s.Repo.CreateHistory(tx, &migration.LetterHistory{LetterID: letter.ID, ActorID: userID, Action: "SUBMITTED"}); err != nil {
			log.Printf("error creating letter history: %v", err)
			return errs.InternalServerError("Terjadi kesalahan dalam membuat surat")
		}

		adminRole, err := s.UsersRepo.GetRoleByCode(tx, "ADMIN")
		if err != nil {
			log.Printf("error getting admin role: %v", err)
			return errs.InternalServerError("Gagal mendapatkan role admin")
		}

		if err := s.Repo.CreateApproval(tx, &migration.LetterApproval{LetterID: letter.ID, RoleID: adminRole.ID, Status: "pending"}); err != nil {
			log.Printf("error creating letter approval: %v", err)
			return errs.InternalServerError("Terjadi kesalahan dalam membuat surat")
		}

		return nil
	})

	if err != nil {
		log.Printf("error creating letter submission: %v", err)
		return nil, errs.InternalServerError("Terjadi kesalahan dalam membuat surat")
	}

	return &Response{
		StatusCode: http.StatusOK,
		Message:    "Surat berhasil dibuat",
		Data: Data{
			ID:           letter.ID,
			LetterTypeID: letter.LetterTypeID,
			Subject:      letter.Subject,
			Status:       letter.Status,
			FilePath:     "/" + outputPDF,
			CreatedAt:    letter.CreatedAt,
		},
	}, nil
}

func (s *Service) ApproveLetter(letterID uint, userID uint, req ApproveLetterRequest) (*Response, error) {
	err := s.Repo.WithTx(func(tx *gorm.DB) error {
		letter, err := s.LettersRepo.GetLetterByID(tx, letterID)
		if err != nil {
			return errs.NotFound("Surat tidak ditemukan")
		}

		if letter.Status != "submitted" && letter.Status != "forwarded" {
			return errs.BadRequest("Surat tidak dalam status yang dapat disetujui")
		}

		switch req.Action {

		case "reject":
			letter.Status = "rejected"

		case "forward":

			if req.SignedByRole == "" {
				return errs.BadRequest("Penandatangan wajib dipilih")
			}

			official, err := s.UsersRepo.GetActiveOfficialByRole(tx, req.SignedByRole)
			if err != nil {
				return errs.NotFound("Pejabat tidak ditemukan")
			}

			letter.Status = "forwarded"
			letter.SignedByID = &official.ID

		case "approve":

			if req.SignedByRole == "" {
				return errs.BadRequest("Penandatangan wajib dipilih")
			}

			official, err := s.UsersRepo.GetActiveOfficialByRole(tx, req.SignedByRole)
			if err != nil {
				return errs.NotFound("Pejabat tidak ditemukan")
			}

			sequence, err := s.Repo.CountApprovedThisYear(tx)
			if err != nil {
				return errs.InternalServerError("Gagal generate nomor surat")
			}

			nomorSurat := helpers.GenerateLetterNumber(sequence + 1)

			student, err := s.UsersRepo.GetStudentByID(tx, letter.StudentID)
			if err != nil {
				return errs.NotFound("Data mahasiswa tidak ditemukan")
			}

			data := map[string]string{
				"Mahasiswa":  student.User.Name,
				"NIM":        student.NIM,
				"Prodi":      student.ProgramStudi,
				"NomorSurat": nomorSurat,
				"NamaTtd":    official.User.Name,
				"nip":        official.NIP,
				"pangkat":    official.Pangkat,
				"jabatan":    official.Jabatan,
			}

			output := fmt.Sprintf("storage/generated/final_%d.docx", letter.ID)

			if err := helpers.FillTemplate(letter.FilePath, output, data); err != nil {
				return errs.InternalServerError("Gagal mengisi template")
			}

			now := time.Now()

			letter.Status = "approved"
			letter.FilePath = output
			letter.LetterNumber = &nomorSurat
			letter.SignedByID = &official.ID
			letter.SignedAt = &now
		}

		return tx.Save(letter).Error
	})

	if err != nil {
		log.Printf("error approving letter: %v", err)
		return nil, errs.InternalServerError("Terjadi kesalahan dalam menyetujui surat")
	}

	return &Response{
		StatusCode: http.StatusOK,
		Message:    "Surat berhasil disetujui",
	}, nil
}
