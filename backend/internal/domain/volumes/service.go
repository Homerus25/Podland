package volumes

import (
	"context"
	"fmt"
	"io"

	appErrors "github.com/Homerus25/podland/backend/internal/errors"
	"github.com/Homerus25/podland/backend/internal/permission"
	"github.com/Homerus25/podland/backend/internal/podmanerr"
	"github.com/Homerus25/podland/backend/internal/removal"
	"github.com/Homerus25/podland/backend/internal/transfer"
	"go.podman.io/podman/v6/pkg/domain/entities"
	"go.podman.io/podman/v6/pkg/domain/entities/reports"
)

// VolumeService provides business logic for volumes. Every host-scoped
// podman call crosses the VolumeEngineProvider (the connection seam); the
// service itself owns only domain mapping and composition.
type VolumeService struct {
	engines VolumeEngineProvider
	// usageEngines resolves the in-use surface the cross-host copy/move
	// (issue 238) derives its facts from: container mounts and quadlet
	// files. Wired via WithUsageEngines; production wiring always sets it
	// — calling a move path on a service without it is a wiring bug and
	// panics at the seam.
	usageEngines VolumeUsageEngineProvider
	// accessRepo resolves the caller's permission level for the
	// cross-host copy/move (issue 238): route middleware only sees the
	// route's host, so the target host's access is checked here,
	// against the store, before any podman call. Wired via WithAccessRepo.
	accessRepo permission.PermissionGetter
	// maxVolumeUploadBytes bounds the copy/move's data stream (issue 238,
	// §19), enforced by the transfer kit's counting reader. Wired via
	// WithMaxVolumeUploadBytes from the same config value populate
	// uploads cap with; zero falls back to the transfer kit's
	// DefaultMaxVolumeUploadBytes (the kit owns the default per §19).
	maxVolumeUploadBytes int64
}

// NewVolumeService creates a new VolumeService
func NewVolumeService(engines VolumeEngineProvider) *VolumeService {
	return &VolumeService{
		engines: engines,
	}
}

// WithUsageEngines wires the in-use surface seam the cross-host
// copy/move (issue 238) reads and returns the service for chaining.
// Production wiring always sets it; calling a move path on a service
// without it is a wiring bug and panics at the seam.
func (s *VolumeService) WithUsageEngines(usageEngines VolumeUsageEngineProvider) *VolumeService {
	s.usageEngines = usageEngines
	return s
}

// WithAccessRepo wires the host-access store the cross-host
// copy/move (issue 238) gates the target host on, and returns the
// service for chaining. Production wiring always sets it; calling a
// move path on a service without it is a wiring bug and panics at the
// gate.
func (s *VolumeService) WithAccessRepo(accessRepo permission.PermissionGetter) *VolumeService {
	s.accessRepo = accessRepo
	return s
}

// WithMaxVolumeUploadBytes wires the copy/move data stream's bound
// (issue 238, §19) and returns the service for chaining. The bound is
// service-owned — the same VOLUME_UPLOAD_MAX_BYTES config value populate
// uploads cap with — so a transfer cannot stream past what the
// deployment already accepts for volume data.
func (s *VolumeService) WithMaxVolumeUploadBytes(max int64) *VolumeService {
	s.maxVolumeUploadBytes = max
	return s
}

// volumeDataLimit answers the wired transfer bound; the zero-value
// fallback resolves through the transfer kit's BoundOrDefault — the
// kit owns the default (§19), so this fallback and the quadlets
// domain's migration bound cannot drift apart.
func (s *VolumeService) volumeDataLimit() int64 {
	return transfer.BoundOrDefault(s.maxVolumeUploadBytes)
}

// List returns every volume on the specified host, named and anonymous
func (s *VolumeService) List(ctx context.Context, userID, hostID string, opts entities.VolumeListOptions) ([]*entities.VolumeListReport, error) {
	var volumes []*entities.VolumeListReport
	err := s.engines.WithEngine(ctx, userID, hostID, "volumes.List", func(e VolumeEngine) error {
		v, err := e.List(opts)
		if err != nil {
			return fmt.Errorf("failed to list volumes: %w", err)
		}
		volumes = v
		return nil
	})
	return volumes, err
}

// Get returns the full configuration of a single volume by exact name
func (s *VolumeService) Get(ctx context.Context, userID, hostID, volumeName string) (*entities.VolumeInspectReport, error) {
	var report *entities.VolumeInspectReport
	err := s.engines.WithEngine(ctx, userID, hostID, "volumes.Get", func(e VolumeEngine) error {
		r, err := s.inspectVolume(e, volumeName)
		if err != nil {
			return err
		}
		report = r
		return nil
	})
	return report, err
}

// inspectVolume resolves a single volume by exact name through the engine,
// mapping a missing volume to the clean 404 sentinel.
func (s *VolumeService) inspectVolume(e VolumeEngine, volumeName string) (*entities.VolumeInspectReport, error) {
	report, err := e.Inspect(volumeName)
	if err != nil {
		if podmanerr.IsNotFound(err) {
			return nil, appErrors.ErrVolumeNotFound
		}
		return nil, fmt.Errorf("failed to inspect volume: %w", err)
	}
	if report == nil {
		return nil, appErrors.ErrVolumeNotFound
	}
	return report, nil
}

// Create makes a volume on the host and returns its full config.
// An empty request name yields an anonymous volume; the returned config
// carries podman's generated name. Podman-side rejections (invalid name
// format, driver errors, already-existing name) surface as 400
// INVALID_REQUEST carrying podman's message, not 5xx.
func (s *VolumeService) Create(ctx context.Context, userID, hostID string, req VolumeCreateRequest) (*entities.VolumeInspectReport, error) {
	var report *entities.VolumeInspectReport
	err := s.engines.WithEngine(ctx, userID, hostID, "volumes.Create", func(e VolumeEngine) error {
		created, err := e.Create(ToPodmanVolumeCreateOptions(req))
		if err != nil {
			if appErr := mapCreateError(err); appErr != nil {
				return appErr
			}
			return fmt.Errorf("failed to create volume: %w", err)
		}

		// Create only reports the name; the contract is the full config, so
		// inspect the freshly created volume by its (possibly generated) name.
		inspect, err := s.inspectVolume(e, created.IDOrName)
		if err != nil {
			return err
		}
		report = inspect
		return nil
	})
	return report, err
}

// Delete removes a volume by exact name. Without force, podman refuses
// to remove a volume in use by a container — that refusal surfaces as
// 409 VOLUME_IN_USE carrying podman's message; with force, podman stops
// dependent containers first (its own semantics) and removes the volume.
// Unknown names surface as the same clean 404 as inspect.
//
// The classification covers the remove call's own failure too (issue
// 263's accepted tightening): a remove that errors at the call rather
// than in podman's per-volume report — the race window where the volume
// vanished or came into use between the request and podman's execution —
// answers the same terminals (404 VOLUME_NOT_FOUND / 409 VOLUME_IN_USE)
// instead of a wrapped unclassified error. The remove-and-classify
// ritual itself rides internal/removal (issue 264).
func (s *VolumeService) Delete(ctx context.Context, userID, hostID, volumeName string, force bool) error {
	return s.engines.WithEngine(ctx, userID, hostID, "volumes.Delete", func(e VolumeEngine) error {
		return removal.Run(func() error {
			return e.Remove(volumeName, force)
		}, removal.Terminals{
			Missing: appErrors.ErrVolumeNotFound,
			// Podman's compat layer reports the in-use refusal as a 409
			// ErrorModel or a plain "in use" error depending on version;
			// the predicate owns both shapes, and the builder threads
			// podman's message so the UI can explain what depends on the
			// volume.
			Refusal: func(cause string) error {
				return appErrors.ErrVolumeInUse.WithMessage(cause)
			},
			IsRefusal: podmanerr.IsConflict,
		}, "failed to remove volume")
	})
}

// mapCreateError converts podman's client-side rejections into 4xx app
// errors. Any podman-side 4xx ErrorModel means the request itself was bad
// (unknown driver/option, name already in use) — the composition preserves
// that contract across the adapter's predicates, since the adapter excludes
// the 404/409 answers from IsClientError. Podman's invalid NAME format is
// special: the compat layer rejects it as a 500 ErrorModel (or a plain
// "invalid argument" error), so the adapter's validation phrases catch it
// too; everything else stays a wrapped 5xx.
func mapCreateError(err error) *appErrors.AppError {
	if podmanerr.IsClientError(err) || podmanerr.IsConflict(err) || podmanerr.IsNotFound(err) {
		return appErrors.ErrInvalidRequest.WithMessage(podmanerr.Message(err))
	}
	return nil
}

// (tunnel VolumeInspect converts a 404 ErrorModel into a plain error and
// per-item reports stringify failures, so not-found classification is the
// adapter's job: podmanerr.IsNotFound.)

// Prune removes every unused volume on the host in one call and
// returns podman's per-volume reports. Volumes in use by containers are
// untouched — podman's own semantics, which is the blast-radius
// mitigation for this endpoint.
func (s *VolumeService) Prune(ctx context.Context, userID, hostID string) ([]*reports.PruneReport, error) {
	var prunes []*reports.PruneReport
	err := s.engines.WithEngine(ctx, userID, hostID, "volumes.Prune", func(e VolumeEngine) error {
		p, err := e.Prune()
		if err != nil {
			return fmt.Errorf("failed to prune volumes: %w", err)
		}
		prunes = p
		return nil
	})
	return prunes, err
}

// Export streams the volume's contents as a tar archive into w. The
// volume's existence is resolved first, so a missing name fails with the
// clean 404 before any byte is written — callers can still send error
// responses when this returns an AppError. Bytes flow straight from the
// remote podman into w with no server-side buffering.
func (s *VolumeService) Export(ctx context.Context, userID, hostID, volumeName string, w io.Writer) error {
	return s.engines.WithEngine(ctx, userID, hostID, "volumes.Export", func(e VolumeEngine) error {
		// Existence gate: fail before writing anything to w.
		if _, err := s.inspectVolume(e, volumeName); err != nil {
			return err
		}

		return e.Export(volumeName, w)
	})
}

// Import streams a tar archive into the volume. Semantics are
// podman-native MERGE: paths in the archive overwrite same-named files;
// everything else in the volume is kept. There is no replace mode. The
// volume's existence is resolved first, so a missing name fails with
// the clean 404 before the archive is consumed.
func (s *VolumeService) Import(ctx context.Context, userID, hostID, volumeName string, r io.Reader) error {
	return s.engines.WithEngine(ctx, userID, hostID, "volumes.Import", func(e VolumeEngine) error {
		// Existence gate: fail before consuming the archive.
		if _, err := s.inspectVolume(e, volumeName); err != nil {
			return err
		}

		return e.Import(volumeName, r)
	})
}
