package server

import (
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/reyimanuel/letter-administration/internal/api"
	config "github.com/reyimanuel/letter-administration/internal/infrastructures/config"
	"github.com/reyimanuel/letter-administration/internal/infrastructures/database"
	"github.com/reyimanuel/letter-administration/internal/infrastructures/middleware"
	"github.com/reyimanuel/letter-administration/internal/infrastructures/pkg/token"
	"gorm.io/gorm"
)

// Run initializes and starts the HTTP server
func Run() {
	// Log server startup
	log.Println("Starting HTTP server...")

	// Load configuration from environment
	cfg := config.Get()
	if cfg == nil {
		log.Fatal("Configuration not loaded")
	}

	token.Load()

	// Connect to database
	db, _, err := database.ConnectDB()
	if err != nil {
		log.Fatal("Database connection failed:", err)
	}

	// Initialize middleware with database connection
	middleware.InitMiddleware(db)
	// Start the HTTP server
	startHTTPServer(cfg, db)
}

// startHTTPServer configures and starts the Gin HTTP server
func startHTTPServer(cfg *config.AppConfigurationMap, db *gorm.DB) {
	// Set Gin mode to release (production) if enabled
	if cfg.IsProduction {
		gin.SetMode(gin.ReleaseMode)
	}

	// Create new Gin router instance
	r := gin.New()
	// Register global middleware
	r.Use(
		gin.Logger(),                // HTTP request logging
		gin.Recovery(),              // Panic recovery
		middleware.CORSMiddleware(), // CORS headers
	)

	// Serve uploaded/generated files.
	r.Static("/public", "./public")

	// Register API routes and inject database connection
	api.RegisterRoutes(r, db)

	// Configure HTTP server with timeouts
	srv := &http.Server{
		Addr:         fmt.Sprintf(":%d", cfg.Port), // Listen on specified port
		Handler:      r,                            // Use Gin router as handler
		ReadTimeout:  30 * time.Second,             // Max time to read request
		WriteTimeout: 30 * time.Second,             // Max time to write response
		IdleTimeout:  120 * time.Second,            // Max idle connection time
	}

	// Log server startup message
	log.Printf("Server running on port %d", cfg.Port)
	// Start server and log any fatal errors
	log.Fatal(srv.ListenAndServe())
}
