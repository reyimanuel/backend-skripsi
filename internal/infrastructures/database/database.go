package database

import (
	"database/sql"
	"log"
	"os"
	"time"

	"github.com/reyimanuel/letter-administration/internal/infrastructures/config"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func ConnectDB() (*gorm.DB, *sql.DB, error) {
	cfg := config.Get() // Get the database configuration from the config package.

	// Set up a custom SQL logger to log queries
	sqlLogger := logger.New(
		log.New(os.Stdout, "\r\n", log.LstdFlags), // Logs output to console
		logger.Config{
			SlowThreshold:             time.Second,
			LogLevel:                  logger.Info,
			IgnoreRecordNotFoundError: true,
			ParameterizedQueries:      true,
			Colorful:                  true,
		})

	log.Println("connecting to database...")

	db, err := gorm.Open(postgres.Open(cfg.DbURI), &gorm.Config{ // Open connection to database using GORM and PostgreSQL
		Logger:                 sqlLogger, // Use the custom SQL logger
		SkipDefaultTransaction: true,      // Skip default transaction
		// Prevent accidental data loss by disabling global updates.
		// This ensures that updates without a WHERE clause are not allowed.
		AllowGlobalUpdate: false, // Do not allow global updates to avoid accidental data loss
	})

	// Check if there's an error while connecting to the database
	if err != nil {
		log.Fatalf("error connecting to SQL: %v", err)
	}

	log.Println("setting database connection configuration...")

	sqlDB, err := db.DB()

	if err != nil {
		log.Fatalf("error setting database connection configuration: %v", err)
	}

	sqlDB.SetMaxIdleConns(10)

	sqlDB.SetConnMaxLifetime(time.Hour)

	// Return both *gorm.DB for ORM operations and *sql.DB for lower-level database control (e.g., connection pooling).
	return db, sqlDB, nil
}
