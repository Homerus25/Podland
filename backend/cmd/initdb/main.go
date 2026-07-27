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

	// Create user repository
	userRepo := models.NewUserRepository(db)

	// Check if user already exists
	exists, err := userRepo.UsernameExists(*username)
	if err != nil {
		log.Fatalf("Failed to check if user exists: %v", err)
	}

	if exists {
		log.Printf("User '%s' already exists", *username)
		os.Exit(0)
	}

	// Hash password
	passwordHash, err := bcrypt.GenerateFromPassword([]byte(*password), bcrypt.DefaultCost)
	if err != nil {
		log.Fatalf("Failed to hash password: %v", err)
	}

	// Create new user
	user := &models.User{
		ID:           uuid.New().String(),
		Username:     *username,
		PasswordHash: string(passwordHash),
		CreatedAt:    time.Now(),
	}

	// Create user in database
	err = userRepo.Create(user)
	if err != nil {
		log.Fatalf("Failed to create user: %v", err)
	}

	log.Printf("Successfully created admin user '%s'", *username)
}

