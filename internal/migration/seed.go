package migration

import (
	"fmt"

	"os"
	"strings"

	"github.com/reyimanuel/letter-administration/internal/api/user"
	"gorm.io/gorm"
)

func Seed(db *gorm.DB, force bool) error {
	host := os.Getenv("DB_HOST")
	if !force && !strings.EqualFold(host, "localhost") && host != "127.0.0.1" {
		return fmt.Errorf("seeding blocked when in production: DB_HOST=%s use --force if needed", host)
	}

	return db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec(`TRUNCATE TABLE "users" RESTART IDENTITY CASCADE`).Error; err != nil {
			return fmt.Errorf("truncate users fail: %w", err)
		}

		users := []user.User{}
		if err := tx.Create(&users).Error; err != nil {

			return fmt.Errorf("gagal insert users: %w", err)
		}
		return nil
	})
}
