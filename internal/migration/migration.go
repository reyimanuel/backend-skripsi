package migration

import (
	"fmt"

	"gorm.io/gorm"
)

func RunMigration(db *gorm.DB, force bool) error {
	fmt.Println("Running migrations...")
	if err := db.AutoMigrate(Models...); err != nil {
		return fmt.Errorf("gagal migrasi: %w", err)
	}
	fmt.Println("✅ Migrations completed")

	fmt.Println("Seeding database...")
	if err := Seed(db, force); err != nil {
		return fmt.Errorf("gagal seeding: %w", err)
	}
	fmt.Println("✅ Seeding completed")

	return nil
}

func RunMigrationOnly(db *gorm.DB) error {
	fmt.Println("Running migrations (schema only, no seeding)...")
	if err := db.AutoMigrate(Models...); err != nil {
		return fmt.Errorf("gagal migrasi: %w", err)
	}
	fmt.Println("✅ Migrations completed (no seeding)")
	return nil
}

func DropMigration(db *gorm.DB) error {
	fmt.Println("Dropping all tables...")
	if err := db.Migrator().DropTable(Models...); err != nil {
		return fmt.Errorf("❌ Failed dropping tables: %w", err)
	}
	fmt.Println("✅ All tables dropped")
	return nil
}
