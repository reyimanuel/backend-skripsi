package migration

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/reyimanuel/letter-administration/internal/infrastructures/pkg/helpers"
	"gorm.io/gorm"
)

type SeedTargets struct {
	Roles       bool
	Users       bool
	Students    bool
	Officials   bool
	LetterTypes bool
	Templates   bool

	// If true, runs the legacy TRUNCATE block (dev-only, destructive).
	TruncateAll bool
}

func (t *SeedTargets) Normalize() {
	// Dependencies.
	if t.Users || t.Students || t.Officials || t.Templates {
		t.Roles = true
	}
	if t.Students || t.Officials || t.Templates {
		t.Users = true
	}
	if t.Templates {
		t.LetterTypes = true
	}
}

// ParseSeedTargets parses a comma-separated list (or presets) into SeedTargets.
//
// Notes:
//   - Some targets expand into closely related datasets for convenience.
//     For example, "users" also enables seeding student + official records that
//     depend on users.
//   - Dependency normalization is handled by SeedTargets.Normalize().
//
// Examples:
// - "users"
// - "roles,users"
// - "all"
func ParseSeedTargets(spec string) (SeedTargets, error) {
	spec = strings.ToLower(strings.TrimSpace(spec))
	if spec == "" || spec == "all" {
		t := SeedTargets{Users: true, Students: true, Officials: true, LetterTypes: true, Templates: true}
		t.Normalize()
		return t, nil
	}

	var t SeedTargets
	for _, raw := range strings.Split(spec, ",") {
		p := strings.TrimSpace(strings.ToLower(raw))
		if p == "" {
			continue
		}
		switch p {
		case "users", "user":
			// Users is a "bundle" target: when you ask to seed users, you usually
			// want the main user-linked entities too.
			t.Roles = true
			t.Users = true
			t.Students = true
			t.Officials = true
		case "templates", "template":
			t.Templates = true
			t.LetterTypes = true
		default:
			return SeedTargets{}, fmt.Errorf("unknown seed target: %s", p)
		}
	}

	t.Normalize()
	return t, nil
}

func findAnyDocxTemplatePath() (string, error) {
	candidates := []string{
		filepath.Join("public", "letter-template"),
		filepath.Join("/app", "public", "letter-template"),
	}
	if exePath, err := os.Executable(); err == nil {
		exeDir := filepath.Dir(exePath)
		if exeDir != "" {
			candidates = append(candidates, filepath.Join(exeDir, "public", "letter-template"))
		}
	}

	var tried []string
	for _, dir := range candidates {
		tried = append(tried, dir)
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			if strings.EqualFold(filepath.Ext(e.Name()), ".docx") {
				return filepath.Join(dir, e.Name()), nil
			}
		}
	}

	return "", fmt.Errorf("no .docx template found; tried: %s", strings.Join(tried, ", "))
}

// SeedSelected allows seeding specific datasets (e.g. only users) and can be
// run without requiring templates unless Templates=true.
//
// Safety:
// - Seeding is blocked when DB_HOST isn't local unless force=true.
// - When TruncateAll=true it will TRUNCATE many tables (dev-only).
func SeedSelected(db *gorm.DB, force bool, targets SeedTargets) error {
	host := os.Getenv("DB_HOST")
	if !force && !strings.EqualFold(host, "localhost") && host != "127.0.0.1" {
		return fmt.Errorf("seeding blocked in production (DB_HOST=%s), use --force", host)
	}

	targets.Normalize()

	return db.Transaction(func(tx *gorm.DB) error {
		now := time.Now()
		photoPath := filepath.ToSlash(filepath.Join("public", "images", "profile-photos", "example.png"))
		signaturePath := filepath.ToSlash(filepath.Join("public", "images", "signatures", "signatures.png"))

		if targets.TruncateAll {
			if err := tx.Exec(`
				TRUNCATE TABLE 
					letter_attachments,
					letter_histories,
					letter_approvals,
					letters,
					letter_templates,
					letter_types,
					officials,
					students,
					user_roles,
					users,
					roles
				RESTART IDENTITY CASCADE
			`).Error; err != nil {
				return err
			}
		}

		var roleMap map[string]*Role
		if targets.Roles {
			m, err := ensureRoles(tx)
			if err != nil {
				return err
			}
			roleMap = m
		}

		pwd, err := helpers.HashPassword("password")
		if err != nil {
			return err
		}

		var users *seededUsers
		if targets.Users {
			if roleMap == nil {
				m, err := ensureRoles(tx)
				if err != nil {
					return err
				}
				roleMap = m
			}

			u, err := ensureUsers(tx, roleMap, photoPath, now, pwd)
			if err != nil {
				return err
			}
			users = u
		}

		if targets.Students {
			if users == nil {
				return fmt.Errorf("students seeding requires users")
			}
			if err := ensureStudent(tx, users, now); err != nil {
				return err
			}
		}

		if targets.Officials {
			if users == nil {
				return fmt.Errorf("officials seeding requires users")
			}
			if err := ensureOfficials(tx, users, signaturePath); err != nil {
				return err
			}
		}

		var letterTypes []LetterType
		if targets.LetterTypes {
			lts, err := ensureLetterTypes(tx)
			if err != nil {
				return err
			}
			letterTypes = lts
		}

		if targets.Templates {
			if users == nil {
				return fmt.Errorf("templates seeding requires users")
			}
			if len(letterTypes) == 0 {
				return fmt.Errorf("templates seeding requires letter types")
			}
			templateDocxPath, err := findAnyDocxTemplatePath()
			if err != nil {
				return err
			}
			if err := ensureTemplates(tx, letterTypes, templateDocxPath, users.Admin.ID); err != nil {
				return err
			}
		}

		return nil
	})
}

// Seed runs the legacy full seeding behavior (destructive, dev-oriented).
func Seed(db *gorm.DB, force bool) error {
	t, _ := ParseSeedTargets("all")
	t.TruncateAll = true
	return SeedSelected(db, force, t)
}

type seededUsers struct {
	Admin     *User
	Dekan     *User
	Wakil     *User
	Mahasiswa *User
}

func ensureRoles(tx *gorm.DB) (map[string]*Role, error) {
	desired := []Role{
		{Code: "MAHASISWA", Name: "Mahasiswa"},
		{Code: "ADMIN", Name: "Administrator"},
		{Code: "WAKIL_DEKAN", Name: "Wakil Dekan"},
		{Code: "DEKAN", Name: "Dekan"},
	}

	roleMap := make(map[string]*Role, len(desired))
	for _, r := range desired {
		var existing Role
		err := tx.Where("code = ?", r.Code).First(&existing).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			existing = Role{Code: r.Code, Name: r.Name}
			if err := tx.Create(&existing).Error; err != nil {
				return nil, err
			}
		} else if err != nil {
			return nil, err
		} else {
			if err := tx.Model(&existing).Update("name", r.Name).Error; err != nil {
				return nil, err
			}
		}
		role := existing
		roleMap[r.Code] = &role
	}
	return roleMap, nil
}

func ensureUsers(tx *gorm.DB, roleMap map[string]*Role, photoPath string, now time.Time, defaultPasswordHash string) (*seededUsers, error) {
	admin, err := ensureUser(tx, "admin@kampus.ac.id", "Admin Fakultas", defaultPasswordHash, roleMap["ADMIN"], photoPath, now)
	if err != nil {
		return nil, err
	}
	dekan, err := ensureUser(tx, "dekan@kampus.ac.id", "Prof. Dr. Dekan", defaultPasswordHash, roleMap["DEKAN"], photoPath, now)
	if err != nil {
		return nil, err
	}
	wakil, err := ensureUser(tx, "wakildekan@kampus.ac.id", "Dr. Wakil Dekan", defaultPasswordHash, roleMap["WAKIL_DEKAN"], photoPath, now)
	if err != nil {
		return nil, err
	}
	mahasiswa, err := ensureUser(tx, "mahasiswa@test.ac.id", "Mahasiswa Test", defaultPasswordHash, roleMap["MAHASISWA"], photoPath, now)
	if err != nil {
		return nil, err
	}

	return &seededUsers{Admin: admin, Dekan: dekan, Wakil: wakil, Mahasiswa: mahasiswa}, nil
}

func ensureUser(tx *gorm.DB, email, name, passwordHash string, role *Role, photoPath string, now time.Time) (*User, error) {
	if role == nil {
		return nil, fmt.Errorf("role is required for user %s", email)
	}

	var u User
	err := tx.Where("email = ?", email).First(&u).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		u = User{
			Name:            name,
			Email:           email,
			Password:        passwordHash,
			ProfilePhoto:    &photoPath,
			EmailVerifiedAt: &now,
			IsActive:        true,
		}
		if err := tx.Create(&u).Error; err != nil {
			return nil, err
		}
	} else if err != nil {
		return nil, err
	} else {
		if err := tx.Model(&u).Updates(map[string]any{"name": name, "is_active": true}).Error; err != nil {
			return nil, err
		}
		if u.ProfilePhoto == nil || strings.TrimSpace(*u.ProfilePhoto) == "" {
			if err := tx.Model(&u).Update("profile_photo", &photoPath).Error; err != nil {
				return nil, err
			}
		}
		if u.EmailVerifiedAt == nil {
			if err := tx.Model(&u).Update("email_verified_at", &now).Error; err != nil {
				return nil, err
			}
		}
	}

	if err := tx.Model(&u).Association("Roles").Append(role); err != nil {
		return nil, err
	}
	return &u, nil
}

func ensureStudent(tx *gorm.DB, users *seededUsers, now time.Time) error {
	if users == nil || users.Mahasiswa == nil || users.Admin == nil {
		return fmt.Errorf("student seeding requires admin and mahasiswa")
	}

	var s Student
	err := tx.Where("user_id = ?", users.Mahasiswa.ID).First(&s).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		adminID := users.Admin.ID
		s = Student{
			UserID:                  users.Mahasiswa.ID,
			NIM:                     "210123456",
			ProgramStudi:            "Teknik Informatika",
			Angkatan:                2021,
			SemesterMasukKuliah:     "Ganjil",
			AdminVerificationStatus: "approved",
			AdminVerifiedBy:         &adminID,
			AdminVerifiedAt:         &now,
			RejectionReason:         "",
		}
		return tx.Create(&s).Error
	}
	if err != nil {
		return err
	}

	adminID := users.Admin.ID
	return tx.Model(&s).Updates(map[string]any{
		"nim":                       "210123456",
		"program_studi":             "Teknik Informatika",
		"angkatan":                  2021,
		"semester_masuk_kuliah":     "Ganjil",
		"admin_verification_status": "approved",
		"admin_verified_by":         &adminID,
		"admin_verified_at":         &now,
		"rejection_reason":          "",
	}).Error
}

func ensureOfficials(tx *gorm.DB, users *seededUsers, signaturePath string) error {
	if users == nil || users.Admin == nil || users.Dekan == nil || users.Wakil == nil {
		return fmt.Errorf("officials seeding requires admin/dekan/wakil users")
	}

	officials := []Official{
		{
			UserID:    users.Dekan.ID,
			NIP:       "196501011990031001",
			Pangkat:   "Pembina Utama",
			Jabatan:   "Dekan",
			Signature: signaturePath,
			IsOnDuty:  true,
		},
		{
			UserID:    users.Wakil.ID,
			NIP:       "197001011995031002",
			Pangkat:   "Pembina",
			Jabatan:   "Wakil Dekan",
			Signature: signaturePath,
			IsOnDuty:  true,
		},
		{
			UserID:    users.Admin.ID,
			NIP:       "198001012005011001",
			Pangkat:   "Penata",
			Jabatan:   "Admin Fakultas",
			Signature: signaturePath,
			IsOnDuty:  true,
		},
	}

	for _, o := range officials {
		var existing Official
		err := tx.Where("user_id = ?", o.UserID).First(&existing).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			if err := tx.Create(&o).Error; err != nil {
				return err
			}
			continue
		}
		if err != nil {
			return err
		}
		if err := tx.Model(&existing).Updates(map[string]any{
			"nip":       o.NIP,
			"pangkat":   o.Pangkat,
			"jabatan":   o.Jabatan,
			"signature": o.Signature,
			"is_active": o.IsOnDuty,
		}).Error; err != nil {
			return err
		}
	}

	return nil
}

func ensureLetterTypes(tx *gorm.DB) ([]LetterType, error) {
	desired := []LetterType{
		{
			Code:        "SURAT_AKTIF",
			Name:        "Surat Keterangan Aktif Kuliah",
			Description: "Digunakan untuk keperluan administrasi mahasiswa",
		},
	}

	out := make([]LetterType, 0, len(desired))
	for _, lt := range desired {
		var existing LetterType
		err := tx.Where("code = ?", lt.Code).First(&existing).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			existing = lt
			if err := tx.Create(&existing).Error; err != nil {
				return nil, err
			}
		} else if err != nil {
			return nil, err
		} else {
			if err := tx.Model(&existing).Updates(map[string]any{"name": lt.Name, "description": lt.Description}).Error; err != nil {
				return nil, err
			}
		}
		out = append(out, existing)
	}
	return out, nil
}

func ensureTemplates(tx *gorm.DB, letterTypes []LetterType, templateDocxPath string, createdBy uint) error {
	for _, lt := range letterTypes {
		var existing LetterTemplate
		err := tx.Where("letter_type_id = ?", lt.ID).First(&existing).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			t := LetterTemplate{
				LetterTypeID: lt.ID,
				FilePath:     templateDocxPath,
				FileType:     "docx",
				CreatedBy:    createdBy,
			}
			if err := tx.Create(&t).Error; err != nil {
				return err
			}
			continue
		}
		if err != nil {
			return err
		}
		if err := tx.Model(&existing).Updates(map[string]any{"file_path": templateDocxPath, "file_type": "docx"}).Error; err != nil {
			return err
		}
	}
	return nil
}
