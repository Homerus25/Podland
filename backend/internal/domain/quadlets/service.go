package quadlets

import (
	"archive/tar"
	"bytes"
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/yourorg/podland/backend/internal/connections"
	"github.com/yourorg/podland/backend/internal/domain/machines"
	appErrors "github.com/yourorg/podland/backend/internal/errors"
	"go.podman.io/podman/v6/pkg/bindings/quadlets"
	"go.podman.io/podman/v6/pkg/domain/entities"
	"go.podman.io/podman/v6/pkg/domain/infra/tunnel"
)

// Compile-time check that *QuadletService satisfies QuadletServiceInterface.
var _ QuadletServiceInterface = (*QuadletService)(nil)

// concreteConnectionManager wraps a *connections.ConnectionManager to implement ConnectionManagerInterface.
// This is needed for backward compatibility.
type concreteConnectionManager struct {
	connManager *connections.ConnectionManager
}

// GetConnection implements ConnectionManagerInterface by calling the underlying ConnectionManager.
func (c *concreteConnectionManager) GetConnection(ctx context.Context, machineID, userID string) (any, error) {
	return c.connManager.GetConnection(ctx, machineID, userID)
}

func (c *concreteConnectionManager) Release(machineID string) {
	c.connManager.Release(machineID)
}

// QuadletServiceInterface defines the interface for quadlet services.
// This interface is used for dependency injection and testing.
type QuadletServiceInterface interface {
	List(ctx context.Context, userID, machineID string, filters ListFilters) ([]QuadletListResponse, error)
	Get(ctx context.Context, userID, machineID, name string) (*QuadletGetResponse, error)
	Create(ctx context.Context, userID, machineID string, req QuadletCreateRequest) (*QuadletCreateResponse, error)
	Delete(ctx context.Context, userID, machineID, name string, force, stop bool) error
}

// ListFilters represents the filters for listing quadlets.
type ListFilters struct {
	Resource string
	Name     string
}

// QuadletService provides business logic for quadlets
type QuadletService struct {
	connManager machines.ConnectionManagerInterface
}

// NewQuadletService creates a new QuadletService with the given connection manager interface.
func NewQuadletService(connManager machines.ConnectionManagerInterface) *QuadletService {
	return &QuadletService{
		connManager: connManager,
	}
}

// NewQuadletServiceWithConcreteDeps creates a new QuadletService with concrete dependencies.
// This maintains backward compatibility with existing code that uses *connections.ConnectionManager.
func NewQuadletServiceWithConcreteDeps(connManager *connections.ConnectionManager) *QuadletService {
	return NewQuadletService(&concreteConnectionManager{connManager: connManager})
}

// List returns a list of quadlets on the specified machine.
func (s *QuadletService) List(ctx context.Context, userID, machineID string, filters ListFilters) ([]QuadletListResponse, error) {
	connCtxInterface, err := s.connManager.GetConnection(context.Background(), machineID, userID)
	if err != nil {
		return nil, appErrors.ErrConnectionFailed
	}

	defer s.connManager.Release(machineID)

	connCtx, ok := connCtxInterface.(context.Context)
	if !ok {
		return nil, fmt.Errorf("invalid connection type: expected context.Context")
	}

	containerEngine := &tunnel.ContainerEngine{
		ClientCtx: connCtx,
	}

	quadlets, err := containerEngine.QuadletList(connCtx, entities.QuadletListOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to list quadlets: %w", err)
	}

	result := ToQuadletListResponse(quadlets)

	result = applyFilters(result, filters)

	return result, nil
}

// applyFilters applies the resource and name filters to the quadlet list.
func applyFilters(quadlets []QuadletListResponse, filters ListFilters) []QuadletListResponse {
	if filters.Resource == "" && filters.Name == "" {
		return quadlets
	}

	var filtered []QuadletListResponse
	for _, quadlet := range quadlets {
		if filters.Resource != "" && quadlet.Resource != filters.Resource {
			continue
		}
		if filters.Name != "" && !strings.Contains(strings.ToLower(quadlet.Name), strings.ToLower(filters.Name)) {
			continue
		}
		filtered = append(filtered, quadlet)
	}

	return filtered
}

// Get returns a specific quadlet by name on the specified machine.
func (s *QuadletService) Get(ctx context.Context, userID, machineID, name string) (*QuadletGetResponse, error) {
	connCtxInterface, err := s.connManager.GetConnection(context.Background(), machineID, userID)
	if err != nil {
		return nil, appErrors.ErrConnectionFailed
	}

	defer s.connManager.Release(machineID)

	connCtx, ok := connCtxInterface.(context.Context)
	if !ok {
		return nil, fmt.Errorf("invalid connection type: expected context.Context")
	}

	containerEngine := &tunnel.ContainerEngine{
		ClientCtx: connCtx,
	}

	content, err := containerEngine.QuadletPrint(connCtx, name)
	if err != nil {
		return nil, fmt.Errorf("failed to get quadlet content: %w", err)
	}

	quadlets, err := containerEngine.QuadletList(connCtx, entities.QuadletListOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to list quadlets: %w", err)
	}

	targetQuadlet := findQuadletByName(quadlets, name)

	if targetQuadlet == nil {
		return nil, appErrors.ErrQuadletNotFound
	}

	listResponse := FromListQuadlet(targetQuadlet)
	return &QuadletGetResponse{
		ID:       listResponse.ID,
		Name:     listResponse.Name,
		Path:     listResponse.Path,
		Unit:     listResponse.Unit,
		Resource: listResponse.Resource,
		State:    listResponse.State,
		Content:  content,
	}, nil
}

// Create installs a new quadlet on the specified machine.
// The request content is wrapped as an in-memory tar (single file named
// `<name>.<resource-extension>`) and passed to the podman QuadletInstall
// primitive with ReloadSystemd enabled so systemd picks up the new unit.
// A name collision in the install report is mapped to ErrQuadletAlreadyExists.
func (s *QuadletService) Create(ctx context.Context, userID, machineID string, req QuadletCreateRequest) (*QuadletCreateResponse, error) {
	connCtxInterface, err := s.connManager.GetConnection(context.Background(), machineID, userID)
	if err != nil {
		return nil, appErrors.ErrConnectionFailed
	}

	defer s.connManager.Release(machineID)

	connCtx, ok := connCtxInterface.(context.Context)
	if !ok {
		return nil, fmt.Errorf("invalid connection type: expected context.Context")
	}

	// Build the in-memory tar: a single file named `<name>.<resource-extension>`.
	// Podman's QuadletInstall primitive accepts a tar body, not raw content —
	// see ~/repos/podman/pkg/domain/infra/tunnel/quadlet.go:161 (createTarFromReader).
	fileName := req.Name + resourceToExtension(req.Resource)
	body, err := createTarBody(fileName, []byte(req.Content))
	if err != nil {
		return nil, fmt.Errorf("failed to build quadlet tar body: %w", err)
	}

	// Call the podman bindings-level Install directly. The engine-level
	// QuadletInstall only accepts on-disk paths / URLs; we already hold the
	// content in memory, so we skip its path-resolution step.
	options := new(quadlets.QuadletInstallOptions).WithReloadSystemd(true)

	report, err := quadlets.Install(connCtx, options, body)
	if err != nil {
		if isAlreadyExistsError(err) {
			return nil, appErrors.ErrQuadletAlreadyExists
		}
		return nil, fmt.Errorf("failed to install quadlet: %w", err)
	}

	// A conflict surfaces as an entry in QuadletErrors keyed by source path.
	if report != nil {
		for _, qErr := range report.QuadletErrors {
			if qErr != nil && isAlreadyExistsError(qErr) {
				return nil, appErrors.ErrQuadletAlreadyExists
			}
		}
	}

	// Resolve the installed path if podman reported one back.
	installedPath := ""
	if report != nil {
		for _, dest := range report.InstalledQuadlets {
			installedPath = dest
			break
		}
	}

	return &QuadletCreateResponse{
		ID:       req.Name,
		Name:     req.Name,
		Path:     installedPath,
		Resource: strings.ToLower(req.Resource),
		State:    "unknown",
		Content:  req.Content,
	}, nil
}

// Delete removes a quadlet by name on the specified machine.
//
// The flow honours the contract's `force` and `stop` query parameters
// (doku/description_api.md:965-984):
//
//   - The quadlet is located via a QuadletList pre-check. A missing quadlet
//     maps to ErrQuadletNotFound (404).
//   - If the managed resource is currently running and force is not set, the
//     delete is refused with ErrQuadletResourceRunning (400).
//   - Otherwise QuadletRemove is called with Force=force and ReloadSystemd=true
//     so systemd picks up the removal.
//
// `stop` (default true) would normally trigger a systemctl stop over SSH before
// remove when the resource is running; the SSH-exec infra lands in slice
// 070-10. Until then stop is accepted and the running-resource guarantee above
// is enforced via the pre-check so a running quadlet is never silently removed
// without force.
func (s *QuadletService) Delete(ctx context.Context, userID, machineID, name string, force, stop bool) error {
	_ = stop // honoured once SSH-exec (070-10) is available; see doc comment.

	connCtxInterface, err := s.connManager.GetConnection(context.Background(), machineID, userID)
	if err != nil {
		return appErrors.ErrConnectionFailed
	}

	defer s.connManager.Release(machineID)

	connCtx, ok := connCtxInterface.(context.Context)
	if !ok {
		return fmt.Errorf("invalid connection type: expected context.Context")
	}

	containerEngine := &tunnel.ContainerEngine{
		ClientCtx: connCtx,
	}

	// Pre-check: locate the quadlet and evaluate the running guard so we map
	// not-found / running-conflict before attempting the destructive call.
	list, err := containerEngine.QuadletList(connCtx, entities.QuadletListOptions{})
	if err != nil {
		return fmt.Errorf("failed to list quadlets: %w", err)
	}

	if err := checkDeletePreconditions(findQuadletByName(list, name), force); err != nil {
		return err
	}

	report, err := containerEngine.QuadletRemove(connCtx, []string{name}, entities.QuadletRemoveOptions{
		Force:         force,
		ReloadSystemd: true,
	})
	if err != nil {
		if isNotFoundError(err) {
			return appErrors.ErrQuadletNotFound
		}
		if !force && isResourceRunningError(err) {
			return appErrors.ErrQuadletResourceRunning
		}
		return fmt.Errorf("failed to remove quadlet: %w", err)
	}

	// A removal failure (e.g. a race where the quadlet vanished or its
	// resource started between the pre-check and the remove) surfaces in the
	// report's Errors map keyed by quadlet name.
	if report != nil {
		if qErr, ok := report.Errors[name]; ok && qErr != nil {
			if isNotFoundError(qErr) {
				return appErrors.ErrQuadletNotFound
			}
			if !force && isResourceRunningError(qErr) {
				return appErrors.ErrQuadletResourceRunning
			}
			return fmt.Errorf("failed to remove quadlet: %w", qErr)
		}
	}

	return nil
}

// createTarBody builds an in-memory tar containing a single file with the given
// name and content. Mirrors the framing of podman's createTarFromReader
// (~/repos/podman/pkg/domain/infra/tunnel/quadlet.go:161) but operates on an
// in-memory byte slice instead of a pipe + goroutine. Writes target an
// in-memory bytes.Buffer and so should not fail in practice, but errors are
// propagated rather than swallowed.
func createTarBody(fileName string, content []byte) (io.Reader, error) {
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	hdr := &tar.Header{
		Name:    fileName,
		Mode:    0o644,
		Size:    int64(len(content)),
		ModTime: time.Now(),
	}
	if err := tw.WriteHeader(hdr); err != nil {
		return nil, fmt.Errorf("failed to write tar header: %w", err)
	}
	if _, err := tw.Write(content); err != nil {
		return nil, fmt.Errorf("failed to write tar content: %w", err)
	}
	if err := tw.Close(); err != nil {
		return nil, fmt.Errorf("failed to close tar: %w", err)
	}
	return &buf, nil
}

// isAlreadyExistsError heuristically detects a quadlet name-collision error.
// Podman surfaces install conflicts via QuadletErrors rather than a typed
// not-found/exists error, so we match on the "already exists" phrase from the
// error message (per the issue spec).
func isAlreadyExistsError(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(strings.ToLower(err.Error()), "already exists")
}

// findQuadletByName returns the first ListQuadlet in the slice whose Name
// matches, or nil if none match. Nil entries in the slice are skipped.
func findQuadletByName(quadlets []*entities.ListQuadlet, name string) *entities.ListQuadlet {
	for _, q := range quadlets {
		if q == nil {
			continue
		}
		if q.Name == name {
			return q
		}
	}
	return nil
}

// checkDeletePreconditions evaluates the not-found and running-resource guards
// for a delete operation against the target quadlet's parsed state. It returns
// ErrQuadletNotFound when the target is absent, ErrQuadletResourceRunning when
// the managed resource is running and force is not set, or nil when the delete
// may proceed.
func checkDeletePreconditions(target *entities.ListQuadlet, force bool) error {
	if target == nil {
		return appErrors.ErrQuadletNotFound
	}
	if parseStateFromStatus(target.Status) == "running" && !force {
		return appErrors.ErrQuadletResourceRunning
	}
	return nil
}

// isNotFoundError heuristically detects a "quadlet missing" error from a
// QuadletRemove report entry or return value. Podman surfaces removal failures
// of absent quadlets via the Errors map rather than a typed error, so we match
// on common "no such" / "not found" phrases.
func isNotFoundError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "not found") || strings.Contains(msg, "no such")
}

// isResourceRunningError heuristically detects a "resource in use / running"
// error from a QuadletRemove call. Per the issue's technical note, podman may
// return a typed "in use" error on Force=false (e.g. when a quadlet-managed
// container started between our pre-check and the remove). Such an error maps
// to ErrQuadletResourceRunning (400) rather than a generic 500.
func isResourceRunningError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "in use") ||
		strings.Contains(msg, "running") ||
		strings.Contains(msg, "active")
}
