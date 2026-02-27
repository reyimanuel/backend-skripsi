package migration

import (
	"fmt"

	"gorm.io/gorm"
)

func RunMigration(db *gorm.DB, force bool) error {
	fmt.Println("Running migrations...")

	// Ensure core tables (users, roles) exist first so we can clean up
	// any orphaned rows in dependent tables (e.g. officials) before
	// AutoMigrate adds foreign key constraints that would fail.
	if err := db.AutoMigrate(&Models[0], &Models[1]); err != nil {
		// fallback: try explicit types for clarity
		if err2 := db.AutoMigrate((*User)(nil), (*Role)(nil)); err2 != nil {
			return fmt.Errorf("gagal migrasi (pre-migrate users/roles): %w; fallback: %v", err, err2)
		}
	}

	// If the officials table already exists from a previous run, remove
	// any rows that reference non-existent users to avoid FK constraint
	// creation failures when AutoMigrate runs for the full schema.
	if db.Migrator().HasTable(&Official{}) {
		if err := db.Exec(`DELETE FROM officials WHERE user_id NOT IN (SELECT id FROM users)`).Error; err != nil {
			return fmt.Errorf("failed cleaning orphan officials: %w", err)
		}
	}

	if err := db.AutoMigrate(Models...); err != nil {
		return fmt.Errorf("gagal migrasi: %w", err)
	}

	// Make approver_id nullable — AutoMigrate won't loosen NOT NULL on its own.
	if err := db.Exec(`ALTER TABLE letter_approvals ALTER COLUMN approver_id DROP NOT NULL`).Error; err != nil {
		fmt.Printf("⚠️  could not alter approver_id (may already be nullable): %v\n", err)
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
	if err := db.Exec(`ALTER TABLE letter_approvals ALTER COLUMN approver_id DROP NOT NULL`).Error; err != nil {
		fmt.Printf("⚠️  could not alter approver_id (may already be nullable): %v\n", err)
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
