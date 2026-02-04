package migration

import (
	"fmt"
	"os"
	"strings"
	"time"

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
				user_roles,
				students,
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

		roleMap := map[string]Role{}
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
		}

		dekan := User{
			Name:     "Dekan",
			Email:    "dekan@kampus.ac.id",
			Password: pwd,
			Roles:    []Role{roleMap["DEKAN"]},
		}

		wakilDekan := User{
			Name:     "Wakil Dekan",
			Email:    "wakildekan@kampus.ac.id",
			Password: pwd,
			Roles:    []Role{roleMap["WAKIL_DEKAN"]},
		}

		mahasiswa := User{
			Name:     "Mahasiswa Test",
			Email:    "miraclesumajow026@student.unsrat.ac.id",
			Password: pwd,
			Roles:    []Role{roleMap["MAHASISWA"]},
		}

		users := []*User{
			&admin,
			&dekan,
			&wakilDekan,
			&mahasiswa,
		}

		if err := tx.Create(&users).Error; err != nil {
			return err
		}

		student := Student{
			UserID:       mahasiswa.ID,
			NPM:          "210123456",
			ProgramStudi: "Teknik Informatika",
			Angkatan:     2021,
			CreatedAt:    time.Now(),
			UpdatedAt:    time.Now(),
		}

		if err := tx.Create(&student).Error; err != nil {
			return err
		}

		return nil
	})
}
