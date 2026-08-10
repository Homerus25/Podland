package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	chimid "github.com/go-chi/chi/v5/middleware"
	appmiddleware "github.com/yourorg/podland/backend/internal/middleware"

	"github.com/yourorg/podland/backend/internal/config"
	"github.com/yourorg/podland/backend/internal/connections"
	"github.com/yourorg/podland/backend/internal/domain/auth"
	"github.com/yourorg/podland/backend/internal/domain/containers"
	"github.com/yourorg/podland/backend/internal/domain/events"
	"github.com/yourorg/podland/backend/internal/domain/images"
	"github.com/yourorg/podland/backend/internal/domain/machines"
	"github.com/yourorg/podland/backend/internal/domain/quadlets"
	"github.com/yourorg/podland/backend/internal/encryption"
	"github.com/yourorg/podland/backend/internal/errors"
	"github.com/yourorg/podland/backend/internal/middleware"
	"github.com/yourorg/podland/backend/internal/models"
	"github.com/yourorg/podland/backend/internal/sshexec"
	"github.com/yourorg/podland/backend/migrations"
)

func main() {
	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	// Initialize database
	db, err := models.NewDatabase(cfg.Database.Path)
	if err != nil {
		log.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	// Run migrations
	if err := migrations.Run(db); err != nil {
		log.Fatalf("Failed to run migrations: %v", err)
	}

	// Initialize encryption
	encryptionService, err := encryption.NewEncryptionService(cfg.Encryption.Key)
	if err != nil {
		log.Fatalf("Failed to initialize encryption: %v", err)
	}

	// Initialize JWT (load or generate keys)
	authService, err := auth.NewAuthService(auth.JWTConfig{
		PrivateKeyPath: cfg.JWT.PrivateKeyPath,
		PublicKeyPath:  cfg.JWT.PublicKeyPath,
		Expiration:     time.Duration(cfg.JWT.ExpirationMinutes) * time.Minute,
		Issuer:         cfg.JWT.Issuer,
	}, models.NewUserRepository(db))
	if err != nil {
		log.Fatalf("Failed to initialize auth: %v", err)
	}

	// Initialize repositories
	userRepo := models.NewUserRepository(db)
	machineRepo := models.NewMachineRepository(db)
	accessRepo := models.NewMachineAccessRepository(db)

	// Initialize connection manager
	connectionFactory := connections.NewConnectionFactory(connections.ConnectionFactoryOptions{
		MachineRepo: machineRepo,
		Encryption:  encryptionService,
	})
	connectionCache := connections.NewConnectionCache(connections.ConnectionCacheOptions{
		MaxSize:  cfg.Connections.MaxConcurrent,
		Factory:  connectionFactory,
	})
	connectionManager := connections.NewConnectionManager(
		connectionCache,
		machineRepo,
		encryptionService,
		cfg.Connections.MaxConcurrent,
		time.Duration(cfg.Connections.QueueTimeoutSeconds)*time.Second,
	)
	go connectionManager.Run()

	// sshExecService is the SSH-exec capability (sshexec.SSHExecInterface).
	// It is constructed here so its dialer cache + cleanup goroutine start
	// at boot, but it has no HTTP route of its own — it is infrastructure
	// consumed by the quadlet lifecycle endpoints (070-06), which run
	// `systemctl` over SSH.
	sshExecService := sshexec.NewService(sshexec.Options{
		MachineRepo: machineRepo,
		Encryption:  encryptionService,
	})

	// Initialize services
	machineService := machines.NewMachineService(machineRepo, accessRepo, connectionManager, encryptionService)
	containerService := containers.NewContainerService(connectionManager)
	imageService := images.NewImageService(connectionManager)
	quadletService := quadlets.NewQuadletService(connectionManager, sshExecService)
	eventService := events.NewEventService(connectionManager)

	// Initialize handlers
	authHandler := auth.NewAuthHandler(authService, userRepo)
	machineHandler := machines.NewMachineHandler(machineService)
	containerHandler := containers.NewContainerHandler(containerService, machineService)
	imageHandler := images.NewImageHandler(imageService, machineService)
	quadletHandler := quadlets.NewQuadletHandler(quadletService, machineService)
	eventHandler := events.NewEventHandler(eventService, machineService, errors.NewErrorHandler())

	// Initialize middleware
	rateLimiter := middleware.NewRateLimiter(
		cfg.RateLimit.RequestsPerMinute,
		time.Minute,
	)

	// Create router
	router := chi.NewRouter()

	// Global middleware
	router.Use(chimid.Recoverer)
	router.Use(appmiddleware.RequestIDMiddleware)
	router.Use(appmiddleware.CORS(cfg.AllowedOrigins))

	// Routes
	router.Route("/api", func(r chi.Router) {
		// Auth
		r.Post("/auth/login", authHandler.Login)
		r.Post("/auth/logout", authHandler.Logout)
		r.Get("/auth/status", authHandler.Status)

		// Protected routes
		r.Group(func(r chi.Router) {
			r.Use(appmiddleware.JWTMiddleware(authService))
			r.Use(appmiddleware.RateLimitMiddleware(rateLimiter))

			// Machines
			r.Route("/machines", func(r chi.Router) {
				r.Post("/", machineHandler.Create)
				r.Get("/", machineHandler.List)
				r.Route("/{id}", func(r chi.Router) {
					r.Get("/", machineHandler.Get)
					r.Put("/", machineHandler.Update)
					r.Delete("/", machineHandler.Delete)
					r.Get("/access", machineHandler.ListAccess)
					r.Post("/access", machineHandler.GrantAccess)
					r.Delete("/access/{userId}", machineHandler.RevokeAccess)

					// Containers
					r.Route("/containers", func(r chi.Router) {
						r.Get("/", containerHandler.List)
						r.Post("/", containerHandler.Create)
						r.Route("/{containerId}", func(r chi.Router) {
							r.Get("/", containerHandler.Get)
							r.Post("/start", containerHandler.Start)
							r.Post("/stop", containerHandler.Stop)
							r.Delete("/", containerHandler.Delete)
							r.Get("/logs", containerHandler.Logs)
						})
					})

					// Images
					r.Route("/images", func(r chi.Router) {
						r.Get("/", imageHandler.List)
						r.Post("/pull", imageHandler.Pull)
						r.Delete("/{imageId}", imageHandler.Delete)
					})

					// Quadlets
					r.Route("/quadlets", func(r chi.Router) {
						r.Get("/", quadletHandler.List)
						r.Post("/", quadletHandler.Create)
						r.Post("/import", quadletHandler.Import)
						r.Route("/{quadletId}", func(r chi.Router) {
							r.Get("/", quadletHandler.Get)
							r.Put("/", quadletHandler.Update)
							r.Delete("/", quadletHandler.Delete)
							r.Post("/start", quadletHandler.Start)
							r.Post("/stop", quadletHandler.Stop)
							r.Post("/restart", quadletHandler.Restart)
							r.Get("/logs", quadletHandler.GetLogs)
							r.Post("/generate", quadletHandler.Generate)
						})
					})

					// Events
					r.Get("/events", eventHandler.HandleEvents)
				})
			})
		})
	})

	// Health check endpoint
	router.Get("/api/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})

	// Start server
	server := &http.Server{
		Addr:              fmt.Sprintf(":%d", cfg.Port),
		Handler:           router,
		ReadHeaderTimeout: 10 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	log.Printf("Starting server on port %d", cfg.Port)

	// Graceful shutdown
	done := make(chan os.Signal, 1)
	signal.Notify(done, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Server failed: %v", err)
		}
	}()

	<-done
	log.Println("Shutting down server")

	// Shutdown server
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		log.Fatalf("Server shutdown failed: %v", err)
	}

	log.Println("Server stopped")
}
