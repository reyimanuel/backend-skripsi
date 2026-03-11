package migration

import (
	"fmt"
	"os"
	"strings"

	"github.com/reyimanuel/letter-administration/internal/infrastructures/pkg/helpers"
	"gorm.io/gorm"
)

func Seed(db *gorm.DB, force bool) error {
	host := os.Getenv("DB_HOST")
	if !force && !strings.EqualFold(host, "localhost") && host != "127.0.0.1" {
		return fmt.Errorf("seeding blocked in production (DB_HOST=%s), use --force", host)
	}

	return db.Transaction(func(tx *gorm.DB) error {

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
			Name:     "Admin Fakultas",
			Email:    "admin@kampus.ac.id",
			Password: pwd,
			Roles:    []Role{roleMap["ADMIN"]},
			Verified: true,
		}

		dekanUser := User{
			Name:     "Prof. Dr. Dekan",
			Email:    "dekan@kampus.ac.id",
			Password: pwd,
			Roles:    []Role{roleMap["DEKAN"]},
			Verified: true,
		}

		wakilUser := User{
			Name:     "Dr. Wakil Dekan",
			Email:    "wakildekan@kampus.ac.id",
			Password: pwd,
			Roles:    []Role{roleMap["WAKIL_DEKAN"]},
			Verified: true,
		}

		mahasiswa := User{
			Name:     "Mahasiswa Test",
			Email:    "mahasiswa@test.ac.id",
			Password: pwd,
			Roles:    []Role{roleMap["MAHASISWA"]},
			Verified: true,
		}

		users := []*User{&admin, &dekanUser, &wakilUser, &mahasiswa}

		if err := tx.Create(&users).Error; err != nil {
			return err
		}

		student := Student{
			UserID:       mahasiswa.ID,
			NIM:          "210123456",
			ProgramStudi: "Teknik Informatika",
			Angkatan:     2021,
		}

		if err := tx.Create(&student).Error; err != nil {
			return err
		}

		dekanOfficial := Official{
			UserID:    dekanUser.ID,
			NIP:       "196501011990031001",
			Pangkat:   "Pembina Utama",
			Jabatan:   "Dekan",
			Signature: "storage/signatures/dekan.png",
			IsActive:  true,
		}

		wakilOfficial := Official{
			UserID:    wakilUser.ID,
			NIP:       "197001011995031002",
			Pangkat:   "Pembina",
			Jabatan:   "Wakil Dekan",
			Signature: "storage/signatures/wakil.png",
			IsActive:  true,
		}

		if err := tx.Create(&dekanOfficial).Error; err != nil {
			return err
		}

		if err := tx.Create(&wakilOfficial).Error; err != nil {
			return err
		}

		letterTypes := []LetterType{
			{
				Code:        "SURAT_AKTIF",
				Name:        "Surat Keterangan Aktif Kuliah",
				Description: "Digunakan untuk keperluan administrasi mahasiswa",
			},
			{
				Code:        "SURAT_PENELITIAN",
				Name:        "Surat Izin Penelitian",
				Description: "Digunakan untuk keperluan penelitian",
			},
		}

		if err := tx.Create(&letterTypes).Error; err != nil {
			return err
		}

		return nil
	})
}
