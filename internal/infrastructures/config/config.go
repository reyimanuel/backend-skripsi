package config

import (
	"fmt"
	"log"
	"os"
	"strconv"

	"github.com/joho/godotenv"
	"github.com/reyimanuel/letter-administration/internal/infrastructures/pkg/utils"
)

type AppConfigurationMap struct {
	Port                 int              // Port is the port number that the server will listen to.
	IsProduction         bool             // IsProduction is a flag that indicates whether the application is running in production mode.
	DbURI                string           // Database connection.
	AccessTokenLifeTime  int64            // AccessTokenLifeTime is the lifetime of the access token in seconds.
	RefreshTokenLifeTime int64            // RefreshTokenLifeTime is the lifetime of the refresh token in seconds.
	PrivateKeyPath       string           // Path to the private key file.
	PublicKeyPath        string           // Path to the public key file.
	BaseURL              string           // BaseURL is the base URL of the application, used for generating absolute URLs.
	SMTP                 SMTPConfig       // SMTP configuration
	MailerSend           MailerSendConfig // MailerSend configuration
	EmailProvider        string           // Which email provider to use: "smtp" or any email provider
	FrontEndURL          string           // FrontEndURL is the URL of the front-end application.
	FirebaseCredentials  string           // Path to the Firebase credentials file.
}

// SMTPConfig holds SMTP configuration.
// Environment variables:
// - SMTP_HOST
// - SMTP_PORT
// - SMTP_USER
// - SMTP_PASS
// - SMTP_SENDER_EMAIL (optional; defaults to SMTP_USER)
// - SMTP_SENDER_NAME (optional)
type SMTPConfig struct {
	Host        string
	Port        string
	User        string
	Pass        string
	SenderEmail string
	SenderName  string
}

// MailerSendConfig holds MailerSend configuration.
type MailerSendConfig struct {
	APIKey      string
	SenderEmail string
	SenderName  string
}

// config is a global variable that stores the loaded application configuration.
var config *AppConfigurationMap

// Get is a function that returns the loaded application configuration.
func Get() *AppConfigurationMap {
	return config
}

// Load is a function that loads the application configuration from the environment variables.
func Load() {
	log.Println("Loading config from environment...")

	// Load environment variables from a .env file.
	err := godotenv.Load()
	if err != nil {
		log.Printf("Error loading environment variables, try to get from environtment OS")
	}

	// Read the PORT environment variable and convert it to an integer.
	port, err := strconv.Atoi(os.Getenv("PORT"))

	// Set default value port if env doesn't have PORT config
	if err != nil {
		port = 8080
	}

	// Check if the application is running in production mode.
	isProduction := utils.SafeCompareString(os.Getenv("IS_PRODUCTION"), "true")

	AccessTokenLifeTime, err := strconv.Atoi(os.Getenv("ACCESS_TOKEN_LIFE_TIME"))
	if err != nil {
		AccessTokenLifeTime = 3600 // Default value of 1 hour
	}

	RefreshTokenLifeTime, err := strconv.Atoi(os.Getenv("REFRESH_TOKEN_LIFE_TIME"))
	if err != nil {
		RefreshTokenLifeTime = 86400 // Default value of 24 hours
	}

	PrivateKeyPath := os.Getenv("PRIVATE_KEY")
	if PrivateKeyPath == "" {
		log.Fatalf("PRIVATE_KEY environment variable is not set, check your .env file")
	}

	PublicKeyPath := os.Getenv("PUBLIC_KEY")
	if PublicKeyPath == "" {
		log.Fatalf("PUBLIC_KEY environment variable is not set, check your .env file")
	}

	BaseURL := os.Getenv("BASE_URL")
	if BaseURL == "" {
		BaseURL = fmt.Sprintf("http://localhost:%d", port)
	}

	SMTP := SMTPConfig{
		Host:        os.Getenv("SMTP_HOST"),
		Port:        os.Getenv("SMTP_PORT"),
		User:        os.Getenv("SMTP_USER"),
		Pass:        os.Getenv("SMTP_PASS"),
		SenderEmail: os.Getenv("SMTP_SENDER_EMAIL"),
		SenderName:  os.Getenv("SMTP_SENDER_NAME"),
	}

	// MailerSend Configuration
	MailerSend := MailerSendConfig{
		APIKey:      os.Getenv("MAILERSEND_API_KEY"),
		SenderEmail: os.Getenv("MAILERSEND_SENDER_EMAIL"),
		SenderName:  os.Getenv("MAILERSEND_SENDER_NAME"),
	}

	// Email provider selection (default to SMTP for backward compatibility)
	EmailProvider := os.Getenv("EMAIL_PROVIDER")
	if EmailProvider == "" {
		EmailProvider = "smtp"
	}

	FrontEndURL := os.Getenv("FRONTEND_URL")
	if FrontEndURL == "" {
		FrontEndURL = "http://localhost:3000"
	}

	FirebaseCredentials := os.Getenv("FIREBASE_CREDENTIALS")
	if FirebaseCredentials == "" {
		log.Fatalf("FIREBASE_CREDENTIALS environment variable is not set, check your .env file")
	}

	// Set global variable config
	config = &AppConfigurationMap{
		Port:                 port,
		IsProduction:         isProduction,
		DbURI:                loadDatabaseConfig(),
		AccessTokenLifeTime:  int64(AccessTokenLifeTime),
		RefreshTokenLifeTime: int64(RefreshTokenLifeTime),
		PrivateKeyPath:       PrivateKeyPath,
		PublicKeyPath:        PublicKeyPath,
		BaseURL:              BaseURL,
		SMTP:                 SMTP,
		MailerSend:           MailerSend,
		EmailProvider:        EmailProvider,
		FrontEndURL:          FrontEndURL,
		FirebaseCredentials:  FirebaseCredentials,
	}

	// Set global variable config
	config = &AppConfigurationMap{
		Port:                 port,
		IsProduction:         isProduction,
		DbURI:                loadDatabaseConfig(),
		AccessTokenLifeTime:  int64(AccessTokenLifeTime),
		RefreshTokenLifeTime: int64(RefreshTokenLifeTime),
		PrivateKeyPath:       PrivateKeyPath,
		PublicKeyPath:        PublicKeyPath,
		BaseURL:              BaseURL,
		SMTP:                 SMTP,
		MailerSend:           MailerSend,
		EmailProvider:        EmailProvider,
		FrontEndURL:          FrontEndURL,
		FirebaseCredentials:  FirebaseCredentials,
	}
}

// loadDatabaseConfig is a function that loads the database configuration from the environment variables.
func loadDatabaseConfig() string {
	user := getFromEnv("DB_USER", true)
	pass := getFromEnv("DB_PASS", true)
	name := getFromEnv("DB_NAME", true)
	host := getFromEnv("DB_HOST", true)
	port := getFromEnv("DB_PORT", true)
	timeZone := getFromEnv("DB_TIME_ZONE", true)

	return fmt.Sprintf("host=%s user=%s password=%s dbname=%s port=%s TimeZone=%s", host, user, pass, name, port, timeZone)
}

// getFromEnv retrieves an environment variable by key and exits the program if it's not set.
func getFromEnv(key string, _ bool) string {
	value := os.Getenv(key)
	if value == "" {
		log.Fatalf("Required environment variable %s is not set", key)
	}

	return value
}
