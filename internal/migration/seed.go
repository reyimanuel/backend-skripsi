package migration

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/reyimanuel/letter-administration/internal/infrastructures/pkg/helpers"
	"gorm.io/gorm"
)

func findAnyDocxTemplatePath() (string, error) {
	dir := filepath.Join("public", "letter-template")
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", fmt.Errorf("cannot read template directory %s: %w", dir, err)
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if strings.EqualFold(filepath.Ext(e.Name()), ".docx") {
			return filepath.Join(dir, e.Name()), nil
		}
	}
	return "", fmt.Errorf("no .docx template found under %s", dir)
}

func Seed(db *gorm.DB, force bool) error {
	host := os.Getenv("DB_HOST")
	if !force && !strings.EqualFold(host, "localhost") && host != "127.0.0.1" {
		return fmt.Errorf("seeding blocked in production (DB_HOST=%s), use --force", host)
	}

	templateDocxPath, err := findAnyDocxTemplatePath()
	if err != nil {
		return err
	}

	return db.Transaction(func(tx *gorm.DB) error {
		now := time.Now()
		photoPath := filepath.ToSlash(filepath.Join("public", "images", "profile-photos", "example.png"))
		signaturePath := filepath.ToSlash(filepath.Join("public", "images", "signatures", "signatures.png"))

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

		roles := []Role{
			{Code: "MAHASISWA", Name: "Mahasiswa"},
			{Code: "ADMIN", Name: "Administrator"},
			{Code: "WAKIL_DEKAN", Name: "Wakil Dekan"},
			{Code: "DEKAN", Name: "Dekan"},
		}

		if err := tx.Create(&roles).Error; err != nil {
			return err
		}

		roleMap := make(map[string]Role)
		for _, r := range roles {
			roleMap[r.Code] = r
		}

		pwd, err := helpers.HashPassword("password")
		if err != nil {
			return err
		}

		admin := User{
			Name:            "Admin Fakultas",
			Email:           "admin@kampus.ac.id",
			Password:        pwd,
			Roles:           []Role{roleMap["ADMIN"]},
			ProfilePhoto:    &photoPath,
			EmailVerifiedAt: &now,
			IsActive:        true,
		}

		dekanUser := User{
			Name:            "Prof. Dr. Dekan",
			Email:           "dekan@kampus.ac.id",
			Password:        pwd,
			Roles:           []Role{roleMap["DEKAN"]},
			ProfilePhoto:    &photoPath,
			EmailVerifiedAt: &now,
			IsActive:        true,
		}

		wakilUser := User{
			Name:            "Dr. Wakil Dekan",
			Email:           "wakildekan@kampus.ac.id",
			Password:        pwd,
			Roles:           []Role{roleMap["WAKIL_DEKAN"]},
			ProfilePhoto:    &photoPath,
			EmailVerifiedAt: &now,
			IsActive:        true,
		}

		mahasiswa := User{
			Name:            "Mahasiswa Test",
			Email:           "mahasiswa@test.ac.id",
			Password:        pwd,
			Roles:           []Role{roleMap["MAHASISWA"]},
			ProfilePhoto:    &photoPath,
			EmailVerifiedAt: &now,
			IsActive:        true,
		}

		users := []*User{&admin, &dekanUser, &wakilUser, &mahasiswa}

		if err := tx.Create(&users).Error; err != nil {
			return err
		}

		student := Student{
			UserID:                  mahasiswa.ID,
			NIM:                     "210123456",
			ProgramStudi:            "Teknik Informatika",
			Angkatan:                2021,
			AdminVerificationStatus: "approved",
			AdminVerifiedBy:         &admin.ID,
			AdminVerifiedAt:         &now,
			RejectionReason:         "",
		}

		if err := tx.Create(&student).Error; err != nil {
			return err
		}

		dekanOfficial := Official{
			UserID:    dekanUser.ID,
			NIP:       "196501011990031001",
			Pangkat:   "Pembina Utama",
			Jabatan:   "Dekan",
			Signature: signaturePath,
			IsOnDuty:  true,
		}

		wakilOfficial := Official{
			UserID:    wakilUser.ID,
			NIP:       "197001011995031002",
			Pangkat:   "Pembina",
			Jabatan:   "Wakil Dekan",
			Signature: signaturePath,
			IsOnDuty:  true,
		}

		adminOfficial := Official{
			UserID:    admin.ID,
			NIP:       "198001012005011001",
			Pangkat:   "Penata",
			Jabatan:   "Admin Fakultas",
			Signature: signaturePath,
			IsOnDuty:  true,
		}

		officials := []Official{dekanOfficial, wakilOfficial, adminOfficial}
		if err := tx.Create(&officials).Error; err != nil {
			return err
		}

		letterTypes := []LetterType{
			{
				Code:        "SURAT_AKTIF",
				Name:        "Surat Keterangan Aktif Kuliah",
				Description: "Digunakan untuk keperluan administrasi mahasiswa",
			},
		}

		if err := tx.Create(&letterTypes).Error; err != nil {
			return err
		}

		templates := make([]LetterTemplate, 0, len(letterTypes))
		for _, lt := range letterTypes {
			templates = append(templates, LetterTemplate{
				LetterTypeID: lt.ID,
				FilePath:     templateDocxPath,
				FileType:     "docx",
				CreatedBy:    admin.ID,
			})
		}

		if err := tx.Create(&templates).Error; err != nil {
			return err
		}

		return nil
	})
}
