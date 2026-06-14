package migration

import (
	"fmt"

	"gorm.io/gorm"
)

func runVerificationRefactorMigration(db *gorm.DB) error {
	// Backfill centralized user email verification from legacy role-specific flags.
	if err := db.Exec(`
		DO $$
		BEGIN
			IF EXISTS (
				SELECT 1 FROM information_schema.columns
				WHERE table_name = 'students' AND column_name = 'email_verified'
			) THEN
				UPDATE users
				SET email_verified_at = COALESCE(email_verified_at, NOW())
				FROM students
				WHERE students.user_id = users.id
				  AND students.email_verified = TRUE;
			END IF;

			IF EXISTS (
				SELECT 1 FROM information_schema.columns
				WHERE table_name = 'officials' AND column_name = 'email_verified'
			) THEN
				UPDATE users
				SET email_verified_at = COALESCE(email_verified_at, NOW())
				FROM officials
				WHERE officials.user_id = users.id
				  AND officials.email_verified = TRUE;
			END IF;
		END $$;
	`).Error; err != nil {
		return fmt.Errorf("failed backfilling email_verified_at: %w", err)
	}

	// Backfill student admin verification status from legacy users.verified flag.
	if err := db.Exec(`
		DO $$
		BEGIN
			IF EXISTS (
				SELECT 1 FROM information_schema.columns
				WHERE table_name = 'users' AND column_name = 'verified'
			) THEN
				UPDATE students
				SET admin_verification_status = CASE
					WHEN users.verified = TRUE THEN 'approved'
					ELSE COALESCE(NULLIF(students.admin_verification_status, ''), 'pending')
				END,
				admin_verified_at = CASE
					WHEN users.verified = TRUE THEN COALESCE(students.admin_verified_at, NOW())
					ELSE students.admin_verified_at
				END,
				rejection_reason = CASE
					WHEN users.verified = TRUE THEN ''
					ELSE students.rejection_reason
				END
				FROM users
				WHERE students.user_id = users.id;
			END IF;
		END $$;
	`).Error; err != nil {
		return fmt.Errorf("failed backfilling student admin verification status: %w", err)
	}

	// Make the students status check constraint idempotent.
	if err := db.Exec(`
		DO $$
		BEGIN
			IF EXISTS (
				SELECT 1
				FROM pg_constraint
				WHERE conname = 'students_admin_verification_status_check'
			) THEN
				ALTER TABLE students
				DROP CONSTRAINT students_admin_verification_status_check;
			END IF;

			ALTER TABLE students
			ADD CONSTRAINT students_admin_verification_status_check
			CHECK (admin_verification_status IN ('invited','pending','approved','rejected'));
		END $$;
	`).Error; err != nil {
		fmt.Printf("⚠️  could not ensure students status check constraint: %v\n", err)
	}

	// Some legacy schemas have a partial officials table (missing columns).
	// Ensure core columns exist so later UPDATE/INSERT statements can run.
	if err := db.Exec(`
		DO $$
		BEGIN
			IF EXISTS (
				SELECT 1 FROM information_schema.tables
				WHERE table_name = 'officials'
			) THEN
				IF NOT EXISTS (
					SELECT 1 FROM information_schema.columns
					WHERE table_name = 'officials' AND column_name = 'nip'
				) THEN
					ALTER TABLE officials ADD COLUMN nip VARCHAR(50);
				END IF;

				IF NOT EXISTS (
					SELECT 1 FROM information_schema.columns
					WHERE table_name = 'officials' AND column_name = 'pangkat'
				) THEN
					ALTER TABLE officials ADD COLUMN pangkat VARCHAR(100);
				END IF;

				IF NOT EXISTS (
					SELECT 1 FROM information_schema.columns
					WHERE table_name = 'officials' AND column_name = 'jabatan'
				) THEN
					ALTER TABLE officials ADD COLUMN jabatan VARCHAR(100);
				END IF;

				IF NOT EXISTS (
					SELECT 1 FROM information_schema.columns
					WHERE table_name = 'officials' AND column_name = 'signature'
				) THEN
					ALTER TABLE officials ADD COLUMN signature VARCHAR(255);
				END IF;

				IF NOT EXISTS (
					SELECT 1 FROM information_schema.columns
					WHERE table_name = 'officials' AND column_name = 'is_active'
				) THEN
					ALTER TABLE officials ADD COLUMN is_active BOOLEAN DEFAULT TRUE;
				END IF;
			END IF;
		END $$;
	`).Error; err != nil {
		fmt.Printf("⚠️  could not ensure officials columns: %v\n", err)
	}

	// Normalize legacy official signature path to current public path.
	if err := db.Exec(`
		DO $$
		BEGIN
			IF EXISTS (
				SELECT 1 FROM information_schema.columns
				WHERE table_name = 'officials' AND column_name = 'signature'
			) THEN
				UPDATE officials
				SET signature = 'public/images/signatures/signatures.png'
				WHERE signature LIKE 'storage/signatures/%';
			END IF;
		END $$;
	`).Error; err != nil {
		fmt.Printf("⚠️  could not normalize official signature path: %v\n", err)
	}

	// Ensure every admin has an official row so admin appears in official section.
	// Guard with column-existence checks to support legacy schemas.
	if err := db.Exec(`
		DO $$
		BEGIN
			IF EXISTS (
				SELECT 1 FROM information_schema.columns
				WHERE table_name = 'officials' AND column_name IN ('user_id','nip','pangkat','jabatan','signature','is_active')
				GROUP BY table_name
				HAVING COUNT(*) = 6
			) THEN
				INSERT INTO officials (user_id, nip, pangkat, jabatan, signature, is_active)
				SELECT u.id, '198001012005011001', 'Penata', 'Admin Fakultas', 'public/images/signatures/signatures.png', TRUE
				FROM users u
				JOIN user_roles ur ON ur.user_id = u.id
				JOIN roles r ON r.id = ur.role_id
				WHERE r.code = 'ADMIN'
				  AND NOT EXISTS (
					  SELECT 1 FROM officials o WHERE o.user_id = u.id
				  );
			END IF;
		END $$;
	`).Error; err != nil {
		fmt.Printf("⚠️  could not backfill admin officials: %v\n", err)
	}

	return nil
}

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

	if err := runVerificationRefactorMigration(db); err != nil {
		return err
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
	if err := runVerificationRefactorMigration(db); err != nil {
		return err
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
