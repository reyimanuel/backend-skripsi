package letters

import (
	"log"

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

func (s *Service) Create(userID uint, req CreateLetterRequest) (*Response, error) {
	var letter *migration.Letter

	err := s.Repo.WithTx(func(tx *gorm.DB) error {

		student, err := s.Repo.GetStudentByUserID(tx, userID)
		if err != nil {
			return errs.Forbidden("Hanya mahasiswa yang dapat mengajukan surat")
		}

		letter = &migration.Letter{
			StudentID:    student.ID,
			LetterTypeID: req.LetterTypeID,
			Subject:      req.Subject,
			Payload:      datatypes.JSON(req.Payload),
			Status:       "submitted",
		}

		if err := s.Repo.CreateLetter(tx, letter); err != nil {
			log.Printf("error membuat surat: %v", err)
			return errs.InternalServerError("Gagal membuat surat")
		}

		if err := s.Repo.CreateHistory(tx, &migration.LetterHistory{LetterID: letter.ID, ActorID: userID, Action: "SUBMITTED"}); err != nil {
			log.Printf("error membuat history surat: %v", err)
			return errs.InternalServerError("Gagal membuat history surat")
		}

		adminRole, _ := s.Repo.GetRoleByCode(tx, "ADMIN")
		if err := s.Repo.CreateApproval(tx, &migration.LetterApproval{LetterID: letter.ID, RoleID: adminRole.ID, Status: "pending"}); err != nil {
			log.Printf("error membuat approval surat: %v", err)
			return errs.InternalServerError("Gagal membuat approval surat")
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	return &Response{
		StatusCode: 200,
		Message:    "Surat berhasil dibuat",
		Data: Data{
			ID:           letter.ID,
			LetterTypeID: letter.LetterTypeID,
			Subject:      letter.Subject,
			Status:       letter.Status,
			CreatedAt:    letter.CreatedAt,
		},
	}, nil
}
