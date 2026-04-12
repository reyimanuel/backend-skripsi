package main

// @title           E-Voting 2025 API
// @version         1.0.0
// @description     REST API untuk sistem E-Voting 2025 UNSRAT IT Community.
// @BasePath        /
// @schemes         http
// @securityDefinitions.apikey  BearerAuth
// @in                           header
// @name                         Authorization

import (
	"fmt"
	"os"
	"slices"
	"strings"

	"github.com/reyimanuel/letter-administration/internal/infrastructures/config"
	"github.com/reyimanuel/letter-administration/internal/infrastructures/database"
	dbMigration "github.com/reyimanuel/letter-administration/internal/migration"
	"github.com/reyimanuel/letter-administration/internal/server"
)

func main() {
	// Load global configuration
	config.Load()

	// Jika ada command → jalankan CLI mode
	if len(os.Args) > 1 {
		handleCLI()
		return
	}

	// Default behavior → run HTTP server
	server.Run()
}

func handleCLI() {
	cmd := strings.ToLower(os.Args[1])
	force := hasFlag("--force")

	switch cmd {
	case "migrate":
		runMigrations(force)
	case "migrate-only":
		runMigrationsOnly()
	case "seed":
		runSeed(force)
	case "reset":
		if !force {
			if blocked := guardLocalOnly(); blocked != "" {
				fmt.Println(blocked)
				return
			}
		}
		runReset(force)
	default:
		fmt.Println("Unknown command. Use:")
		fmt.Println("  migrate [--force]")
		fmt.Println("  migrate-only")
		fmt.Println("  seed [--only=users|roles,users|all] [--truncate-all] [--force]")
		fmt.Println("  reset [--force]")
	}
}

// Helper CLI
func runMigrations(force bool) {
	db, _, err := database.ConnectDB()
	if err != nil {
		panic(err)
	}

	if err := dbMigration.RunMigration(db, force); err != nil {
		panic(err)
	}
}

func runMigrationsOnly() {
	db, _, err := database.ConnectDB()
	if err != nil {
		panic(err)
	}

	if err := dbMigration.RunMigrationOnly(db); err != nil {
		panic(err)
	}
}

func runReset(force bool) {
	db, _, err := database.ConnectDB()
	if err != nil {
		panic(err)
	}

	fmt.Println("Dropping all tables...")
	if err := dbMigration.DropMigration(db); err != nil {
		panic(err)
	}

	fmt.Println("Recreating tables...")
	if err := dbMigration.RunMigration(db, force); err != nil {
		panic(err)
	}

	fmt.Println("Database reset completed ✅")
}

func runSeed(force bool) {
	db, _, err := database.ConnectDB()
	if err != nil {
		panic(err)
	}

	only := ""
	if v, ok := getFlagValue("--only"); ok {
		only = v
	} else {
		// Allow positional target: `seed users`
		for _, a := range os.Args[2:] {
			if strings.HasPrefix(a, "-") {
				continue
			}
			only = a
			break
		}
	}
	if only == "" {
		only = "all"
	}

	targets, err := dbMigration.ParseSeedTargets(only)
	if err != nil {
		panic(err)
	}
	targets.TruncateAll = hasFlag("--truncate-all")

	if err := dbMigration.SeedSelected(db, force, targets); err != nil {
		panic(err)
	}
}

func guardLocalOnly() string {
	host := os.Getenv("DB_HOST")
	if host != "localhost" && host != "127.0.0.1" {
		return fmt.Sprintf(
			"Blocked: This operation can only be run locally (DB_HOST=%s)",
			host,
		)
	}
	return ""
}

func hasFlag(flag string) bool {
	return slices.Contains(os.Args[2:], flag)
}

func getFlagValue(flag string) (string, bool) {
	// Supports:
	// - --only=value
	// - --only value
	args := os.Args[2:]
	prefix := flag + "="
	for i := 0; i < len(args); i++ {
		a := args[i]
		if strings.HasPrefix(a, prefix) {
			v := strings.TrimSpace(strings.TrimPrefix(a, prefix))
			if v == "" {
				return "", false
			}
			return v, true
		}
		if a == flag {
			if i+1 >= len(args) {
				return "", false
			}
			v := strings.TrimSpace(args[i+1])
			if v == "" || strings.HasPrefix(v, "-") {
				return "", false
			}
			return v, true
		}
	}
	return "", false
}
