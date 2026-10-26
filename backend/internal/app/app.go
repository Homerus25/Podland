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

	"github.com/Homerus25/podland/backend/internal/config"
	"github.com/Homerus25/podland/backend/internal/connections"
	"github.com/Homerus25/podland/backend/internal/domain/auth"
	"github.com/Homerus25/podland/backend/internal/domain/containers"
	"github.com/Homerus25/podland/backend/internal/domain/images"
	"github.com/Homerus25/podland/backend/internal/domain/machines"
	"github.com/Homerus25/podland/backend/internal/domain/networks"
	"github.com/Homerus25/podland/backend/internal/domain/pods"
	"github.com/Homerus25/podland/backend/internal/domain/quadlets"
	"github.com/Homerus25/podland/backend/internal/domain/secrets"
	"github.com/Homerus25/podland/backend/internal/domain/users"
	"github.com/Homerus25/podland/backend/internal/domain/volumes"
	"github.com/Homerus25/podland/backend/internal/encryption"
	"github.com/Homerus25/podland/backend/internal/failurebudget"
	appmiddleware "github.com/Homerus25/podland/backend/internal/middleware"
	"github.com/Homerus25/podland/backend/internal/models"
	"github.com/Homerus25/podland/backend/internal/sshexec"
	"github.com/Homerus25/podland/backend/internal/webui"
	"github.com/Homerus25/podland/backend/migrations"
	"github.com/go-chi/chi/v5"
	chimid "github.com/go-chi/chi/v5/middleware"
)

// App owns every backend resource and background goroutine constructed by
// the previous cmd/server/main.go. The zero value is not usable — build
// one with New.
type App struct {
	db                *sql.DB
	connectionManager *connections.ConnectionManager
	sshExecService    *sshexec.Service

	// userSvc is the single user-creation path (hash + exists-check +
	// insert). The POST /api/users endpoint, the first-run setup
	// endpoint, and the SeedUserWithRole test seam all go through it.
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

	// User management and the first-run setup endpoint share one
	// creation path; the session repo backs the admin password reset's
	// session revocation. The service is also the auth service's
	// FirstUserCreator: the first-run setup endpoint rides the same
	// creation path (hash + validation + the atomic first-user guard).
	userRepo := models.NewUserRepository(db)

	// The credential-failure budgets (issues 185-187): keyed not by IP
	// or user ID but by the identity a request claims — the login body's
	// username, the password change's and the admin reset's caller ID —
	// so they cannot live in middleware. One failurebudget.Surface per
	// credential endpoint, each over its own limiter instance over the
	// same config pair (issue 236): the Surface owns the
	// endpoint-namespaced key, so a username and a caller ID can never
	// collide (issue 196), and the surface-name strings live here, at
	// the wiring site. Each surface value is handed to its endpoint's
	// service and handler as one value.
	newFailureSurface := func(name string) *failurebudget.Surface {
		return failurebudget.NewSurface(name, appmiddleware.NewRateLimiter(
			cfg.RateLimit.FailureBudget,
			time.Duration(cfg.RateLimit.FailureWindowMinutes)*time.Minute,
		))
	}
	loginBudget := newFailureSurface("login")
	changeBudget := newFailureSurface("change-password")
	resetBudget := newFailureSurface("reset-password")

	userService := users.NewUserService(userRepo, sessionRepo).WithFailureBudget(resetBudget)

	authService, err := auth.NewAuthService(auth.JWTConfig{
		PrivateKeyPath: cfg.JWT.PrivateKeyPath,
		PublicKeyPath:  cfg.JWT.PublicKeyPath,
		Expiration:     time.Duration(cfg.JWT.ExpirationMinutes) * time.Minute,
		Issuer:         cfg.JWT.Issuer,
	}, userRepo, sessionRepo, time.Duration(cfg.Session.TTLDays)*24*time.Hour, time.Duration(cfg.Session.RememberTTLDays)*24*time.Hour, userService)
	if err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("app: failed to initialize auth: %w", err)
	}
	authService = authService.WithLoginBudget(loginBudget).WithChangeBudget(changeBudget)

	// Initialize repositories
	machineRepo := models.NewMachineRepository(db)
	accessRepo := models.NewMachineAccessRepository(db)

	// Initialize connection manager
	connectionManager := connections.NewConnectionManager(
		machineRepo,
		encryptionService,
		cfg.Connections.MaxConcurrent,
		time.Duration(cfg.Connections.TestTimeoutSeconds)*time.Second,
	)

	// sshExecService is the SSH-exec capability (sshexec.SSHExecInterface).
	// It is constructed here so its dialer cache + cleanup goroutine start
	// at boot, but it has no HTTP route of its own — it is infrastructure
	// consumed by the quadlet lifecycle endpoints (070-06), which run
	// `systemctl` over SSH.
	sshExecService := sshexec.NewService(sshexec.Options{
		MachineRepo: machineRepo,
		Encryption:  encryptionService,
	})

	// Initialize services. sshExecService is handed to the machine service
	// too: it satisfies the machines package's narrow sshInvalidator seam,
	// so machine deletion can invalidate the machine's pooled SSH clients.
	machineService := machines.NewMachineService(machineRepo, accessRepo, connectionManager, encryptionService, sshExecService)
	containerService := containers.NewContainerService(containers.NewContainerEngineProvider(connectionManager))
	imageService := images.NewImageService(images.NewImageEngineProvider(connectionManager)).
		WithAccessRepo(accessRepo).
		WithMaxImageTransferBytes(cfg.Images.MaxTransferBytes)
	// The quadlets domain is two modules (architecture review 2026-10-19,
	// candidate C2): the lean QuadletService (CRUD, lifecycle, logs,
	// generate, import, auto-update) and the QuadletTransferService
	// (cross-machine move/copy), which crosses into the lean half through
	// the lifecycle seam the lean service satisfies.
	quadletEngineProvider := quadlets.NewQuadletEngineProvider(connectionManager)
	// The compose-to-quadlet conversion (spec #336) runs behind the
	// quadlets domain's ComposeConverter seam; the production adapter
	// shells out to the podlet binary on PATH.
	quadletService := quadlets.NewQuadletService(quadletEngineProvider, sshExecService, machineRepo).
		WithComposeConverter(quadlets.NewPodletComposeConverter())
	quadletTransferService := quadlets.NewQuadletTransferService(
		quadletEngineProvider,
		quadlets.NewQuadletSecretEngineProvider(connectionManager),
		accessRepo,
		cfg.Volumes.MaxUploadBytes,
		quadletService,
	)
	secretService := secrets.NewSecretService(secrets.NewSecretEngineProvider(connectionManager), accessRepo).
		WithUsageEngines(secrets.NewSecretUsageEngineProvider(connectionManager))
	volumeService := volumes.NewVolumeService(volumes.NewVolumeEngineProvider(connectionManager)).
		WithAccessRepo(accessRepo).
		WithUsageEngines(volumes.NewVolumeUsageEngineProvider(connectionManager)).
		WithMaxVolumeUploadBytes(cfg.Volumes.MaxUploadBytes)
	networkService := networks.NewNetworkService(networks.NewNetworkEngineProvider(connectionManager))
	podService := pods.NewPodService(pods.NewPodEngineProvider(connectionManager))

	// Initialize handlers. Each credential endpoint's failure-budget
	// surface is the same value its service checks and spends — the
	// handler's half only advertises the trio on the budget's 429s.
	authHandler := auth.NewAuthHandler(authService, userRepo).
		WithLoginBudget(loginBudget).
		WithChangeBudget(changeBudget)
	userHandler := users.NewUserHandler(userService).WithFailureBudget(resetBudget)
	machineHandler := machines.NewMachineHandler(machineService)
	containerHandler := containers.NewContainerHandler(containerService)
	imageHandler := images.NewImageHandler(imageService)
	quadletHandler := quadlets.NewQuadletHandler(quadletService, quadletTransferService)
	secretHandler := secrets.NewSecretHandler(secretService)
	volumeHandler := volumes.NewVolumeHandler(volumeService).
		WithMaxUploadBytes(cfg.Volumes.MaxUploadBytes)
	networkHandler := networks.NewNetworkHandler(networkService)
	podHandler := pods.NewPodHandler(podService)

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
	// Request logging lives here, not per handler: one line per request
	// (method, path, status, duration, requestID) covering every
	// endpoint, streaming included (long-lived streams log at stream
	// end). Runs inside RequestIDMiddleware so the ID is in context.
	router.Use(appmiddleware.RequestLogging)

	// Routes
	router.Route("/api", func(r chi.Router) {
		// Auth
		//
		// Login, refresh, the anonymous status probe, and first-run setup
		// are the unauthenticated entry points of the auth surface, so
		// they share an IP-keyed rate-limit group — brute-force and
		// enumeration protection that needs no user to key on. Logout
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
			// Status serves anonymous callers too (authenticated: false,
			// plus whether a fresh instance still needs setup), so it
			// validates the token only when one is presented — and rides
			// the same per-IP group: the needs-setup flag is exactly what
			// a fresh-instance enumerator wants.
			r.With(appmiddleware.OptionalJWTMiddleware(authService)).Get("/auth/status", authHandler.Status)
			// First-run setup is the bootstrap entry point (issue 179):
			// on a user-less instance it creates the first admin and signs
			// the caller in (login payload + refresh cookie). The atomic
			// zero-user guard lives in the creation service; a configured
			// instance answers 409 SETUP_COMPLETED with no side effects.
			r.Post("/setup/user", authHandler.Setup)
		})
		r.Post("/auth/logout", authHandler.Logout)
		// Password change is strictly authenticated (the JWT middleware
		// is the guarantor; authority comes from the current password in
		// the body) and lives under /api/auth so the fresh refresh cookie
		// it hands back shares the login/refresh cookie scope. All of the
		// caller's other sessions die in AuthService.ChangePassword.
		r.With(appmiddleware.JWTMiddleware(authService)).Post("/auth/password", authHandler.ChangePassword)

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
				// endpoints and the containers/images/quadlets/secrets/
				// volumes/networks families that ride on a machine.
				r.With(appmiddleware.RequireMachineAccess(machineRepo, accessRepo)).Route("/{id}", func(r chi.Router) {
					r.Get("/", machineHandler.Get)
					r.Get("/status", machineHandler.Status)
					r.Put("/", machineHandler.Update)
					r.Delete("/", machineHandler.Delete)
					r.Get("/access", machineHandler.ListAccess)
					r.Post("/access", machineHandler.GrantAccess)
					r.Delete("/access/{userId}", machineHandler.RevokeAccess)

					// requireWrite is the level-aware gate for mutating
					// routes: the subtree-level RequireMachineAccess above
					// only proves an access row exists, so without this a
					// read-level grantee could change remote state. Read-only
					// routes (lists, gets, logs, generate) stay at the
					// subtree level.
					requireWrite := appmiddleware.RequireMachinePermission(machineRepo, accessRepo, models.PermissionWrite)

					// Containers
					r.Route("/containers", func(r chi.Router) {
						r.Get("/", containerHandler.List)
						r.With(requireWrite).Post("/", containerHandler.Create)
						r.Route("/{containerId}", func(r chi.Router) {
							r.Get("/", containerHandler.Get)
							r.With(requireWrite).Post("/start", containerHandler.Start)
							r.With(requireWrite).Post("/stop", containerHandler.Stop)
							r.With(requireWrite).Delete("/", containerHandler.Delete)
							r.Get("/logs", containerHandler.Logs)
							r.Get("/inspect", containerHandler.Inspect)
						})
					})

					// Images
					r.Route("/images", func(r chi.Router) {
						r.Get("/", imageHandler.List)
						r.With(requireWrite).Post("/pull", imageHandler.Pull)
						// Bulk deletion, so write-level like the volume
						// prune. No shadowing hazard: the parametric
						// routes are a DELETE and a GET on distinct
						// paths, and the router routes by method+path.
						r.With(requireWrite).Post("/prune", imageHandler.Prune)
						// Detail DTO (list fields + inspect config), read-level
						// like the raw inspect route below.
						r.Get("/{imageId}", imageHandler.Get)
						r.With(requireWrite).Delete("/{imageId}", imageHandler.Delete)
						r.Get("/{imageId}/inspect", imageHandler.Inspect)
						// Copy preview stays read-level: it mutates
						// nothing on the source, and the target
						// machine's read access is enforced inside
						// the service against the access store
						// (issue 375).
						r.Get("/{imageId}/copy-preview", imageHandler.CopyPreview)
						// The copy mutates remote state (it loads
						// on the target machine), so it gates at
						// write level on the source; the target
						// machine's write access is enforced
						// inside the service against the access
						// store — route middleware only sees the
						// route's machine (issue 375).
						r.With(requireWrite).Post("/{imageId}/copy", imageHandler.Copy)
					})

					// Quadlets
					r.Route("/quadlets", func(r chi.Router) {
						r.Get("/", quadletHandler.List)
						r.With(requireWrite).Post("/", quadletHandler.Create)
						r.With(requireWrite).Post("/import", quadletHandler.Import)
						r.With(requireWrite).Post("/auto-update", quadletHandler.AutoUpdate)
						r.Route("/{quadletId}", func(r chi.Router) {
							r.Get("/", quadletHandler.Get)
							r.With(requireWrite).Put("/", quadletHandler.Update)
							r.With(requireWrite).Delete("/", quadletHandler.Delete)
							r.With(requireWrite).Post("/start", quadletHandler.Start)
							r.With(requireWrite).Post("/stop", quadletHandler.Stop)
							r.With(requireWrite).Post("/restart", quadletHandler.Restart)
							r.Get("/logs", quadletHandler.GetLogs)
							r.Post("/generate", quadletHandler.Generate)
							// Move/copy preview stays read-level: it mutates
							// nothing on the source, and the target machine's
							// read access is enforced inside the service
							// against the access store (issue 209).
							r.Get("/move-preview", quadletHandler.MovePreview)
							// The transfer mutates remote state (it installs on
							// the target machine), so it gates at write level on
							// the source; the target machine's write access is
							// enforced inside the service against the access
							// store — route middleware only sees the route's
							// machine (issue 210).
							r.With(requireWrite).Post("/move", quadletHandler.Move)
						})
					})

					// Secrets — read-level subtree: secret METADATA (list +
					// inspect) is visible to any grantee (subtree membership
					// is the read gate). The value reveal is NOT a
					// route-level gate: ?reveal=true is a query param, so
					// Service.Get checks the caller's level itself and
					// answers 403 ACCESS_DENIED below admin. Creating and
					// deleting secrets are remote-state mutations, so both
					// are gated at write (160-03/160-04).
					r.Route("/secrets", func(r chi.Router) {
						r.Get("/", secretHandler.List)
						r.With(requireWrite).Post("/", secretHandler.Create)
						r.Route("/{secretId}", func(r chi.Router) {
							r.Get("/", secretHandler.Get)
							r.Get("/inspect", secretHandler.Inspect)
							// Move/copy preview stays read-level: it mutates
							// nothing and reveals nothing on the source, and
							// the target machine's read access is enforced
							// inside the service against the access store
							// (issue 239).
							r.Get("/move-preview", secretHandler.MovePreview)
							// The transfer mutates remote state (it creates
							// on the target machine) and the value leaving the
							// source is a reveal, so the service gates the
							// source at admin on top of this write gate; the
							// target machine's write access is enforced inside
							// the service against the access store — route
							// middleware only sees the route's machine (issue
							// 239).
							r.With(requireWrite).Post("/move", secretHandler.Move)
							r.With(requireWrite).Delete("/", secretHandler.Delete)
						})
					})

					// Volumes — list/inspect stay read-level; create (and
					// later slices) mutate remote state, so they gate at
					// write (200-03).
					r.Route("/volumes", func(r chi.Router) {
						r.Get("/", volumeHandler.List)
						r.With(requireWrite).Post("/", volumeHandler.Create)
						// The literal /prune route must beat the
						// /{volumeName} subtree or prune calls get
						// swallowed as a volume named "prune".
						r.With(requireWrite).Post("/prune", volumeHandler.Prune)
						r.Route("/{volumeName}", func(r chi.Router) {
							r.Get("/", volumeHandler.Get)
							// Export stays read-level: users who can
							// read a machine's state can read its data.
							r.Get("/export", volumeHandler.Export)
							// Move/copy preview stays read-level: it mutates
							// nothing on the source, and the target machine's
							// read access is enforced inside the service
							// against the access store (issue 238).
							r.Get("/move-preview", volumeHandler.MovePreview)
							r.With(requireWrite).Post("/populate", volumeHandler.Populate)
							// The transfer mutates remote state (it creates
							// on the target machine), so it gates at write
							// level on the source; the target machine's write
							// access is enforced inside the service against
							// the access store — route middleware only sees
							// the route's machine (issue 238).
							r.With(requireWrite).Post("/move", volumeHandler.Move)
							r.With(requireWrite).Delete("/", volumeHandler.Delete)
						})
					})

					// Networks — the list and the detail endpoint
					// stay at the subtree's read level; create and
					// connect mutate remote state, so they gate at
					// write (issue 252).
					r.Route("/networks", func(r chi.Router) {
						r.Get("/", networkHandler.List)
						r.Get("/{networkName}", networkHandler.Get)
						r.Get("/{networkName}/inspect", networkHandler.Inspect)
						r.With(requireWrite).Post("/", networkHandler.Create)
						r.With(requireWrite).Post("/{networkName}/connect", networkHandler.Connect)
					})

					// Pods — the list, detail, and raw inspect
					// endpoints stay at the subtree's read level;
					// create, prune, lifecycle actions, and remove
					// mutate remote state, so they gate at write
					// (spec #353). The prune route is literal so it
					// is not shadowed by the {podId} subtree.
					r.Route("/pods", func(r chi.Router) {
						r.Get("/", podHandler.List)
						r.With(requireWrite).Post("/", podHandler.Create)
						r.With(requireWrite).Post("/prune", podHandler.Prune)
						r.Route("/{podId}", func(r chi.Router) {
							r.Get("/", podHandler.Get)
							r.Get("/inspect", podHandler.Inspect)
							r.With(requireWrite).Post("/pause", podHandler.Pause)
							r.With(requireWrite).Post("/unpause", podHandler.Unpause)
							r.With(requireWrite).Post("/kill", podHandler.Kill)
							r.With(requireWrite).Post("/restart", podHandler.Restart)
							r.With(requireWrite).Delete("/", podHandler.Remove)
						})
					})
				})
			})

			// Quadlets — the compose conversion endpoint is the first
			// quadlets route OUTSIDE the machine subtree (spec #336):
			// conversion touches no machine data, so any authenticated
			// user may call it — no RequireMachineAccess, no write
			// gate. It sits beside /machines and /users.
			r.Post("/quadlets/convert-compose", quadletHandler.ConvertCompose)

			// Users — the list endpoint is available to any authenticated
			// caller; the response shape depends on the caller's role
			// (shaped in users.UserService.List). Creating users,
			// changing roles, and resetting passwords stay admin-only
			// (090-01, 162, 165), the role change with the self-lockout
			// guard and the password reset with revoke-all-sessions in
			// the service. All sit beside /machines, not inside it.
			r.Get("/users", userHandler.List)
			r.With(appmiddleware.RequireAdmin(userRepo)).Post("/users", userHandler.Create)
			r.With(appmiddleware.RequireAdmin(userRepo)).Put("/users/{userId}/role", userHandler.UpdateRole)
			r.With(appmiddleware.RequireAdmin(userRepo)).Put("/users/{userId}/password", userHandler.ResetPassword)
		})
	})

	// Health check endpoint
	router.Get("/api/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("OK"))
	})

	// SPA — the frontend build embedded by internal/webui, mounted under
	// the catch-all AFTER /api: chi gives the API subtree precedence, so
	// every API route (and its 404s) stays API-shaped while all other
	// paths fall back to index.html, mirroring the Caddy container's
	// try_files contract this replaces.
	router.Handle("/*", webui.Handler())

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

// Close stops the background goroutines (connection-manager idle cleanup
// and sshexec dialer-cleanup) and closes the DB. Safe to call any number
// of times — subsequent calls are no-ops that return the first call's
// result.
func (a *App) Close() error {
	a.closeOnce.Do(func() {
		// Stop the connection manager (its idle-cleanup goroutine) first.
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
// (bcrypt-hashed with bcrypt.DefaultCost) into the App's database,
// returning the new user's ID. It is intended for the in-process
// integration harness (PRD 080-01), which needs a seeded user before the
// first real POST /api/auth/login can fly.
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
// can mint users of any role (e.g. an admin) without a backdoor around
// login. It rides the same user-creation service as the endpoints, so
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
