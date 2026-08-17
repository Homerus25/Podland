package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/yourorg/podland/backend/internal/domain/users"
	"github.com/yourorg/podland/backend/internal/errors"
	"github.com/yourorg/podland/backend/internal/models"
	"github.com/yourorg/podland/backend/migrations"
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
	if err := createAdminUser(models.NewUserRepository(db), *username, *password); err != nil {
		log.Fatalf("Failed to create admin user: %v", err)
	}
}

// createAdminUser creates the bootstrap user with the admin role by
// riding the same user-creation service as the POST /api/users
// endpoint (hash + exists-check + insert). If the username already
// exists it is left untouched — re-running the bootstrap tool against
// an existing admin is an idempotent no-op.
func createAdminUser(userRepo models.UserRepositoryInterface, username, password string) error {
	_, err := users.NewUserService(userRepo).CreateUserWithRole(username, password, models.RoleAdmin)
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
