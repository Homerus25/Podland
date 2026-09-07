package volumes

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/Homerus25/podland/backend/internal/connections"
	appErrors "github.com/Homerus25/podland/backend/internal/errors"
	bindingsVolumes "go.podman.io/podman/v6/pkg/bindings/volumes"
	"go.podman.io/podman/v6/pkg/domain/entities"
	"go.podman.io/podman/v6/pkg/domain/entities/reports"
	"go.podman.io/podman/v6/pkg/domain/infra/tunnel"
	errorhandling "go.podman.io/podman/v6/pkg/errorhandling"
)

// VolumeServiceInterface defines the interface for volume services
// This interface is used for dependency injection and testing
type VolumeServiceInterface interface {
	List(ctx context.Context, userID, machineID string, opts entities.VolumeListOptions) ([]*entities.VolumeListReport, error)
	Get(ctx context.Context, userID, machineID, volumeName string) (*entities.VolumeInspectReport, error)
	Create(ctx context.Context, userID, machineID string, req VolumeCreateRequest) (*entities.VolumeInspectReport, error)
	Delete(ctx context.Context, userID, machineID, volumeName string, force bool) error
	Prune(ctx context.Context, userID, machineID string) ([]*reports.PruneReport, error)
	Export(ctx context.Context, userID, machineID, volumeName string, w io.Writer) error
	Import(ctx context.Context, userID, machineID, volumeName string, r io.Reader) error
}

// VolumeService provides business logic for volumes
type VolumeService struct {
	conn connections.Manager
}

// NewVolumeService creates a new VolumeService
func NewVolumeService(conn connections.Manager) *VolumeService {
	return &VolumeService{
		conn: conn,
	}
}

// List returns every volume on the specified machine, named and anonymous
func (s *VolumeService) List(ctx context.Context, userID, machineID string, opts entities.VolumeListOptions) ([]*entities.VolumeListReport, error) {
	var volumes []*entities.VolumeListReport
	err := s.conn.WithConnection(ctx, userID, machineID, "volumes.List", func(connCtx context.Context) error {
		volumeEngine := &tunnel.ContainerEngine{
			ClientCtx: connCtx,
		}

		v, err := volumeEngine.VolumeList(ctx, opts)
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
	err := s.conn.WithConnection(ctx, userID, machineID, "volumes.Get", func(connCtx context.Context) error {
		volumeEngine := &tunnel.ContainerEngine{
			ClientCtx: connCtx,
		}

		r, err := s.inspectVolume(ctx, volumeEngine, volumeName)
		if err != nil {
			return err
		}
		report = r
		return nil
	})
	return report, err
}

// inspectVolume resolves a single volume by exact name through the
// tunnel engine, mapping a missing volume to the clean 404 sentinel.
func (s *VolumeService) inspectVolume(ctx context.Context, volumeEngine *tunnel.ContainerEngine, volumeName string) (*entities.VolumeInspectReport, error) {
	reports, errs, err := volumeEngine.VolumeInspect(ctx, []string{volumeName}, entities.InspectOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to inspect volume: %w", err)
	}
	for _, itemErr := range errs {
		if itemErr != nil {
			if isVolumeNotFound(itemErr) {
				return nil, appErrors.ErrVolumeNotFound
			}
			return nil, fmt.Errorf("failed to inspect volume: %w", itemErr)
		}
	}
	if len(reports) == 0 || reports[0] == nil {
		return nil, appErrors.ErrVolumeNotFound
	}
	return reports[0], nil
}

// Create makes a volume on the machine and returns its full config.
// An empty request name yields an anonymous volume; the returned config
// carries podman's generated name. Podman-side rejections (invalid name
// format, driver errors, already-existing name) surface as 400
// INVALID_REQUEST carrying podman's message, not 5xx.
func (s *VolumeService) Create(ctx context.Context, userID, machineID string, req VolumeCreateRequest) (*entities.VolumeInspectReport, error) {
	var report *entities.VolumeInspectReport
	err := s.conn.WithConnection(ctx, userID, machineID, "volumes.Create", func(connCtx context.Context) error {
		volumeEngine := &tunnel.ContainerEngine{
			ClientCtx: connCtx,
		}

		created, err := volumeEngine.VolumeCreate(ctx, ToPodmanVolumeCreateOptions(req))
		if err != nil {
			if appErr := mapCreateError(err); appErr != nil {
				return appErr
			}
			return fmt.Errorf("failed to create volume: %w", err)
		}

		// Create only reports the name; the contract is the full config, so
		// inspect the freshly created volume by its (possibly generated) name.
		inspect, err := s.inspectVolume(ctx, volumeEngine, created.IDOrName)
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
	return s.conn.WithConnection(ctx, userID, machineID, "volumes.Delete", func(connCtx context.Context) error {
		volumeEngine := &tunnel.ContainerEngine{
			ClientCtx: connCtx,
		}

		reports, err := volumeEngine.VolumeRm(ctx, []string{volumeName}, entities.VolumeRmOptions{Force: force})
		if err != nil {
			return fmt.Errorf("failed to remove volume: %w", err)
		}
		for _, report := range reports {
			if report == nil || report.Err == nil {
				continue
			}
			if isVolumeNotFound(report.Err) {
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
// or a plain "in use" error depending on version; both map here.
func mapInUseError(err error) *appErrors.AppError {
	msg := err.Error()
	var errModel *errorhandling.ErrorModel
	if errors.As(err, &errModel) {
		msg = errModel.Message
		if errModel.ResponseCode == http.StatusConflict {
			return appErrors.ErrVolumeInUse.WithMessage(msg)
		}
	}
	if strings.Contains(strings.ToLower(msg), "in use") {
		return appErrors.ErrVolumeInUse.WithMessage(msg)
	}
	return nil
}

// mapCreateError converts podman's client-side rejections into 4xx app
// errors. A 4xx ErrorModel from the podman service means the request
// itself was bad (unknown driver/option, name in use). Invalid NAME
// format is special: podman's compat layer rejects it as a 500
// ErrorModel (or a plain "invalid argument" error), so detection also
// rides on podman's validation text; everything else stays a wrapped 5xx.
func mapCreateError(err error) *appErrors.AppError {
	msg := err.Error()
	var errModel *errorhandling.ErrorModel
	if errors.As(err, &errModel) {
		msg = errModel.Message
		if errModel.ResponseCode >= 400 && errModel.ResponseCode < 500 {
			return appErrors.ErrInvalidRequest.WithMessage(msg)
		}
	}
	if strings.Contains(msg, "invalid argument") || strings.Contains(msg, "names must match") {
		return appErrors.ErrInvalidRequest.WithMessage(msg)
	}
	return nil
}

// isVolumeNotFound reports whether an item error from tunnel
// VolumeInspect means the volume does not exist. The tunnel layer
// converts a 404 ErrorModel from the podman service into a plain
// "no such volume %q" error (pkg/domain/infra/tunnel/volumes.go), so
// detection rides on that text; podman ErrorModels that reach us
// directly are still recognized via ResponseCode == 404.
func isVolumeNotFound(err error) bool {
	if strings.Contains(strings.ToLower(err.Error()), "no such volume") {
		return true
	}
	var errModel *errorhandling.ErrorModel
	return errors.As(err, &errModel) && errModel.ResponseCode == http.StatusNotFound
}

// Prune removes every unused volume on the machine in one call and
// returns podman's per-volume reports. Volumes in use by containers are
// untouched — podman's own semantics, which is the blast-radius
// mitigation for this endpoint.
func (s *VolumeService) Prune(ctx context.Context, userID, machineID string) ([]*reports.PruneReport, error) {
	var prunes []*reports.PruneReport
	err := s.conn.WithConnection(ctx, userID, machineID, "volumes.Prune", func(connCtx context.Context) error {
		volumeEngine := &tunnel.ContainerEngine{
			ClientCtx: connCtx,
		}

		p, err := volumeEngine.VolumePrune(ctx, entities.VolumePruneOptions{})
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
	return s.conn.WithConnection(ctx, userID, machineID, "volumes.Export", func(connCtx context.Context) error {
		volumeEngine := &tunnel.ContainerEngine{
			ClientCtx: connCtx,
		}

		// Existence gate: fail before writing anything to w.
		if _, err := s.inspectVolume(ctx, volumeEngine, volumeName); err != nil {
			return err
		}

		return bindingsVolumes.Export(connCtx, volumeName, w)
	})
}

// Import streams a tar archive into the volume. Semantics are
// podman-native MERGE: paths in the archive overwrite same-named files;
// everything else in the volume is kept. There is no replace mode. The
// volume's existence is resolved first, so a missing name fails with
// the clean 404 before the archive is consumed.
func (s *VolumeService) Import(ctx context.Context, userID, machineID, volumeName string, r io.Reader) error {
	return s.conn.WithConnection(ctx, userID, machineID, "volumes.Import", func(connCtx context.Context) error {
		volumeEngine := &tunnel.ContainerEngine{
			ClientCtx: connCtx,
		}

		// Existence gate: fail before consuming the archive.
		if _, err := s.inspectVolume(ctx, volumeEngine, volumeName); err != nil {
			return err
		}

		return bindingsVolumes.Import(connCtx, volumeName, r)
	})
}
