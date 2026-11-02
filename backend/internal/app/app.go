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
	"github.com/Homerus25/podland/backend/internal/contextutils"
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
	apperrors "github.com/Homerus25/podland/backend/internal/errors"
	"github.com/Homerus25/podland/backend/internal/failurebudget"
	appmiddleware "github.com/Homerus25/podland/backend/internal/middleware"
	"github.com/Homerus25/podland/backend/internal/models"
	"github.com/Homerus25/podland/backend/internal/sshexec"
	"github.com/Homerus25/podland/backend/internal/webui"
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

	// The access-token denylist (issue 441): the revoked_access_tokens
	// repository behind logout's per-device and the password paths'
	// per-user access-token revocation.
	revokedTokenRepo := models.NewRevokedTokenRepository(db)

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
	authService = authService.WithLoginBudget(loginBudget).WithChangeBudget(changeBudget).WithRevokedTokenRepo(revokedTokenRepo)

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
	// The host policy is the opt-in egress block on the registration probe
	// target (pentest 2026-11-01 finding 1): NewHostPolicy(false) is the
	// disabled zero value, so an unset/false MACHINE_HOST_BLOCK_PRIVATE_
	// RANGES leaves registration exactly as before.
	machineService := machines.NewMachineService(machineRepo, accessRepo, connectionManager, encryptionService, sshExecService).
		WithHostPolicy(machines.NewHostPolicy(cfg.Machines.HostBlockPrivateRanges))
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

	// Router (issue #451): nested *http.ServeMux with Go 1.22+ method
	// patterns — the standard library alone serves the identical API,
	// chi and chi/middleware gone from go.mod. The shape:
	//
	//   - A root mux: "/api/" strips the prefix and hands the API mux
	//     everything under /api; "/" is the SPA catch-all, so every API
	//     route (and its 404s) stays API-shaped while all other paths
	//     fall back to index.html, mirroring the Caddy container's
	//     try_files contract this replaces.
	//   - An API mux with method patterns ("GET /machines") and one
	//     sub-mux per middleware boundary, mounted with its middleware
	//     wrapping it: apiMux.Handle("/machines/{id}", requireMachine-
	//     Access(machineMux)) plus the /{id}/ subtree twin — the mount
	//     matches only when an {id} segment is present, so the slashed
	//     /machines/ form falls to the API catch-all's 404 instead of
	//     reaching the access middleware with an empty id.
	//   - Sub-muxes are mounted behind http.StripPrefix, so their
	//     patterns drop the stripped literal: the machine sub-mux is
	//     rooted at "{id}" ("GET /{id}/containers"), not at the full
	//     request path. The {id} segment itself stays in every
	//     pattern — the standard library has no wildcard-aware
	//     StripPrefix.
	//   - Per-route middleware (requireWrite) wraps individual handlers
	//     at registration.
	//   - Each mux level gets a "/" JSON-404 catch-all, so unknown /api
	//     paths answer the JSON error envelope at every depth, never
	//     the SPA's index.html.
	//
	// URLs canonicalize on the slashless form: ServeMux's subtree
	// patterns ("/machines/") would 301 "/machines" to "/machines/"
	// were the exact method patterns ("GET /machines") not registered
	// alongside — they are, so slashless serves and slashed forms fall
	// to the catch-alls' 404 (the frontend was swept: no production
	// call uses a trailing slash). One accepted quirk: an unregistered
	// method on a registered path may answer via ServeMux's subtree
	// redirect instead of chi's 405; no frontend or test sends such a
	// request.

	// apiNotFound is the JSON-404 catch-all every API mux level mounts
	// at "/": the error envelope, not the SPA shell and not net/http's
	// plain-text miss.
	apiNotFound := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		apperrors.RespondWithHTTPError(w, http.StatusNotFound, "NOT_FOUND", "Route not found", contextutils.GetRequestID(r.Context()))
	})

	// The machine sub-mux — the RequireMachineAccess boundary. The
	// mount strips the /machines literal (http.StripPrefix), so every
	// pattern below is rooted at the {id} segment; the {id} itself
	// stays in each pattern — the standard library has no
	// wildcard-aware StripPrefix. Each domain family (containers,
	// images, quadlets, secrets, volumes, networks, pods) is its own
	// sub-mux mounted here with the exact + subtree twin — the same
	// two-pattern trick the machine subtree itself is mounted with —
	// and carries its own "/" JSON-404 catch-all.
	machineMux := http.NewServeMux()

	// requireWrite is the level-aware gate for mutating routes: the
	// subtree-level RequireMachineAccess mounting this mux only proves
	// an access row exists, so without this a read-level grantee could
	// change remote state. Read-only routes (lists, gets, logs,
	// generate) stay at the subtree level.
	requireWrite := func(h http.HandlerFunc) http.Handler {
		return appmiddleware.RequireMachinePermission(machineRepo, accessRepo, models.PermissionWrite)(h)
	}

	// Machines. Every machine-scoped route below is gated on the
	// caller's machine_access row: 403 ACCESS_DENIED for a machine
	// the user cannot use, 404 MACHINE_NOT_FOUND for an unknown ID.
	// One seam covers the machines endpoints and the containers/
	// images/quadlets/secrets/volumes/networks/pods families that ride
	// on a machine.
	machineMux.HandleFunc("GET /{id}", machineHandler.Get)
	machineMux.HandleFunc("GET /{id}/status", machineHandler.Status)
	machineMux.HandleFunc("GET /{id}/version", machineHandler.Version)
	machineMux.HandleFunc("PUT /{id}", machineHandler.Update)
	machineMux.HandleFunc("DELETE /{id}", machineHandler.Delete)
	machineMux.HandleFunc("GET /{id}/access", machineHandler.ListAccess)
	machineMux.HandleFunc("POST /{id}/access", machineHandler.GrantAccess)
	machineMux.HandleFunc("DELETE /{id}/access/{userId}", machineHandler.RevokeAccess)

	// Containers. The literal prune and stats routes win over the
	// {containerId} subtree by ServeMux's literal-before-wildcard
	// precedence, so a container named "prune" or "stats" cannot
	// swallow them (the chi registration-order trick, now structural).
	containersMux := http.NewServeMux()
	containersMux.HandleFunc("GET /{id}/containers", containerHandler.List)
	containersMux.Handle("POST /{id}/containers", requireWrite(containerHandler.Create))
	// Bulk deletion, so write-level like the volume prune.
	containersMux.Handle("POST /{id}/containers/prune", requireWrite(containerHandler.Prune))
	// One-shot stats snapshot for every running container, read-level
	// like list/get/logs.
	containersMux.HandleFunc("GET /{id}/containers/stats", containerHandler.Stats)
	containersMux.HandleFunc("GET /{id}/containers/{containerId}", containerHandler.Get)
	containersMux.Handle("POST /{id}/containers/{containerId}/start", requireWrite(containerHandler.Start))
	containersMux.Handle("POST /{id}/containers/{containerId}/stop", requireWrite(containerHandler.Stop))
	containersMux.Handle("POST /{id}/containers/{containerId}/restart", requireWrite(containerHandler.Restart))
	containersMux.Handle("POST /{id}/containers/{containerId}/rename", requireWrite(containerHandler.Rename))
	containersMux.Handle("POST /{id}/containers/{containerId}/update", requireWrite(containerHandler.Update))
	containersMux.Handle("DELETE /{id}/containers/{containerId}", requireWrite(containerHandler.Delete))
	containersMux.HandleFunc("GET /{id}/containers/{containerId}/logs", containerHandler.Logs)
	containersMux.HandleFunc("GET /{id}/containers/{containerId}/inspect", containerHandler.Inspect)
	containersMux.HandleFunc("GET /{id}/containers/{containerId}/stats", containerHandler.ContainerStats)
	containersMux.Handle("/", apiNotFound)
	machineMux.Handle("/{id}/containers", containersMux)
	machineMux.Handle("/{id}/containers/", containersMux)

	// Images. Bulk deletion (prune), so write-level like the volume
	// prune. No shadowing hazard: the parametric routes are a DELETE
	// and a GET on distinct paths, and the router routes by
	// method+path.
	imagesMux := http.NewServeMux()
	imagesMux.HandleFunc("GET /{id}/images", imageHandler.List)
	imagesMux.Handle("POST /{id}/images/pull", requireWrite(imageHandler.Pull))
	imagesMux.Handle("POST /{id}/images/prune", requireWrite(imageHandler.Prune))
	// Detail DTO (list fields + inspect config), read-level like the
	// raw inspect route below.
	imagesMux.HandleFunc("GET /{id}/images/{imageId}", imageHandler.Get)
	imagesMux.Handle("DELETE /{id}/images/{imageId}", requireWrite(imageHandler.Delete))
	imagesMux.HandleFunc("GET /{id}/images/{imageId}/inspect", imageHandler.Inspect)
	// The archive download mutates nothing, so it stays read-level
	// like the volume export (issue 401): users who can read a
	// machine's state can read its data.
	imagesMux.HandleFunc("GET /{id}/images/{imageId}/export", imageHandler.Export)
	// Copy preview stays read-level: it mutates nothing on the source,
	// and the target machine's read access is enforced inside the
	// service against the access store (issue 375).
	imagesMux.HandleFunc("GET /{id}/images/{imageId}/copy-preview", imageHandler.CopyPreview)
	// The copy mutates remote state (it loads on the target machine),
	// so it gates at write level on the source; the target machine's
	// write access is enforced inside the service against the access
	// store — route middleware only sees the route's machine (issue
	// 375).
	imagesMux.Handle("POST /{id}/images/{imageId}/copy", requireWrite(imageHandler.Copy))
	imagesMux.Handle("/", apiNotFound)
	machineMux.Handle("/{id}/images", imagesMux)
	machineMux.Handle("/{id}/images/", imagesMux)

	// Quadlets.
	quadletsMux := http.NewServeMux()
	quadletsMux.HandleFunc("GET /{id}/quadlets", quadletHandler.List)
	quadletsMux.Handle("POST /{id}/quadlets", requireWrite(quadletHandler.Create))
	quadletsMux.Handle("POST /{id}/quadlets/import", requireWrite(quadletHandler.Import))
	quadletsMux.Handle("POST /{id}/quadlets/auto-update", requireWrite(quadletHandler.AutoUpdate))
	quadletsMux.HandleFunc("GET /{id}/quadlets/{quadletId}", quadletHandler.Get)
	quadletsMux.Handle("PUT /{id}/quadlets/{quadletId}", requireWrite(quadletHandler.Update))
	quadletsMux.Handle("DELETE /{id}/quadlets/{quadletId}", requireWrite(quadletHandler.Delete))
	quadletsMux.Handle("POST /{id}/quadlets/{quadletId}/start", requireWrite(quadletHandler.Start))
	quadletsMux.Handle("POST /{id}/quadlets/{quadletId}/stop", requireWrite(quadletHandler.Stop))
	quadletsMux.Handle("POST /{id}/quadlets/{quadletId}/restart", requireWrite(quadletHandler.Restart))
	quadletsMux.HandleFunc("GET /{id}/quadlets/{quadletId}/logs", quadletHandler.GetLogs)
	quadletsMux.HandleFunc("POST /{id}/quadlets/{quadletId}/generate", quadletHandler.Generate)
	// Move/copy preview stays read-level: it mutates nothing on the
	// source, and the target machine's read access is enforced inside
	// the service against the access store (issue 209).
	quadletsMux.HandleFunc("GET /{id}/quadlets/{quadletId}/move-preview", quadletHandler.MovePreview)
	// The transfer mutates remote state (it installs on the target
	// machine), so it gates at write level on the source; the target
	// machine's write access is enforced inside the service against
	// the access store — route middleware only sees the route's
	// machine (issue 210).
	quadletsMux.Handle("POST /{id}/quadlets/{quadletId}/move", requireWrite(quadletHandler.Move))
	quadletsMux.Handle("/", apiNotFound)
	machineMux.Handle("/{id}/quadlets", quadletsMux)
	machineMux.Handle("/{id}/quadlets/", quadletsMux)

	// Secrets — read-level subtree: secret METADATA (list + inspect)
	// is visible to any grantee (subtree membership is the read
	// gate). The value reveal is NOT a route-level gate: ?reveal=true
	// is a query param, so Service.Get checks the caller's level
	// itself and answers 403 ACCESS_DENIED below admin. Creating and
	// deleting secrets are remote-state mutations, so both are gated
	// at write (160-03/160-04).
	secretsMux := http.NewServeMux()
	secretsMux.HandleFunc("GET /{id}/secrets", secretHandler.List)
	secretsMux.Handle("POST /{id}/secrets", requireWrite(secretHandler.Create))
	secretsMux.HandleFunc("GET /{id}/secrets/{secretId}", secretHandler.Get)
	secretsMux.HandleFunc("GET /{id}/secrets/{secretId}/inspect", secretHandler.Inspect)
	// Move/copy preview stays read-level: it mutates nothing and
	// reveals nothing on the source, and the target machine's read
	// access is enforced inside the service against the access store
	// (issue 239).
	secretsMux.HandleFunc("GET /{id}/secrets/{secretId}/move-preview", secretHandler.MovePreview)
	// The transfer mutates remote state (it creates on the target
	// machine) and the value leaving the source is a reveal, so the
	// service gates the source at admin on top of this write gate; the
	// target machine's write access is enforced inside the service
	// against the access store — route middleware only sees the
	// route's machine (issue 239).
	secretsMux.Handle("POST /{id}/secrets/{secretId}/move", requireWrite(secretHandler.Move))
	secretsMux.Handle("DELETE /{id}/secrets/{secretId}", requireWrite(secretHandler.Delete))
	secretsMux.Handle("/", apiNotFound)
	machineMux.Handle("/{id}/secrets", secretsMux)
	machineMux.Handle("/{id}/secrets/", secretsMux)

	// Volumes — list/inspect stay read-level; create (and later
	// slices) mutate remote state, so they gate at write (200-03). The
	// literal /prune route wins over the {volumeName} subtree by
	// ServeMux precedence, so prune calls cannot be swallowed as a
	// volume named "prune".
	volumesMux := http.NewServeMux()
	volumesMux.HandleFunc("GET /{id}/volumes", volumeHandler.List)
	volumesMux.Handle("POST /{id}/volumes", requireWrite(volumeHandler.Create))
	volumesMux.Handle("POST /{id}/volumes/prune", requireWrite(volumeHandler.Prune))
	volumesMux.HandleFunc("GET /{id}/volumes/{volumeName}", volumeHandler.Get)
	// Export stays read-level: users who can read a machine's state
	// can read its data.
	volumesMux.HandleFunc("GET /{id}/volumes/{volumeName}/export", volumeHandler.Export)
	// Move/copy preview stays read-level: it mutates nothing on the
	// source, and the target machine's read access is enforced inside
	// the service against the access store (issue 238).
	volumesMux.HandleFunc("GET /{id}/volumes/{volumeName}/move-preview", volumeHandler.MovePreview)
	volumesMux.Handle("POST /{id}/volumes/{volumeName}/populate", requireWrite(volumeHandler.Populate))
	// The transfer mutates remote state (it creates on the target
	// machine), so it gates at write level on the source; the target
	// machine's write access is enforced inside the service against
	// the access store — route middleware only sees the route's
	// machine (issue 238).
	volumesMux.Handle("POST /{id}/volumes/{volumeName}/move", requireWrite(volumeHandler.Move))
	volumesMux.Handle("DELETE /{id}/volumes/{volumeName}", requireWrite(volumeHandler.Delete))
	volumesMux.Handle("/", apiNotFound)
	machineMux.Handle("/{id}/volumes", volumesMux)
	machineMux.Handle("/{id}/volumes/", volumesMux)

	// Networks — the list and the detail endpoint stay at the
	// subtree's read level; create, connect, and prune mutate remote
	// state, so they gate at write (issue 252; prune spec #385). The
	// prune route is literal so it is not shadowed by the
	// {networkName} subtree.
	networksMux := http.NewServeMux()
	networksMux.HandleFunc("GET /{id}/networks", networkHandler.List)
	networksMux.Handle("POST /{id}/networks/prune", requireWrite(networkHandler.Prune))
	networksMux.HandleFunc("GET /{id}/networks/{networkName}", networkHandler.Get)
	networksMux.HandleFunc("GET /{id}/networks/{networkName}/inspect", networkHandler.Inspect)
	networksMux.Handle("POST /{id}/networks", requireWrite(networkHandler.Create))
	networksMux.Handle("POST /{id}/networks/{networkName}/connect", requireWrite(networkHandler.Connect))
	networksMux.Handle("/", apiNotFound)
	machineMux.Handle("/{id}/networks", networksMux)
	machineMux.Handle("/{id}/networks/", networksMux)

	// Pods — the list, detail, raw inspect, and logs endpoints stay at
	// the subtree's read level; create, prune, lifecycle actions, and
	// remove mutate remote state, so they gate at write (spec #353).
	// The prune route is literal so it is not shadowed by the {podId}
	// subtree.
	podsMux := http.NewServeMux()
	podsMux.HandleFunc("GET /{id}/pods", podHandler.List)
	podsMux.Handle("POST /{id}/pods", requireWrite(podHandler.Create))
	podsMux.Handle("POST /{id}/pods/prune", requireWrite(podHandler.Prune))
	podsMux.HandleFunc("GET /{id}/pods/{podId}", podHandler.Get)
	podsMux.HandleFunc("GET /{id}/pods/{podId}/inspect", podHandler.Inspect)
	podsMux.HandleFunc("GET /{id}/pods/{podId}/logs", podHandler.Logs)
	podsMux.Handle("POST /{id}/pods/{podId}/pause", requireWrite(podHandler.Pause))
	podsMux.Handle("POST /{id}/pods/{podId}/unpause", requireWrite(podHandler.Unpause))
	podsMux.Handle("POST /{id}/pods/{podId}/kill", requireWrite(podHandler.Kill))
	podsMux.Handle("POST /{id}/pods/{podId}/restart", requireWrite(podHandler.Restart))
	podsMux.Handle("POST /{id}/pods/{podId}/start", requireWrite(podHandler.Start))
	podsMux.Handle("POST /{id}/pods/{podId}/stop", requireWrite(podHandler.Stop))
	podsMux.Handle("DELETE /{id}/pods/{podId}", requireWrite(podHandler.Remove))
	podsMux.Handle("/", apiNotFound)
	machineMux.Handle("/{id}/pods", podsMux)
	machineMux.Handle("/{id}/pods/", podsMux)

	// The machine subtree's JSON-404 catch-all: unknown paths and
	// unregistered method+path combinations inside the subtree answer
	// the envelope, never the SPA shell.
	machineMux.Handle("/", apiNotFound)

	// The API mux. http.StripPrefix at the root hands it paths without
	// the /api prefix; its own patterns are method-qualified.
	apiMux := http.NewServeMux()

	// Auth. Login, refresh, the anonymous status probe, and first-run
	// setup are the unauthenticated entry points of the auth surface,
	// so they share an IP-keyed rate-limit group — brute-force and
	// enumeration protection that needs no user to key on. The group
	// is not prefix-shaped, so the limiter wraps each handler at
	// registration. Logout stays outside the group: it is idempotent
	// and revokes only the cookie it is handed, so there is nothing to
	// brute-force.
	authRateLimit := func(h http.HandlerFunc) http.Handler {
		return appmiddleware.AuthRateLimitMiddleware(authRateLimiter)(h)
	}
	apiMux.Handle("POST /auth/login", authRateLimit(authHandler.Login))
	// Refresh is deliberately public: it authenticates with the
	// httpOnly refresh cookie instead of a Bearer JWT — presenting
	// that cookie to a JWT-gated route would be a contradiction (the
	// whole point is minting a new JWT). Rotation lives inside
	// AuthService.Refresh.
	apiMux.Handle("POST /auth/refresh", authRateLimit(authHandler.Refresh))
	// Status serves anonymous callers too (authenticated: false, plus
	// whether a fresh instance still needs setup), so it validates the
	// token only when one is presented — and rides the same per-IP
	// group: the needs-setup flag is exactly what a fresh-instance
	// enumerator wants.
	apiMux.Handle("GET /auth/status", appmiddleware.AuthRateLimitMiddleware(authRateLimiter)(
		appmiddleware.OptionalJWTMiddleware(authService)(http.HandlerFunc(authHandler.Status))))
	// First-run setup is the bootstrap entry point (issue 179): on a
	// user-less instance it creates the first admin and signs the
	// caller in (login payload + refresh cookie). The atomic
	// zero-user guard lives in the creation service; a configured
	// instance answers 409 SETUP_COMPLETED with no side effects.
	apiMux.Handle("POST /setup/user", authRateLimit(authHandler.Setup))
	apiMux.HandleFunc("POST /auth/logout", authHandler.Logout)
	// Password change is strictly authenticated (the JWT middleware
	// is the guarantor; authority comes from the current password in
	// the body) and lives under /api/auth so the fresh refresh cookie
	// it hands back shares the login/refresh cookie scope. All of the
	// caller's other sessions die in AuthService.ChangePassword.
	apiMux.Handle("POST /auth/password", appmiddleware.JWTMiddleware(authService)(http.HandlerFunc(authHandler.ChangePassword)))

	// Protected group: every route below demands a JWT and rides the
	// per-user rate limiter. The group is not prefix-shaped (machines,
	// the compose conversion, and users sit side by side), so the
	// chain wraps each mount point.
	jwtAuth := appmiddleware.JWTMiddleware(authService)
	perUserLimit := appmiddleware.RateLimitMiddleware(rateLimiter)
	protected := func(h http.HandlerFunc) http.Handler {
		return jwtAuth(perUserLimit(h))
	}
	protectedAdmin := func(h http.HandlerFunc) http.Handler {
		return jwtAuth(perUserLimit(appmiddleware.RequireAdmin(userRepo)(h)))
	}

	// Machines — Create/List sit outside the RequireMachineAccess
	// boundary (user-scoped, no machine yet); the {id} subtree is the
	// machine sub-mux behind it.
	apiMux.Handle("GET /machines", protected(machineHandler.List))
	apiMux.Handle("POST /machines", protected(machineHandler.Create))
	// The {id} subtree mount: the "/machines/{id}" exact pattern and
	// its "/{id}/" subtree twin both hand the machine sub-mux to the
	// access middleware — and nothing else does, so "/machines/"
	// (the slashed form) never reaches RequireMachineAccess with an
	// empty id: it falls to the API catch-all's JSON 404. StripPrefix
	// drops the /machines literal on the way in, so the sub-mux's
	// patterns are rooted at {id}.
	machineSubtree := jwtAuth(perUserLimit(appmiddleware.RequireMachineAccess(machineRepo, accessRepo)(
		http.StripPrefix("/machines", machineMux))))
	apiMux.Handle("/machines/{id}", machineSubtree)
	apiMux.Handle("/machines/{id}/", machineSubtree)

	// Quadlets — the compose conversion endpoint is the first
	// quadlets route OUTSIDE the machine subtree (spec #336):
	// conversion touches no machine data, so any authenticated user
	// may call it — no RequireMachineAccess, no write gate. It sits
	// beside /machines and /users.
	apiMux.Handle("POST /quadlets/convert-compose", protected(quadletHandler.ConvertCompose))

	// Users — the list endpoint is available to any authenticated
	// caller; the response shape depends on the caller's role (shaped
	// in users.UserService.List). Creating users, changing roles, and
	// resetting passwords stay admin-only (090-01, 162, 165), the role
	// change with the self-lockout guard and the password reset with
	// revoke-all-sessions in the service. All sit beside /machines,
	// not inside it.
	apiMux.Handle("GET /users", protected(userHandler.List))
	apiMux.Handle("POST /users", protectedAdmin(userHandler.Create))
	apiMux.Handle("PUT /users/{userId}/role", protectedAdmin(userHandler.UpdateRole))
	apiMux.Handle("PUT /users/{userId}/password", protectedAdmin(userHandler.ResetPassword))

	// Health check endpoint.
	apiMux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("OK"))
	})

	// The API's JSON-404 catch-all: unknown /api paths answer the
	// envelope at this depth too.
	apiMux.Handle("/", apiNotFound)

	// The root mux: the API subtree strips /api; the SPA — the
	// frontend build embedded by internal/webui — rides the "/"
	// catch-all AFTER /api, so every API route (and its 404s) stays
	// API-shaped while all other paths fall back to index.html,
	// mirroring the Caddy container's try_files contract this
	// replaces.
	root := http.NewServeMux()
	root.Handle("/api/", http.StripPrefix("/api", apiMux))
	root.Handle("/", webui.Handler())

	// Global middleware chain, outermost first — the same order the
	// chi router's Use calls established. Security headers live in the
	// global chain, not in the SPA handler or per handler: the pentest
	// finding (2026-11-01, finding 6) was that NO response carried
	// them, /api/health included, so one owner here covers API + SPA
	// + error and streaming responses. HSTS is config-gated
	// (HSTS_MAX_AGE_SECONDS, default 0 = omitted — the backend serves
	// plain HTTP). Request logging lives here, not per handler: one
	// line per request (method, path, status, duration, requestID)
	// covering every endpoint, streaming included (long-lived streams
	// log at stream end). Runs inside RequestIDMiddleware so the ID is
	// in context. Recoverer (ported off chi's, issue #451) is the
	// outermost: a panic anywhere below answers the JSON 500 envelope.
	global := []func(http.Handler) http.Handler{
		appmiddleware.Recoverer,
		appmiddleware.RequestIDMiddleware,
		appmiddleware.CORS(cfg.AllowedOrigins),
		appmiddleware.SecurityHeaders(cfg.HSTSMaxAgeSeconds),
		appmiddleware.RequestLogging,
	}
	var router http.Handler = root
	for i := len(global) - 1; i >= 0; i-- {
		router = global[i](router)
	}

	return &App{
		db:                db,
		connectionManager: connectionManager,
		sshExecService:    sshExecService,
		userSvc:           userService,
		router:            router,
	}, nil
}

// Handler returns the configured router. The returned handler is the
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
