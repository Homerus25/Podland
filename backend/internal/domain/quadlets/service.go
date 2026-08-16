package quadlets

import (
	"archive/tar"
	"bytes"
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/yourorg/podland/backend/internal/domain/machines"
	appErrors "github.com/yourorg/podland/backend/internal/errors"
	"github.com/yourorg/podland/backend/internal/sshexec"
	"go.podman.io/podman/v6/pkg/bindings/quadlets"
	"go.podman.io/podman/v6/pkg/domain/entities"
	"go.podman.io/podman/v6/pkg/domain/infra/tunnel"
)

// Compile-time check that *QuadletService satisfies QuadletServiceInterface.
var _ QuadletServiceInterface = (*QuadletService)(nil)

// QuadletServiceInterface defines the interface for quadlet services.
// This interface is used for dependency injection and testing.
type QuadletServiceInterface interface {
	List(ctx context.Context, userID, machineID string, filters ListFilters) ([]QuadletListResponse, error)
	Get(ctx context.Context, userID, machineID, name string) (*QuadletGetResponse, error)
	Create(ctx context.Context, userID, machineID string, req QuadletCreateRequest) (*QuadletCreateResponse, error)
	Update(ctx context.Context, userID, machineID, name string, req QuadletUpdateRequest) (*QuadletGetResponse, error)
	Delete(ctx context.Context, userID, machineID, name string, force, stop bool) error
	Start(ctx context.Context, userID, machineID, name string) (*QuadletLifecycleResponse, error)
	Stop(ctx context.Context, userID, machineID, name string, opts LifecycleOptions) (*QuadletLifecycleResponse, error)
	Restart(ctx context.Context, userID, machineID, name string, opts LifecycleOptions) (*QuadletLifecycleResponse, error)
	GetLogs(ctx context.Context, userID, machineID, quadletName string, since time.Time, tail int64) (*QuadletLogsResponse, error)
	GetLogsWithStream(ctx context.Context, userID, machineID, quadletName string, since time.Time, tail int64, logCallback func(event QuadletLogEvent)) error
	Generate(ctx context.Context, userID, machineID string, req QuadletGenerateRequest) (*QuadletGenerateResponse, error)
	Import(ctx context.Context, userID, machineID string, req QuadletImportRequest) (*QuadletImportResponse, error)
}

// ListFilters represents the filters for listing quadlets.
type ListFilters struct {
	Resource string
	Name     string
}

// QuadletService provides business logic for quadlets
type QuadletService struct {
	connManager machines.ConnectionManagerInterface
	// sshExec runs systemctl over SSH for the lifecycle (Start/Stop/Restart)
	// operations. It is a SEPARATE dependency from connManager (which is a
	// podman-socket tunnel only) per the 070-10 SSH-exec infra slice.
	sshExec sshexec.SSHExecInterface
}

// NewQuadletService creates a new QuadletService with the given connection
// manager interface and SSH-exec interface. The SSH-exec dependency is required
// by the lifecycle endpoints (Start/Stop/Restart) which run systemctl over SSH;
// the other operations (List/Get/Create/Update/Delete) use only the connection
// manager and are unaffected by it. Tests that do not exercise lifecycle may
// pass nil for sshExec.
func NewQuadletService(connManager machines.ConnectionManagerInterface, sshExec sshexec.SSHExecInterface) *QuadletService {
	return &QuadletService{
		connManager: connManager,
		sshExec:     sshExec,
	}
}

// List returns a list of quadlets on the specified machine.
func (s *QuadletService) List(ctx context.Context, userID, machineID string, filters ListFilters) ([]QuadletListResponse, error) {
	connCtx, err := s.connManager.GetConnection(machineID, userID)
	if err != nil {
		return nil, appErrors.ErrConnectionFailed
	}

	defer s.connManager.Release(machineID)
	containerEngine := &tunnel.ContainerEngine{
		ClientCtx: connCtx,
	}

	quadlets, err := containerEngine.QuadletList(connCtx, entities.QuadletListOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to list quadlets: %w", err)
	}

	return applyFilters(ToQuadletListResponse(quadlets), filters), nil
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
	connCtx, err := s.connManager.GetConnection(machineID, userID)
	if err != nil {
		return nil, appErrors.ErrConnectionFailed
	}

	defer s.connManager.Release(machineID)
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
	connCtx, err := s.connManager.GetConnection(machineID, userID)
	if err != nil {
		return nil, appErrors.ErrConnectionFailed
	}

	defer s.connManager.Release(machineID)
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
		installedPath = firstInstalledPath(report.InstalledQuadlets)
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

// Update replaces an existing quadlet's content on the specified machine.
//
// Podman has no dedicated update primitive, so the service composes
// QuadletRemove + QuadletInstall (with Replace: true) of the new content
// (doku/description_api.md:939-961):
//
//  1. Existence check: locate the quadlet via QuadletList. A missing quadlet
//     maps to ErrQuadletNotFound (404). The list result also supplies the
//     resource extension used to name the rebuilt tar entry.
//  2. Remove the existing quadlet (Force: true so an active managed resource
//     does not block the replace; the PUT contract exposes no running-conflict
//     error, so an update is allowed even while the resource is up).
//  3. Install the new content as an in-memory tar (single file named
//     `<name>.<resource-extension>`), with Replace: true and ReloadSystemd: true.
//
// IMPORTANT: this flow is NOT atomic across the remove/install pair. A failure
// after the remove succeeds but before the install completes leaves the quadlet
// removed. The returned error reflects the install failure, but the caller
// cannot assume the original quadlet is still present.
//
// The response reuses the Get shape (doku/description_api.md:953). State is
// reported as "unknown" immediately after the reinstall because systemd has not
// yet reloaded to report a fresh status — matching the Create slice's behaviour.
func (s *QuadletService) Update(ctx context.Context, userID, machineID, name string, req QuadletUpdateRequest) (*QuadletGetResponse, error) {
	connCtx, err := s.connManager.GetConnection(machineID, userID)
	if err != nil {
		return nil, appErrors.ErrConnectionFailed
	}

	defer s.connManager.Release(machineID)
	containerEngine := &tunnel.ContainerEngine{
		ClientCtx: connCtx,
	}

	// 1. Existence check + resource derivation. A missing quadlet is a 404.
	list, err := containerEngine.QuadletList(connCtx, entities.QuadletListOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to list quadlets: %w", err)
	}

	target := findQuadletByName(list, name)
	if target == nil {
		return nil, appErrors.ErrQuadletNotFound
	}

	// 2. Remove the existing quadlet. Force: true so an active managed resource
	//    does not block the replace (the PUT contract has no running-conflict
	//    error). ReloadSystemd: true so systemd drops the old unit.
	if _, err := containerEngine.QuadletRemove(connCtx, []string{name}, entities.QuadletRemoveOptions{
		Force:         true,
		ReloadSystemd: true,
	}); err != nil {
		if isNotFoundError(err) {
			return nil, appErrors.ErrQuadletNotFound
		}
		return nil, fmt.Errorf("failed to remove existing quadlet: %w", err)
	}

	// 3. Install the new content. The filename preserves the original resource
	//    extension so the rebuilt quadlet keeps its type. Replace: true overwrites
	//    any leftover file from a partial state; ReloadSystemd: true picks up the
	//    change.
	//    The derive→extension round-trip re-normalizes the contract enum (so a
	//    .kube/.build path rebuilds as .container/.image), matching Create's use
	//    of resourceToExtension on the request resource.
	//
	//    NOTE: The `name` parameter includes the extension (e.g. "my-quadlet.container"),
	//    but we need to strip it before adding the new extension to avoid doubling it.
	resource := deriveResourceFromPath(target.Path)
	// Strip the existing extension from the name to avoid doubling
	ext := resourceToExtension(resource)
	baseName := strings.TrimSuffix(name, ext)
	fileName := baseName + ext
	body, err := createTarBody(fileName, []byte(req.Content))
	if err != nil {
		return nil, fmt.Errorf("failed to build quadlet tar body: %w", err)
	}

	options := new(quadlets.QuadletInstallOptions).WithReplace(true).WithReloadSystemd(true)
	report, err := quadlets.Install(connCtx, options, body)
	if err != nil {
		return nil, fmt.Errorf("failed to install updated quadlet: %w", err)
	}

	installedPath := ""
	if report != nil {
		installedPath = firstInstalledPath(report.InstalledQuadlets)
	}

	return buildUpdateResponse(name, target.UnitName, resource, installedPath, req.Content), nil
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

	connCtx, err := s.connManager.GetConnection(machineID, userID)
	if err != nil {
		return appErrors.ErrConnectionFailed
	}

	defer s.connManager.Release(machineID)

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

// firstInstalledPath returns the destination path of the first installed
// quadlet reported by QuadletInstall, or the empty string when none is present.
// The install report's InstalledQuadlets is a path→path map whose ordering is
// non-deterministic, so "first" is arbitrary — callers only need *a* populated
// path to echo back, mirroring the install primitive's own reporting.
func firstInstalledPath(installed map[string]string) string {
	for _, dest := range installed {
		return dest
	}
	return ""
}

// buildUpdateResponse assembles the Get-shaped response for a freshly updated
// quadlet. State is reported as "unknown" because systemd has not yet reloaded
// to report a fresh status (matching the Create slice's behaviour). Extracted as
// a pure function so the success-shape mapping is unit-testable independently of
// the unmockable engine boundary (see the service-seam note in service_test.go).
func buildUpdateResponse(name, unit, resource, installedPath, content string) *QuadletGetResponse {
	return &QuadletGetResponse{
		ID:       name,
		Name:     name,
		Path:     installedPath,
		Unit:     unit,
		Resource: resource,
		State:    "unknown",
		Content:  content,
	}
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

// GetLogs retrieves logs for the managed container of a quadlet.
// Per the issue spec, this resolves the quadlet name → managed container name
// and then streams logs via ContainerEngine.ContainerLogs, mirroring the
// 060-04 image-pull NDJSON pattern. The non-streaming mode returns a single
// multiline string; the streaming mode emits NDJSON events via callback.
//
// The flow (doku/description_api.md:1072-1109):
//
//  1. Resolve the quadlet via QuadletList — existence check (404 if absent)
//     and managed container name resolution (prefer the Pod container name,
//     fall back to deriving it from the quadlet name).
//  2. Call ContainerEngine.ContainerLogs with Follow=false, concatenating
//     stdout and stderr into a single multiline string.
//  3. Map "no such container" → QUADLET_NOT_FOUND (404).
//
// Returns `{id, logs}` on success. Supports `since` and `tail` query params.
func (s *QuadletService) GetLogs(_ context.Context, userID, machineID, quadletName string, since time.Time, tail int64) (*QuadletLogsResponse, error) {
	connCtx, err := s.connManager.GetConnection(machineID, userID)
	if err != nil {
		return nil, appErrors.ErrConnectionFailed
	}
	defer s.connManager.Release(machineID)

	containerEngine := &tunnel.ContainerEngine{
		ClientCtx: connCtx,
	}

	containerName, err := s.resolveForLogs(containerEngine, userID, machineID, quadletName)
	println("containername: " + containerName)
	if err != nil {
		return nil, err
	}
	defer s.connManager.Release(machineID)

	var stdoutBuffer, stderrBuffer bytes.Buffer
	logsOpts := entities.ContainerLogsOptions{
		Follow:       false,
		Since:        since,
		Tail:         tail,
		Timestamps:   false,
		StdoutWriter: &stdoutBuffer,
		StderrWriter: &stderrBuffer,
	}

	err = containerEngine.ContainerLogs(connCtx, []string{containerName}, logsOpts)
	if err != nil {
		if strings.Contains(err.Error(), "no such container") || strings.Contains(err.Error(), "container not found") {
			return nil, appErrors.ErrQuadletNotFound
		}
		return nil, fmt.Errorf("failed to get quadlet logs: %w", err)
	}

	combinedLogs := strings.TrimSuffix(stdoutBuffer.String(), "\n")
	if stderrBuffer.Len() > 0 {
		stderrContent := strings.TrimSuffix(stderrBuffer.String(), "\n")
		if combinedLogs != "" {
			combinedLogs += "\n" + stderrContent
		} else {
			combinedLogs = stderrContent
		}
	}

	return &QuadletLogsResponse{
		ID:   quadletName,
		Logs: combinedLogs,
	}, nil
}

// GetLogsWithStream retrieves logs for the managed container of a quadlet
// with streaming callback. When follow=true, this method streams log lines
// one at a time via the logCallback function. Mirrors the 060-04 image-pull
// NDJSON pattern.
//
// The flow mirrors GetLogs but uses Follow=true and a callback writer that
// converts each log line to a QuadletLogEvent. Client-disconnect is detected
// via context cancellation and mapped to OPERATION_CANCELLED.
//
// Supports `since` and `tail` query params. Emits NDJSON events via callback.
func (s *QuadletService) GetLogsWithStream(_ context.Context, userID, machineID, quadletName string, since time.Time, tail int64, logCallback func(event QuadletLogEvent)) error {
	connCtx, err := s.connManager.GetConnection(machineID, userID)
	if err != nil {
		return appErrors.ErrConnectionFailed
	}
	defer s.connManager.Release(machineID)

	containerEngine := &tunnel.ContainerEngine{
		ClientCtx: connCtx,
	}

	containerName, err := s.resolveForLogs(containerEngine, userID, machineID, quadletName)
	if err != nil {
		return err
	}

	logsOpts := entities.ContainerLogsOptions{
		Follow:     true,
		Since:      since,
		Tail:       tail,
		Timestamps: true,
	}

	stdoutWriter := &quadletLogEventWriter{
		callback:   logCallback,
		level:      "INFO",
		timestamps: true,
	}
	stderrWriter := &quadletLogEventWriter{
		callback:   logCallback,
		level:      "ERROR",
		timestamps: true,
	}

	logsOpts.StdoutWriter = stdoutWriter
	logsOpts.StderrWriter = stderrWriter

	err = containerEngine.ContainerLogs(connCtx, []string{containerName}, logsOpts)
	if err != nil {
		if strings.Contains(err.Error(), "no such container") || strings.Contains(err.Error(), "container not found") {
			return appErrors.ErrQuadletNotFound
		}
		if connCtx.Err() == context.Canceled {
			return appErrors.ErrOperationCancelled
		}
		return fmt.Errorf("failed to get quadlet logs: %w", err)
	}

	return nil
}

// resolveForLogs performs the shared pre-work for GetLogs and GetLogsWithStream:
// it locates the quadlet (404 if absent) and resolves the managed container name.
// For container-type quadlets, the managed container name is the quadlet name;
// for pod-type quadlets, we use the Pod container name from the ListQuadlet result.
// Returns the connection context and resolved container name to avoid redundant connection acquisition.
func (s *QuadletService) resolveForLogs(containerEngine *tunnel.ContainerEngine, userID, machineID, quadletName string) (string, error) {
	target, err := s.resolveTarget(containerEngine, quadletName)
	if err != nil {
		return "", err
	}
	content, err := containerEngine.QuadletPrint(context.Background(), quadletName)
	if err == nil {
		if name := explicitContainerNameFromQuadletContent(content); name != "" {
			return name, nil
		}
	}

	return resolveManagedContainerName(target, quadletName), nil
}

// quadletLogEventWriter wraps podman's log writers and converts each line to a QuadletLogEvent
type quadletLogEventWriter struct {
	callback   func(event QuadletLogEvent)
	level      string
	timestamps bool
}

// Write implements io.Writer to capture log lines from podman
func (w *quadletLogEventWriter) Write(data []byte) (int, error) {
	line := string(data)
	line = strings.TrimSuffix(line, "\n")
	if line == "" {
		return len(data), nil
	}

	timestamp := time.Now().UTC().Format(time.RFC3339)
	message := line

	// Parse podman timestamp if enabled
	if w.timestamps {
		// Try to parse the timestamp if it looks like it's at the start
		if strings.Contains(line, " ") {
			parts := strings.SplitN(line, " ", 2)
			if len(parts) == 2 {
				parsedTime, err := time.Parse(time.RFC3339Nano, parts[0])
				if err == nil {
					timestamp = parsedTime.UTC().Format(time.RFC3339)
					message = parts[1]
				}
			}
		}
	}

	// Emit log event via callback
	w.callback(QuadletLogEvent{
		Timestamp: timestamp,
		Level:     w.level,
		Message:   message,
	})

	return len(data), nil
}

// resourceType represents a resource type that can be converted to a quadlet.
// Each resource type knows its own inspection logic, name extraction,
// and conversion logic. This eliminates primitive obsession on resource type
// strings and consolidates switch-case logic into type methods, following
// the lifecycleAction pattern from lifecycle.go.
type resourceType string

const (
	resourceTypeContainer resourceType = "container"
	resourceTypePod       resourceType = "pod"
)

func (rt resourceType) inspectAndConvert(ctx context.Context, engine *tunnel.ContainerEngine, resourceID string) (string, string, error) {
	switch rt {
	case resourceTypeContainer:
		containerReports, _, err := engine.ContainerInspect(ctx, []string{resourceID}, entities.InspectOptions{})
		if err != nil {
			return "", "", appErrors.ErrContainerNotFound
		}
		if len(containerReports) == 0 {
			return "", "", appErrors.ErrContainerNotFound
		}

		inspectReport := containerReports[0]
		quadletName := inspectReport.Name
		if quadletName == "" {
			quadletName = resourceID
		}

		content, err := generateContainerQuadlet(containerReports[0])
		if err != nil {
			return "", "", appErrors.ErrQuadletGenerationFailed
		}
		return quadletName, content, nil

	case resourceTypePod:
		podReports, _, err := engine.PodInspect(ctx, []string{resourceID}, entities.InspectOptions{})
		if err != nil {
			return "", "", appErrors.ErrPodNotFound
		}
		if len(podReports) == 0 {
			return "", "", appErrors.ErrPodNotFound
		}

		inspectReport := podReports[0]
		quadletName := inspectReport.Name
		if quadletName == "" {
			quadletName = resourceID
		}

		content, err := generatePodQuadlet(podReports[0])
		if err != nil {
			return "", "", appErrors.ErrQuadletGenerationFailed
		}
		return quadletName, content, nil

	default:
		return "", "", appErrors.ErrInvalidRequest
	}
}

// Generate generates a quadlet file from an existing container or pod on the
// remote machine. It inspects the source resource via the podman API and
// converts it to quadlet format using the pkg/systemd/quadlet converter.
func (s *QuadletService) Generate(ctx context.Context, userID, machineID string, req QuadletGenerateRequest) (*QuadletGenerateResponse, error) {
	connCtx, err := s.connManager.GetConnection(machineID, userID)
	if err != nil {
		return nil, appErrors.ErrConnectionFailed
	}
	defer s.connManager.Release(machineID)

	containerEngine := &tunnel.ContainerEngine{
		ClientCtx: connCtx,
	}

	rt := resourceType(req.ResourceType)
	quadletName, content, err := rt.inspectAndConvert(connCtx, containerEngine, req.ResourceID)
	if err != nil {
		return nil, err
	}

	return &QuadletGenerateResponse{
		ID:       quadletName,
		Name:     quadletName,
		Resource: req.ResourceType,
		State:    "unknown",
		Content:  content,
	}, nil
}

// generateContainerQuadlet converts a container inspect report to quadlet format.
// This implements the conversion logic from podman's pkg/systemd/quadlet package.
func generateContainerQuadlet(inspect *entities.ContainerInspectReport) (string, error) {
	if inspect == nil || inspect.Config == nil {
		return "", fmt.Errorf("invalid container inspect data")
	}

	var builder strings.Builder

	quadletName := inspect.Name
	if quadletName == "" {
		return "", fmt.Errorf("container name is required")
	}

	builder.WriteString(fmt.Sprintf("# Generated by Podland from container %s\n", quadletName))
	builder.WriteString(fmt.Sprintf("[Unit]\n"))
	builder.WriteString(fmt.Sprintf("Description=Podman container %s\n\n", quadletName))

	builder.WriteString(fmt.Sprintf("[Container]\n"))
	builder.WriteString(fmt.Sprintf("ContainerName=%s\n", quadletName))

	if inspect.Config.Image != "" {
		builder.WriteString(fmt.Sprintf("Image=%s\n", inspect.Config.Image))
	}

	if len(inspect.Config.Env) > 0 {
		for _, env := range inspect.Config.Env {
			builder.WriteString(fmt.Sprintf("Environment=%s\n", env))
		}
	}

	if inspect.HostConfig != nil {
		if inspect.HostConfig.RestartPolicy != nil {
			policy := inspect.HostConfig.RestartPolicy.Name
			if policy != "" && policy != "no" {
				builder.WriteString(fmt.Sprintf("RestartPolicy=%s\n", policy))
			}
		}

		if len(inspect.HostConfig.PortBindings) > 0 {
			builder.WriteString("PublishPort=")
			var ports []string
			for containerPort, bindings := range inspect.HostConfig.PortBindings {
				for _, binding := range bindings {
					ports = append(ports, fmt.Sprintf("%s:%s", binding.HostPort, containerPort))
				}
			}
			builder.WriteString(strings.Join(ports, ","))
			builder.WriteString("\n")
		}

		if len(inspect.HostConfig.Binds) > 0 {
			for _, bind := range inspect.HostConfig.Binds {
				builder.WriteString(fmt.Sprintf("Volume=%s\n", bind))
			}
		}

		if len(inspect.HostConfig.NetworkMode) > 0 {
			builder.WriteString(fmt.Sprintf("Network=%s\n", inspect.HostConfig.NetworkMode))
		}
	}

	if inspect.Config.Labels != nil {
		for key, value := range inspect.Config.Labels {
			builder.WriteString(fmt.Sprintf("Label=%s=%s\n", key, value))
		}
	}

	return builder.String(), nil
}

// generatePodQuadlet converts a pod inspect report to quadlet format.
// This implements the conversion logic from podman's pkg/systemd/quadlet package.
func generatePodQuadlet(inspect *entities.PodInspectReport) (string, error) {
	if inspect == nil {
		return "", fmt.Errorf("invalid pod inspect data")
	}

	var builder strings.Builder

	podName := inspect.Name
	if podName == "" {
		return "", fmt.Errorf("pod name is required")
	}

	builder.WriteString(fmt.Sprintf("# Generated by Podland from pod %s\n", podName))
	builder.WriteString(fmt.Sprintf("[Unit]\n"))
	builder.WriteString(fmt.Sprintf("Description=Podman pod %s\n\n", podName))

	builder.WriteString(fmt.Sprintf("[Pod]\n"))
	builder.WriteString(fmt.Sprintf("PodName=%s\n", podName))

	if inspect.CreateCommand != nil && len(inspect.CreateCommand) > 0 {
		builder.WriteString("PodmanArgs=")
		builder.WriteString(strings.Join(inspect.CreateCommand, " "))
		builder.WriteString("\n")
	}

	if len(inspect.Labels) > 0 {
		for key, value := range inspect.Labels {
			builder.WriteString(fmt.Sprintf("Label=%s=%s\n", key, value))
		}
	}

	return builder.String(), nil
}

// Import installs multiple quadlets on the specified machine from uploaded content.
// Each entry in the request is wrapped as an in-memory tar and passed to podman's
// QuadletInstall primitive. The method handles partial failures by collecting
// successful and failed imports separately.
func (s *QuadletService) Import(ctx context.Context, userID, machineID string, req QuadletImportRequest) (*QuadletImportResponse, error) {
	connCtx, err := s.connManager.GetConnection(machineID, userID)
	if err != nil {
		return nil, appErrors.ErrConnectionFailed
	}

	defer s.connManager.Release(machineID)

	response := &QuadletImportResponse{
		Imported: make([]QuadletImportResult, 0),
		Failed:   make([]QuadletImportError, 0),
	}

	for _, entry := range req.Quadlets {
		importResult, err := s.importSingleQuadlet(connCtx, entry)
		if err != nil {
			response.Failed = append(response.Failed, QuadletImportError{
				Name:  entry.Name,
				Error: err.Error(),
			})
		} else {
			response.Imported = append(response.Imported, importResult)
		}
	}

	return response, nil
}

// importSingleQuadlet imports a single quadlet entry using QuadletInstall.
// Returns the import result or an error explaining why the import failed.
func (s *QuadletService) importSingleQuadlet(connCtx context.Context, entry QuadletImportEntry) (QuadletImportResult, error) {
	fileName := entry.Name + resourceToExtension(entry.Resource)
	body, err := createTarBody(fileName, []byte(entry.Content))
	if err != nil {
		return QuadletImportResult{}, fmt.Errorf("failed to build quadlet tar body: %w", err)
	}

	options := new(quadlets.QuadletInstallOptions).WithReloadSystemd(true)

	report, err := quadlets.Install(connCtx, options, body)
	if err != nil {
		if isAlreadyExistsError(err) {
			return QuadletImportResult{}, fmt.Errorf("quadlet already exists")
		}
		return QuadletImportResult{}, fmt.Errorf("failed to install quadlet: %w", err)
	}

	if report != nil {
		for _, qErr := range report.QuadletErrors {
			if qErr != nil && isAlreadyExistsError(qErr) {
				return QuadletImportResult{}, fmt.Errorf("quadlet already exists")
			}
		}
	}

	installedPath := ""
	if report != nil {
		installedPath = firstInstalledPath(report.InstalledQuadlets)
	}

	return QuadletImportResult{
		ID:    entry.Name,
		Name:  entry.Name,
		Path:  installedPath,
		State: "unknown",
	}, nil
}
