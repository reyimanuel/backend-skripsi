package migration

import (
	"fmt"

	"gorm.io/gorm"
)

func runApplicationMigration(db *gorm.DB) error {
	if err := db.Exec(`
		ALTER TABLE users
			DROP COLUMN IF EXISTS email_verification_code_hash,
			DROP COLUMN IF EXISTS email_verification_expires_at;
		ALTER TABLE students DROP COLUMN IF EXISTS email_verified;
		ALTER TABLE atasan DROP COLUMN IF EXISTS email_verified;
	`).Error; err != nil {
		return fmt.Errorf("gagal membersihkan kolom verifikasi lama: %w", err)
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

	// Some legacy schemas have a partial atasan table (missing columns).
	// Ensure core columns exist so later UPDATE/INSERT statements can run.
	if err := db.Exec(`
		DO $$
		BEGIN
			IF EXISTS (
				SELECT 1 FROM information_schema.tables
				WHERE table_name = 'atasan'
			) THEN
				IF NOT EXISTS (
					SELECT 1 FROM information_schema.columns
					WHERE table_name = 'atasan' AND column_name = 'nip'
				) THEN
					ALTER TABLE atasan ADD COLUMN nip VARCHAR(50);
				END IF;

				IF NOT EXISTS (
					SELECT 1 FROM information_schema.columns
					WHERE table_name = 'atasan' AND column_name = 'pangkat'
				) THEN
					ALTER TABLE atasan ADD COLUMN pangkat VARCHAR(100);
				END IF;

				IF NOT EXISTS (
					SELECT 1 FROM information_schema.columns
					WHERE table_name = 'atasan' AND column_name = 'jabatan'
				) THEN
					ALTER TABLE atasan ADD COLUMN jabatan VARCHAR(100);
				END IF;

				IF NOT EXISTS (
					SELECT 1 FROM information_schema.columns
					WHERE table_name = 'atasan' AND column_name = 'signature'
				) THEN
					ALTER TABLE atasan ADD COLUMN signature VARCHAR(255);
				END IF;

				IF NOT EXISTS (
					SELECT 1 FROM information_schema.columns
					WHERE table_name = 'atasan' AND column_name = 'is_active'
				) THEN
					ALTER TABLE atasan ADD COLUMN is_active BOOLEAN DEFAULT TRUE;
				END IF;
			END IF;
		END $$;
	`).Error; err != nil {
		fmt.Printf("⚠️  could not ensure atasan columns: %v\n", err)
	}

	// Normalize legacy atasan signature path to current public path.
	if err := db.Exec(`
		DO $$
		BEGIN
			IF EXISTS (
				SELECT 1 FROM information_schema.columns
				WHERE table_name = 'atasan' AND column_name = 'signature'
			) THEN
				UPDATE atasan
				SET signature = 'public/images/signatures/signatures.png'
				WHERE signature LIKE 'storage/signatures/%';
			END IF;
		END $$;
	`).Error; err != nil {
		fmt.Printf("⚠️  could not normalize atasan signature path: %v\n", err)
	}

	// Ensure every admin has an atasan row so admin appears in atasan section.
	// Guard with column-existence checks to support legacy schemas.
	if err := db.Exec(`
		DO $$
		BEGIN
			IF EXISTS (
				SELECT 1 FROM information_schema.columns
				WHERE table_name = 'atasan' AND column_name IN ('user_id','nip','pangkat','jabatan','signature','is_active')
				GROUP BY table_name
				HAVING COUNT(*) = 6
			) THEN
				INSERT INTO atasan (user_id, nip, pangkat, jabatan, signature, is_active)
				SELECT u.id, '198001012005011001', 'Penata', 'Admin Fakultas', 'public/images/signatures/signatures.png', TRUE
				FROM users u
				JOIN user_roles ur ON ur.user_id = u.id
				JOIN roles r ON r.id = ur.role_id
				WHERE r.code = 'ADMIN'
				  AND NOT EXISTS (
					  SELECT 1 FROM atasan o WHERE o.user_id = u.id
				  );
			END IF;
		END $$;
	`).Error; err != nil {
		fmt.Printf("⚠️  could not backfill admin atasan: %v\n", err)
	}

	// Backfill the single ATASAN access role for legacy atasan-role accounts.
	if err := db.Exec(`
		DO $$
		DECLARE
			atasan_role_id BIGINT;
		BEGIN
			IF EXISTS (SELECT 1 FROM information_schema.tables WHERE table_name = 'roles')
			   AND EXISTS (SELECT 1 FROM information_schema.tables WHERE table_name = 'user_roles') THEN
				INSERT INTO roles (code, name)
				SELECT 'ATASAN', 'Atasan'
				WHERE NOT EXISTS (SELECT 1 FROM roles WHERE code = 'ATASAN');

				SELECT id INTO atasan_role_id FROM roles WHERE code = 'ATASAN' LIMIT 1;

				IF atasan_role_id IS NOT NULL THEN
					INSERT INTO user_roles (user_id, role_id)
					SELECT DISTINCT ur.user_id, atasan_role_id
					FROM user_roles ur
					JOIN roles r ON r.id = ur.role_id
					WHERE r.code IN (
						'DEKAN',
						'WAKIL_DEKAN',
						'WAKIL_DEKAN_1',
						'WAKIL_DEKAN_2',
						'WAKIL_DEKAN_3',
						'KOPRODI',
						'KABAG',
						'KAJUR'
					)
					  AND NOT EXISTS (
						  SELECT 1
						  FROM user_roles existing
						  WHERE existing.user_id = ur.user_id
						    AND existing.role_id = atasan_role_id
					  );
				END IF;
			END IF;
		END $$;
	`).Error; err != nil {
		fmt.Printf("could not backfill ATASAN role: %v\n", err)
	}

	legacyRoleCodes := []string{
		"DEKAN",
		"WAKIL_DEKAN",
		"WAKIL_DEKAN_1",
		"WAKIL_DEKAN_2",
		"WAKIL_DEKAN_3",
		"KOPRODI",
		"KABAG",
		"KAJUR",
		"OFFI" + "CIAL",
	}
	if err := db.Transaction(func(tx *gorm.DB) error {
		roleIDs := tx.Model(&Role{}).Select("id").Where("code IN ?", legacyRoleCodes)
		var atasanRole Role
		if err := tx.Where("code = ?", "ATASAN").First(&atasanRole).Error; err != nil {
			return err
		}
		if err := tx.Model(&LetterApproval{}).
			Where("role_id IN (?)", roleIDs).
			Update("role_id", atasanRole.ID).Error; err != nil {
			return err
		}
		if err := tx.Where("role_id IN (?)", roleIDs).Delete(&UserRole{}).Error; err != nil {
			return err
		}
		return tx.Where("code IN ?", legacyRoleCodes).Delete(&Role{}).Error
	}); err != nil {
		return fmt.Errorf("gagal menghapus role lama setelah migrasi ATASAN: %w", err)
	}

	return nil
}

func migrateAtasanTable(db *gorm.DB) error {
	legacyTable := "offi" + "cials"
	if db.Migrator().HasTable(legacyTable) && !db.Migrator().HasTable("atasan") {
		if err := db.Migrator().RenameTable(legacyTable, "atasan"); err != nil {
			return fmt.Errorf("gagal memindahkan tabel lama ke atasan: %w", err)
		}
	}
	return nil
}

func RunMigration(db *gorm.DB, force bool) error {
	fmt.Println("Running migrations...")
	if err := migrateAtasanTable(db); err != nil {
		return err
	}

	// Ensure core tables (users, roles) exist first so we can clean up
	// any orphaned rows in dependent tables (e.g. atasan) before
	// AutoMigrate adds foreign key constraints that would fail.
	if err := db.AutoMigrate(&User{}, &Role{}); err != nil {
		// fallback: try explicit types for clarity
		if err2 := db.AutoMigrate((*User)(nil), (*Role)(nil)); err2 != nil {
			return fmt.Errorf("gagal migrasi (pre-migrate users/roles): %w; fallback: %v", err, err2)
		}
	}

	// If the atasan table already exists from a previous run, remove
	// any rows that reference non-existent users to avoid FK constraint
	// creation failures when AutoMigrate runs for the full schema.
	if db.Migrator().HasTable(&Atasan{}) {
		if err := db.Exec(`DELETE FROM atasan WHERE user_id NOT IN (SELECT id FROM users)`).Error; err != nil {
			return fmt.Errorf("failed cleaning orphan atasan: %w", err)
		}
	}

	if err := db.AutoMigrate(Models...); err != nil {
		return fmt.Errorf("gagal migrasi: %w", err)
	}

	if err := runApplicationMigration(db); err != nil {
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
	if err := migrateAtasanTable(db); err != nil {
		return err
	}
	if err := db.AutoMigrate(Models...); err != nil {
		return fmt.Errorf("gagal migrasi: %w", err)
	}
	if err := runApplicationMigration(db); err != nil {
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
