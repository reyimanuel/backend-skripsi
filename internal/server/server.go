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
	"gorm.io/gorm"
)

func Run() {
	log.Println("Starting HTTP server...")

	cfg := config.Get()
	if cfg == nil {
		log.Fatal("Configuration not loaded")
	}

	db, _, err := database.ConnectDB()
	if err != nil {
		log.Fatal("Database connection failed:", err)
	}

	middleware.InitMiddleware(db)
	startHTTPServer(cfg, db)
}

func startHTTPServer(cfg *config.AppConfigurationMap, db *gorm.DB) {
	if cfg.IsProduction {
		gin.SetMode(gin.ReleaseMode)
	}

	r := gin.New()
	r.Use(
		gin.Logger(),
		gin.Recovery(),
		middleware.CORSMiddleware(),
	)

	// 🔑 DB disuntikkan ke API layer
	api.RegisterRoutes(r, db)

	srv := &http.Server{
		Addr:         fmt.Sprintf(":%d", cfg.Port),
		Handler:      r,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	log.Printf("Server running on port %d", cfg.Port)
	log.Fatal(srv.ListenAndServe())
}
