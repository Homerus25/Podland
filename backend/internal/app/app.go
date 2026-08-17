// Package app owns the in-process backend wiring so the production
// `cmd/server` binary and the integration test harness (PRD 080-01) bring
// the backend up through the identical path.
//
// Before this package existed, every resource (DB, encryption, auth,
// repos, connection manager + goroutine, sshexec service + cleanup
// goroutine, services, handlers, middleware, router) was constructed
// inline in cmd/server/main.go. That worked for prod but blocked any
// in-process caller: there was no way to reach the configured router
// without binding a TCP port, and no way to tear the goroutines down
// cleanly. App closes both gaps.
//
// Ownership boundary: App owns resources and background goroutines; the
// caller owns the *http.Server. That keeps prod (cmd/server) and the
// in-process test (httptest.NewServer) on identical wiring.
package app

import (
	"database/sql"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	chimid "github.com/go-chi/chi/v5/middleware"
	"github.com/google/uuid"
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
	appmiddleware "github.com/yourorg/podland/backend/internal/middleware"
	"github.com/yourorg/podland/backend/internal/models"
	"github.com/yourorg/podland/backend/internal/sshexec"
	"github.com/yourorg/podland/backend/migrations"
	"golang.org/x/crypto/bcrypt"
)

// App owns every backend resource and background goroutine constructed by
// the previous cmd/server/main.go. The zero value is not usable — build
// one with New.
type App struct {
	db                *sql.DB
	connectionManager *connections.ConnectionManager
	sshExecService    *sshexec.Service

	router http.Handler

	closeOnce sync.Once
	closeErr  error
}

// New builds the entire backend wiring chain from an already-loaded config
// and returns an *App whose Handler is ready to serve. It mirrors the
// construction order of cmd/server/main.go exactly, including starting the
// connection-manager Run goroutine and the sshexec dialer-cleanup
// goroutine, so startup semantics are identical between prod and tests.
//
// Ownership boundary: New owns resources; the caller owns the *http.Server
// it wraps the returned Handler in. Use Close to release resources.
func New(cfg *config.Config) (*App, error) {
	if cfg == nil {
		return nil, fmt.Errorf("app: config is nil")
	}

	// Initialize database
	db, err := models.NewDatabase(cfg.Database.Path)
	if err != nil {
		return nil, fmt.Errorf("app: failed to open database: %w", err)
	}

	// Run migrations
	if err := migrations.Run(db); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("app: failed to run migrations: %w", err)
	}

	// Initialize encryption
	encryptionService, err := encryption.NewEncryptionService(cfg.Encryption.Key)
	if err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("app: failed to initialize encryption: %w", err)
	}

	// Initialize JWT (load or generate keys)
	authService, err := auth.NewAuthService(auth.JWTConfig{
		PrivateKeyPath: cfg.JWT.PrivateKeyPath,
		PublicKeyPath:  cfg.JWT.PublicKeyPath,
		Expiration:     time.Duration(cfg.JWT.ExpirationMinutes) * time.Minute,
		Issuer:         cfg.JWT.Issuer,
	}, models.NewUserRepository(db))
	if err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("app: failed to initialize auth: %w", err)
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
		MaxSize: cfg.Connections.MaxConcurrent,
		Factory: connectionFactory,
	})
	connectionManager := connections.NewConnectionManager(
		connectionCache,
		machineRepo,
		encryptionService,
		cfg.Connections.MaxConcurrent,
		time.Duration(cfg.Connections.QueueTimeoutSeconds)*time.Second,
		time.Duration(cfg.Connections.TestTimeoutSeconds)*time.Second,
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
	rateLimiter := appmiddleware.NewRateLimiter(
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
		// Status serves anonymous callers too (authenticated: false),
		// so it validates the token only when one is presented.
		r.With(appmiddleware.OptionalJWTMiddleware(authService)).Get("/auth/status", authHandler.Status)

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
		_, _ = w.Write([]byte("OK"))
	})

	return &App{
		db:                db,
		connectionManager: connectionManager,
		sshExecService:    sshExecService,
		router:            router,
	}, nil
}

// Handler returns the configured chi router. The returned handler is the
// same one production serves via *http.Server; pass it to
// httptest.NewServer for in-process tests.
func (a *App) Handler() http.Handler {
	return a.router
}

// Close stops the background goroutines (connection-manager processQueue
// and sshexec dialer-cleanup) and closes the DB. Safe to call any number
// of times — subsequent calls are no-ops that return the first call's
// result.
//
// Note: the connection cache's cleanup goroutine (internal/connections)
// has no stop mechanism today and is intentionally left running — it
// matches existing main() behaviour and is not on the PRD's stop list.
// Stopping it is a separate prefactoring concern.
func (a *App) Close() error {
	a.closeOnce.Do(func() {
		// Stop the connection-manager processQueue goroutine first so no
		// new connection requests are processed while we're tearing down.
		if a.connectionManager != nil {
			a.connectionManager.Stop()
		}
		// Then stop the sshexec dialer-cleanup goroutine and release
		// cached SSH clients.
		if a.sshExecService != nil {
			_ = a.sshExecService.Close()
		}
		// Finally close the DB.
		if a.db != nil {
			a.closeErr = a.db.Close()
		}
	})
	return a.closeErr
}

// SeedUser inserts a user with the given username and plaintext password
// (bcrypt-hashed with bcrypt.DefaultCost, matching cmd/initdb) into the
// App's database, returning the new user's ID. It is intended for the
// in-process integration harness (PRD 080-01), which needs a seeded user
// before the first real POST /api/auth/login can fly; production gets
// the same effect from cmd/initdb.
//
// Returning the ID lets the harness seed additional users and reference
// them by ID downstream — e.g. the machines CRUD/access slice (080-04)
// needs a second user's ID to exercise grant/revoke without minting a
// token for that user.
//
// This is a seam, not a backdoor: it does not bypass auth, mint tokens,
// or otherwise grant capabilities the HTTP API wouldn't. It only writes
// a row that the real login endpoint will then validate against.
func (a *App) SeedUser(username, password string) (string, error) {
	return a.SeedUserWithRole(username, password, models.RoleUser)
}

// SeedUserWithRole is SeedUser with an explicit role, so harness tests
// can mint users of any role (e.g. an admin mirroring the cmd/initdb
// bootstrap user) without a backdoor around login. Role values are
// validated by the repository like any other write.
func (a *App) SeedUserWithRole(username, password, role string) (string, error) {
	if a.db == nil {
		return "", fmt.Errorf("app: SeedUserWithRole on closed app")
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", fmt.Errorf("app: bcrypt hash: %w", err)
	}
	user := &models.User{
		ID:           uuid.New().String(),
		Username:     username,
		PasswordHash: string(hash),
		Role:         role,
		// cmd/initdb uses time.Now() (local); we use UTC for
		// determinism in tests. The difference is cosmetic — the
		// column stores whatever time.Now() returns in either case.
		CreatedAt: time.Now().UTC(),
	}
	repo := models.NewUserRepository(a.db)
	if err := repo.Create(user); err != nil {
		return "", fmt.Errorf("app: create seeded user: %w", err)
	}
	return user.ID, nil
}
