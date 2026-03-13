package policy

import (
	"strings"

	"github.com/reyimanuel/letter-administration/internal/infrastructures/pkg/errs"
	"github.com/reyimanuel/letter-administration/internal/migration"
)

func CanStudentSubmitLetter(user *migration.User, student *migration.Student) error {
	if user == nil || student == nil || student.UserID != user.ID {
		return errs.Forbidden("Data mahasiswa tidak valid")
	}

	if !user.IsActive {
		return errs.Forbidden("Akun Anda tidak aktif")
	}

	if user.EmailVerifiedAt == nil {
		return errs.Forbidden("Email belum diverifikasi. Silakan verifikasi email terlebih dahulu")
	}

	switch strings.ToLower(strings.TrimSpace(student.AdminVerificationStatus)) {
	case "approved":
		return nil
	case "rejected":
		if strings.TrimSpace(student.RejectionReason) != "" {
			return errs.Forbidden("Verifikasi admin ditolak: " + student.RejectionReason)
		}
		return errs.Forbidden("Verifikasi admin ditolak")
	default:
		return errs.Forbidden("Akun mahasiswa belum diverifikasi admin")
	}
}

func CanOfficialAct(user *migration.User, official *migration.Official) error {
	if user == nil || official == nil || official.UserID != user.ID {
		return errs.Forbidden("Data official tidak valid")
	}

	if user.EmailVerifiedAt == nil {
		return errs.Forbidden("Email belum diverifikasi. Silakan verifikasi email terlebih dahulu")
	}

	if !official.IsOnDuty {
		return errs.Forbidden("Jabatan anda tidak aktif")
	}

	return nil
}
