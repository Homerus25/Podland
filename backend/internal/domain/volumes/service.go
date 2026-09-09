package volumes

import (
	"context"
	"fmt"
	"io"

	appErrors "github.com/Homerus25/podland/backend/internal/errors"
	"github.com/Homerus25/podland/backend/internal/podmanerr"
	"go.podman.io/podman/v6/pkg/domain/entities"
	"go.podman.io/podman/v6/pkg/domain/entities/reports"
)

// VolumeService provides business logic for volumes. Every machine-scoped
// podman call crosses the VolumeEngineProvider (the connection seam); the
// service itself owns only domain mapping and composition.
type VolumeService struct {
	engines VolumeEngineProvider
}

// NewVolumeService creates a new VolumeService
func NewVolumeService(engines VolumeEngineProvider) *VolumeService {
	return &VolumeService{
		engines: engines,
	}
}

// List returns every volume on the specified machine, named and anonymous
func (s *VolumeService) List(ctx context.Context, userID, machineID string, opts entities.VolumeListOptions) ([]*entities.VolumeListReport, error) {
	var volumes []*entities.VolumeListReport
	err := s.engines.WithEngine(ctx, userID, machineID, "volumes.List", func(e VolumeEngine) error {
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
func (s *VolumeService) Get(ctx context.Context, userID, machineID, volumeName string) (*entities.VolumeInspectReport, error) {
	var report *entities.VolumeInspectReport
	err := s.engines.WithEngine(ctx, userID, machineID, "volumes.Get", func(e VolumeEngine) error {
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

// Create makes a volume on the machine and returns its full config.
// An empty request name yields an anonymous volume; the returned config
// carries podman's generated name. Podman-side rejections (invalid name
// format, driver errors, already-existing name) surface as 400
// INVALID_REQUEST carrying podman's message, not 5xx.
func (s *VolumeService) Create(ctx context.Context, userID, machineID string, req VolumeCreateRequest) (*entities.VolumeInspectReport, error) {
	var report *entities.VolumeInspectReport
	err := s.engines.WithEngine(ctx, userID, machineID, "volumes.Create", func(e VolumeEngine) error {
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
func (s *VolumeService) Delete(ctx context.Context, userID, machineID, volumeName string, force bool) error {
	return s.engines.WithEngine(ctx, userID, machineID, "volumes.Delete", func(e VolumeEngine) error {
		removed, err := e.Remove(volumeName, force)
		if err != nil {
			return fmt.Errorf("failed to remove volume: %w", err)
		}
		for _, report := range removed {
			if report == nil || report.Err == nil {
				continue
			}
			if podmanerr.IsNotFound(report.Err) {
				return appErrors.ErrVolumeNotFound
			}
			if appErr := mapInUseError(report.Err); appErr != nil {
				return appErr
			}
			return fmt.Errorf("failed to remove volume: %w", report.Err)
		}
		return nil
	})
}

// mapInUseError converts podman's in-use refusal into 409 VOLUME_IN_USE
// carrying podman's message, so the UI can explain what depends on the
// volume. Podman's compat layer reports the refusal as a 409 ErrorModel
// or a plain "in use" error depending on version; the adapter owns both
// shapes (podmanerr.IsConflict).
func mapInUseError(err error) *appErrors.AppError {
	if podmanerr.IsConflict(err) {
		return appErrors.ErrVolumeInUse.WithMessage(podmanerr.Message(err))
	}
	return nil
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

// Prune removes every unused volume on the machine in one call and
// returns podman's per-volume reports. Volumes in use by containers are
// untouched — podman's own semantics, which is the blast-radius
// mitigation for this endpoint.
func (s *VolumeService) Prune(ctx context.Context, userID, machineID string) ([]*reports.PruneReport, error) {
	var prunes []*reports.PruneReport
	err := s.engines.WithEngine(ctx, userID, machineID, "volumes.Prune", func(e VolumeEngine) error {
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
func (s *VolumeService) Export(ctx context.Context, userID, machineID, volumeName string, w io.Writer) error {
	return s.engines.WithEngine(ctx, userID, machineID, "volumes.Export", func(e VolumeEngine) error {
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
func (s *VolumeService) Import(ctx context.Context, userID, machineID, volumeName string, r io.Reader) error {
	return s.engines.WithEngine(ctx, userID, machineID, "volumes.Import", func(e VolumeEngine) error {
		// Existence gate: fail before consuming the archive.
		if _, err := s.inspectVolume(e, volumeName); err != nil {
			return err
		}

		return e.Import(volumeName, r)
	})
}
