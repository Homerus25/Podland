package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/google/uuid"
	"github.com/yourorg/podland/backend/internal/models"
	"github.com/yourorg/podland/backend/migrations"
	"golang.org/x/crypto/bcrypt"
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

// createAdminUser creates the bootstrap user with the admin role. If the
// username already exists it is left untouched — re-running the bootstrap
// tool against an existing admin is an idempotent no-op.
func createAdminUser(userRepo models.UserRepositoryInterface, username, password string) error {
	exists, err := userRepo.UsernameExists(username)
	if err != nil {
		return fmt.Errorf("failed to check if user exists: %w", err)
	}

	if exists {
		log.Printf("User '%s' already exists", username)
		return nil
	}

	// Hash password
	passwordHash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("failed to hash password: %w", err)
	}

	// Create new user with the admin role so it can manage other users
	user := &models.User{
		ID:           uuid.New().String(),
		Username:     username,
		PasswordHash: string(passwordHash),
		Role:         models.RoleAdmin,
		CreatedAt:    time.Now(),
	}

	if err := userRepo.Create(user); err != nil {
		return fmt.Errorf("failed to create user: %w", err)
	}

	log.Printf("Successfully created admin user '%s'", username)
	return nil
}
