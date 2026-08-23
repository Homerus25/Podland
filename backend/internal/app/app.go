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
	"github.com/Homerus25/podland/backend/internal/config"
	"github.com/Homerus25/podland/backend/internal/connections"
	"github.com/Homerus25/podland/backend/internal/domain/auth"
	"github.com/Homerus25/podland/backend/internal/domain/containers"
	"github.com/Homerus25/podland/backend/internal/domain/events"
	"github.com/Homerus25/podland/backend/internal/domain/images"
	"github.com/Homerus25/podland/backend/internal/domain/machines"
	"github.com/Homerus25/podland/backend/internal/domain/quadlets"
	"github.com/Homerus25/podland/backend/internal/domain/users"
	"github.com/Homerus25/podland/backend/internal/encryption"
	"github.com/Homerus25/podland/backend/internal/errors"
	appmiddleware "github.com/Homerus25/podland/backend/internal/middleware"
	"github.com/Homerus25/podland/backend/internal/models"
	"github.com/Homerus25/podland/backend/internal/sshexec"
	"github.com/Homerus25/podland/backend/migrations"
)

// App owns every backend resource and background goroutine constructed by
// the previous cmd/server/main.go. The zero value is not usable — build
// one with New.
type App struct {
	db                *sql.DB
	connectionManager *connections.ConnectionManager
	sshExecService    *sshexec.Service

	// userSvc is the single user-creation path (hash + exists-check +
	// insert). The POST /api/users endpoint, cmd/initdb, and the
	// SeedUserWithRole test seam all go through it.
	userSvc *users.UserService

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
	sessionRepo := models.NewSessionRepository(db)
	authService, err := auth.NewAuthService(auth.JWTConfig{
		PrivateKeyPath: cfg.JWT.PrivateKeyPath,
		PublicKeyPath:  cfg.JWT.PublicKeyPath,
		Expiration:     time.Duration(cfg.JWT.ExpirationMinutes) * time.Minute,
		Issuer:         cfg.JWT.Issuer,
	}, models.NewUserRepository(db), sessionRepo, time.Duration(cfg.Session.TTLDays)*24*time.Hour)
	if err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("app: failed to initialize auth: %w", err)
	}

	// Initialize repositories
	userRepo := models.NewUserRepository(db)
	machineRepo := models.NewMachineRepository(db)
	accessRepo := models.NewMachineAccessRepository(db)

	// User management shares one creation path with cmd/initdb
	userService := users.NewUserService(userRepo)

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
	userHandler := users.NewUserHandler(userService)
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
	// Separate instance with its own budget: login and refresh are
	// unauthenticated, so the per-user limiter has no key for them —
	// the auth limiter keys by client IP instead.
	authRateLimiter := appmiddleware.NewRateLimiter(
		cfg.RateLimit.AuthRequestsPerMinute,
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
		//
		// Login and refresh are the unauthenticated entry points of the
		// auth surface, so they share an IP-keyed rate-limit group —
		// brute-force protection that needs no user to key on. Logout
		// stays outside the group: it is idempotent and revokes only the
		// cookie it is handed, so there is nothing to brute-force.
		r.Group(func(r chi.Router) {
			r.Use(appmiddleware.AuthRateLimitMiddleware(authRateLimiter))
			r.Post("/auth/login", authHandler.Login)
			// Refresh is deliberately public: it authenticates with the httpOnly
			// refresh cookie instead of a Bearer JWT — presenting that cookie to
			// a JWT-gated route would be a contradiction (the whole point is
			// minting a new JWT). Rotation lives inside AuthService.Refresh.
			r.Post("/auth/refresh", authHandler.Refresh)
		})
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
				// Every machine-scoped route below is gated on the
				// caller's machine_access row: 403 ACCESS_DENIED for a
				// machine the user cannot use, 404 MACHINE_NOT_FOUND
				// for an unknown ID. One seam covers the machines
				// endpoints and the containers/images/quadlets/events
				// families that ride on a machine.
				r.With(appmiddleware.RequireMachineAccess(machineRepo, accessRepo)).Route("/{id}", func(r chi.Router) {
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

			// Users — admin-only (090-01): creating users is the first
			// role-enforced endpoint. Sits beside /machines, not inside it.
			r.With(appmiddleware.RequireAdmin(userRepo)).Post("/users", userHandler.Create)
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
		userSvc:           userService,
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
// bootstrap user) without a backdoor around login. It rides the same
// user-creation service as the endpoint and the bootstrap tool, so
// hashing and duplicate detection cannot drift between them.
func (a *App) SeedUserWithRole(username, password, role string) (string, error) {
	if a.db == nil {
		return "", fmt.Errorf("app: SeedUserWithRole on closed app")
	}
	user, err := a.userSvc.CreateUserWithRole(username, password, role)
	if err != nil {
		return "", fmt.Errorf("app: create seeded user: %w", err)
	}
	return user.ID, nil
}
