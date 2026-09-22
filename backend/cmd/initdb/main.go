package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/Homerus25/podland/backend/internal/domain/users"
	"github.com/Homerus25/podland/backend/internal/errors"
	"github.com/Homerus25/podland/backend/internal/models"
	"github.com/Homerus25/podland/backend/migrations"
)

func main() {
	// Parse command line arguments
	username := flag.String("username", "admin", "Admin username")
	password := flag.String("password", "", "Admin password")
	dbPath := flag.String("db", "./podland.db", "Database path")
	flag.Parse()

	// Change working directory to the backend root to ensure migrations can be found
	if err := os.Chdir("../.."); err != nil {
		log.Fatalf("Failed to change working directory: %v", err)
	}

	// The chdir above only lands on the backend root when this tool is
	// started from cmd/initdb (as `make initDB` does). A binary started
	// anywhere else — e.g. from backend/ — climbs past the root and would
	// otherwise open its default database in the wrong place, so refuse
	// before any file is touched.
	if _, err := os.Stat("migrations"); err != nil {
		log.Fatalf("No migrations/ directory here — start initdb from cmd/initdb (or run `make initDB`): %v", err)
	}

	// Validate required arguments
	if *password == "" {
		fmt.Println("Error: password is required")
		flag.Usage()
		os.Exit(1)
	}

	// Connect to database
	db, err := models.NewDatabase(*dbPath)
	if err != nil {
		log.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	// Run migrations to ensure tables exist
	if err := migrations.Run(db); err != nil {
		log.Fatalf("Failed to run migrations: %v", err)
	}

	// Create the bootstrap admin user
	sessionRepo := models.NewSessionRepository(db)
	if err := createAdminUser(models.NewUserRepository(db), sessionRepo, *username, *password); err != nil {
		log.Fatalf("Failed to create admin user: %v", err)
	}
}

// createAdminUser creates the bootstrap user with the admin role by
// riding the same user-creation service as the POST /api/users
// endpoint (hash + exists-check + insert). If the username already
// exists it is left untouched — re-running the bootstrap tool against
// an existing admin is an idempotent no-op.
func createAdminUser(userRepo models.UserRepositoryInterface, sessionRepo models.SessionRepositoryInterface, username, password string) error {
	_, err := users.NewUserService(userRepo, sessionRepo).CreateUserWithRole(username, password, models.RoleAdmin)
	if err != nil {
		if err == errors.ErrUsernameTaken {
			log.Printf("User '%s' already exists", username)
			return nil
		}
		return err
	}

	log.Printf("Successfully created admin user '%s'", username)
	return nil
}
