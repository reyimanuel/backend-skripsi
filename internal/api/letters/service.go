package letters

import (
	"github.com/reyimanuel/letter-administration/internal/infrastructures/pkg/errs"
	"github.com/reyimanuel/letter-administration/internal/migration"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

type Service struct {
	Repo *repository
}

func NewService(repo *repository) *Service {
	return &Service{Repo: repo}
}

func (s *Service) CreateLetter(userID uint, req CreateLetterRequest) (*migration.Letter, error) {
	return s.Repo.WithTx(func(tx *gorm.DB) (*migration.Letter, error) {

		student, err := s.Repo.GetStudentByUserID(tx, userID)
		if err != nil {
			return nil, errs.Forbidden("Hanya mahasiswa yang bisa mengajukan surat")
		}

		letter := migration.Letter{
			StudentID:    student.ID,
			LetterTypeID: req.LetterTypeID,
			Subject:      req.Subject,
			Payload:      datatypes.JSON(req.Payload),
			Status:       "submitted",
		}

		if err := tx.Create(&letter).Error; err != nil {
			return nil, errs.InternalServerError("Gagal membuat surat")
		}

		// History
		history := migration.LetterHistory{
			LetterID: letter.ID,
			ActorID:  userID,
			Action:   "SUBMITTED",
			Notes:    "Surat diajukan oleh mahasiswa",
		}
		tx.Create(&history)

		// Approval awal → ADMIN
		adminRole, _ := s.Repo.GetRoleByCode(tx, "ADMIN")

		approval := migration.LetterApproval{
			LetterID: letter.ID,
			RoleID:   adminRole.ID,
			Status:   "pending",
		}
		tx.Create(&approval)

		return &letter, nil
	})
}
