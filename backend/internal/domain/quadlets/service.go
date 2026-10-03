package quadlets

import (
	"context"
	"fmt"
	"strings"
	"time"

	appErrors "github.com/Homerus25/podland/backend/internal/errors"
	"github.com/Homerus25/podland/backend/internal/models"
	"github.com/Homerus25/podland/backend/internal/permission"
	"github.com/Homerus25/podland/backend/internal/podmanerr"
	"github.com/Homerus25/podland/backend/internal/sshexec"
	"github.com/Homerus25/podland/backend/internal/streaming"
	"go.podman.io/podman/v6/pkg/domain/entities"
)

// machineLookup is the narrow subset of *models.MachineRepository that the
// quadlets service depends on: resolving a machine's registered SSH user
// decides the systemctl scope (`--user` for a non-root login, the system
// manager for root). Defining it locally keeps the constructor honest about
// what is actually needed and lets tests supply a fake, mirroring the
// machineRepo seam in internal/sshexec. *models.MachineRepository satisfies
// it implicitly.
type machineLookup interface {
	GetByID(id string) (*models.Machine, error)
}

// ListFilters represents the filters for listing quadlets.
type ListFilters struct {
	Resource string
	Name     string
}

// QuadletService provides business logic for quadlets. Every machine-scoped
// podman call crosses the QuadletEngineProvider (the connection seam); the
// service itself owns only domain mapping, composition, and the lifecycle's
// systemctl-via-SSH transport.
type QuadletService struct {
	engines QuadletEngineProvider
	// sshExec runs systemctl over SSH for the lifecycle (Start/Stop/Restart)
	// operations. It is a SEPARATE dependency from the engine provider (which
	// is a podman-socket seam only) per the 070-10 SSH-exec infra slice.
	sshExec sshexec.SSHExecInterface
	// machines resolves the machine's registered SSH user, which scopes the
	// systemctl invocations: a non-root login targets the user manager
	// (`systemctl --user`), root targets the system manager.
	machines machineLookup
	// accessRepo resolves the caller's permission level for the cross-machine
	// operations (move/copy preview and transfer): route middleware only sees
	// the route's machine, so the target machine's access is checked here,
	// against the store, before any podman call. Wired via WithAccessRepo.
	accessRepo permission.PermissionGetter
	// maxVolumeUploadBytes bounds the volume data migration's stream (issue
	// 214, §18), enforced by the data phase's counting reader. Wired via
	// WithMaxVolumeUploadBytes from the same config value the volumes
	// domain caps populate uploads with; zero falls back to
	// DefaultMaxVolumeUploadBytes.
	maxVolumeUploadBytes int64
	// secretEngines resolves the podman secret stores the move/copy's
	// secret surface crosses (issue 215): the existence reads for the
	// preview, the value reveal on the source and the create on the
	// target for the transfer. Wired via WithSecretEngines; production
	// wiring always sets it — calling a secret-touching path on a service
	// without it is a wiring bug and panics at the seam.
	secretEngines moveSecretEngineProvider
}

// NewQuadletService creates a new QuadletService with the given engine
// provider, SSH-exec interface, and machine lookup. The SSH-exec dependency
// is required by the lifecycle endpoints (Start/Stop/Restart) and the
// install paths' daemon-reload, which run systemctl over SSH; the machine
// lookup decides each invocation's scope from the registered SSH user.
// Tests that do not exercise those paths may pass nil for either.
func NewQuadletService(engines QuadletEngineProvider, sshExec sshexec.SSHExecInterface, machines machineLookup) *QuadletService {
	return &QuadletService{
		engines:  engines,
		sshExec:  sshExec,
		machines: machines,
	}
}

// WithAccessRepo wires the machine-access store the cross-machine
// operations (move/copy, issue 207) gate the target machine on, and
// returns the service for chaining. Production wiring always sets it;
// calling a cross-machine operation on a service without it is a wiring
// bug and panics at the gate.
func (s *QuadletService) WithAccessRepo(accessRepo permission.PermissionGetter) *QuadletService {
	s.accessRepo = accessRepo
	return s
}

// DefaultMaxVolumeUploadBytes is the volume data migration's bound when no
// explicit override is wired in: 2 GiB, matching VOLUME_UPLOAD_MAX_BYTES
// (the same env value the volumes domain's populate upload cap reads).
const DefaultMaxVolumeUploadBytes int64 = 2 << 30

// WithMaxVolumeUploadBytes wires the volume data migration's bound (issue
// 214, §18) and returns the service for chaining. The bound is
// service-owned — the same VOLUME_UPLOAD_MAX_BYTES config value the
// volumes domain caps populate uploads with — so the migration cannot
// stream past what the deployment already accepts for volume data.
func (s *QuadletService) WithMaxVolumeUploadBytes(max int64) *QuadletService {
	s.maxVolumeUploadBytes = max
	return s
}

// WithSecretEngines wires the podman secret-store seam the move/copy's
// secret surface crosses (issue 215) and returns the service for
// chaining. Production wiring always sets it; calling a secret-touching
// path (a preview of quadlet content with `Secret=` lines, or a transfer
// with transferSecrets set) on a service without it is a wiring bug and
// panics at the seam.
func (s *QuadletService) WithSecretEngines(secretEngines moveSecretEngineProvider) *QuadletService {
	s.secretEngines = secretEngines
	return s
}

// volumeDataLimit answers the wired migration bound; zero (unset in tests)
// falls back to DefaultMaxVolumeUploadBytes.
func (s *QuadletService) volumeDataLimit() int64 {
	if s.maxVolumeUploadBytes == 0 {
		return DefaultMaxVolumeUploadBytes
	}
	return s.maxVolumeUploadBytes
}

// List returns a list of quadlets on the specified machine.
func (s *QuadletService) List(ctx context.Context, userID, machineID string, filters ListFilters) ([]QuadletListResponse, error) {
	var quadlets []QuadletListResponse
	err := s.engines.WithEngine(ctx, userID, machineID, "quadlets.List", func(e QuadletEngine) error {
		list, err := e.List(entities.QuadletListOptions{})
		if err != nil {
			return fmt.Errorf("failed to list quadlets: %w", err)
		}

		quadlets = applyFilters(ToQuadletListResponse(list), filters)
		return nil
	})
	return quadlets, err
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
	var response *QuadletGetResponse
	err := s.engines.WithEngine(ctx, userID, machineID, "quadlets.Get", func(e QuadletEngine) error {
		content, err := e.Print(name)
		if err != nil {
			return fmt.Errorf("failed to get quadlet content: %w", err)
		}

		quadlets, err := e.List(entities.QuadletListOptions{})
		if err != nil {
			return fmt.Errorf("failed to list quadlets: %w", err)
		}

		targetQuadlet := findQuadletByName(quadlets, name)

		if targetQuadlet == nil {
			return appErrors.ErrQuadletNotFound
		}

		listResponse := FromListQuadlet(targetQuadlet)
		response = &QuadletGetResponse{
			ID:       listResponse.ID,
			Name:     listResponse.Name,
			Path:     listResponse.Path,
			Unit:     listResponse.Unit,
			Resource: listResponse.Resource,
			State:    listResponse.State,
			Content:  content,
		}
		return nil
	})
	return response, err
}

// Create installs a new quadlet on the specified machine.
// The request content is installed under the name `<name>.<resource-extension>`
// with systemd reload enabled so systemd picks up the new unit (the tar
// framing podman requires is the engine adapter's job). A name collision in
// the install report is mapped to ErrQuadletAlreadyExists.
//
// After a successful install the service also reloads the machine's
// systemd manager (reloadUserSystemd): podman's internal reload reaches the
// manager the podman service sees, not necessarily the one the lifecycle's
// scoped systemctl calls address. A reload failure fails the create with
// ErrConnectionFailed even though the file is installed — the quadlet would
// not be startable otherwise.
func (s *QuadletService) Create(ctx context.Context, userID, machineID string, req QuadletCreateRequest) (*QuadletCreateResponse, error) {
	var response *QuadletCreateResponse
	err := s.engines.WithEngine(ctx, userID, machineID, "quadlets.Create", func(e QuadletEngine) error {
		fileName := req.Name + resourceToExtension(req.Resource)

		report, err := e.Install(fileName, []byte(req.Content), false)
		if err != nil {
			if podmanerr.IsAlreadyExists(err) {
				return appErrors.ErrQuadletAlreadyExists
			}
			return fmt.Errorf("failed to install quadlet: %w", err)
		}

		// A conflict surfaces as an entry in QuadletErrors keyed by source path.
		if report != nil {
			for _, qErr := range report.QuadletErrors {
				if qErr != nil && podmanerr.IsAlreadyExists(qErr) {
					return appErrors.ErrQuadletAlreadyExists
				}
			}
		}

		// Resolve the installed path if podman reported one back.
		installedPath := ""
		if report != nil {
			installedPath = firstInstalledPath(report.InstalledQuadlets)
		}

		response = &QuadletCreateResponse{
			ID:       req.Name,
			Name:     req.Name,
			Path:     installedPath,
			Resource: strings.ToLower(req.Resource),
			State:    "unknown",
			Content:  req.Content,
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	if err := s.reloadUserSystemd(ctx, machineID, userID); err != nil {
		return nil, err
	}
	return response, nil
}

// Update replaces an existing quadlet's content on the specified machine.
//
// Podman has no dedicated update primitive, so the service composes
// Remove + Install (with replace) of the new content
// (doku/description_quadlets_api.md:114-138):
//
//  1. Existence check: locate the quadlet via List. A missing quadlet
//     maps to ErrQuadletNotFound (404). The list result also supplies the
//     resource extension used to name the rebuilt file.
//  2. Remove the existing quadlet (forced, so an active managed resource
//     does not block the replace; the PUT contract exposes no running-conflict
//     error, so an update is allowed even while the resource is up).
//  3. Install the new content, with replace and ReloadSystemd enabled.
//
// IMPORTANT: this flow is NOT atomic across the remove/install pair. A failure
// after the remove succeeds but before the install completes leaves the quadlet
// removed. The returned error reflects the install failure, but the caller
// cannot assume the original quadlet is still present.
//
// The response reuses the Get shape (doku/description_quadlets_api.md:128). State is
// reported as "unknown" immediately after the reinstall because systemd has not
// yet reloaded to report a fresh status — matching the Create slice's behaviour.
//
// Like Create, a successful reinstall is followed by a manager reload
// (reloadUserSystemd) so the rebuilt unit is visible to the lifecycle's
// scoped systemctl calls. A reload failure fails the update with
// ErrConnectionFailed even though the reinstall itself succeeded.
func (s *QuadletService) Update(ctx context.Context, userID, machineID, name string, req QuadletUpdateRequest) (*QuadletGetResponse, error) {
	var response *QuadletGetResponse
	err := s.engines.WithEngine(ctx, userID, machineID, "quadlets.Update", func(e QuadletEngine) error {
		// 1. Existence check + resource derivation. A missing quadlet is a 404.
		list, err := e.List(entities.QuadletListOptions{})
		if err != nil {
			return fmt.Errorf("failed to list quadlets: %w", err)
		}

		target := findQuadletByName(list, name)
		if target == nil {
			return appErrors.ErrQuadletNotFound
		}

		// 2. Remove the existing quadlet. Force: true so an active managed resource
		//    does not block the replace (the PUT contract has no running-conflict
		//    error). ReloadSystemd: true so systemd drops the old unit.
		if _, err := e.Remove(name, true); err != nil {
			if podmanerr.IsNotFound(err) {
				return appErrors.ErrQuadletNotFound
			}
			return fmt.Errorf("failed to remove existing quadlet: %w", err)
		}

		// 3. Install the new content. The filename preserves the original resource
		//    extension so the rebuilt quadlet keeps its type. Replace overwrites
		//    any leftover file from a partial state; ReloadSystemd picks up the
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

		report, err := e.Install(fileName, []byte(req.Content), true)
		if err != nil {
			return fmt.Errorf("failed to install updated quadlet: %w", err)
		}

		installedPath := ""
		if report != nil {
			installedPath = firstInstalledPath(report.InstalledQuadlets)
		}

		response = buildUpdateResponse(name, target.UnitName, resource, installedPath, req.Content)
		return nil
	})
	if err != nil {
		return nil, err
	}

	if err := s.reloadUserSystemd(ctx, machineID, userID); err != nil {
		return nil, err
	}
	return response, nil
}

// Delete removes a quadlet by name on the specified machine.
//
// The flow honours the contract's `force` and `stop` query parameters
// (doku/description_quadlets_api.md:140-161):
//
//   - The quadlet is located via a List pre-check. A missing quadlet
//     maps to ErrQuadletNotFound (404).
//   - If the managed resource is currently running:
//     force removes it over the running resource; without force, a true
//     `stop` (the default) stops the resource through the lifecycle seam
//     (applyUnitAction — `systemctl [--user] stop`, scoped to the SSH
//     login's manager) before the remove and
//     answers 204, while `stop=false` is refused with
//     ErrQuadletResourceRunning (400).
//   - A stop failure propagates and nothing is removed.
//
// Otherwise Remove is called with Force=force (systemd reload included)
// so systemd picks up the removal.
func (s *QuadletService) Delete(ctx context.Context, userID, machineID, name string, force, stop bool) error {
	return s.engines.WithEngine(ctx, userID, machineID, "quadlets.Delete", func(e QuadletEngine) error {
		// Pre-check: locate the quadlet and evaluate the running guard so we map
		// not-found / running-conflict before attempting the destructive call.
		list, err := e.List(entities.QuadletListOptions{})
		if err != nil {
			return fmt.Errorf("failed to list quadlets: %w", err)
		}

		target := findQuadletByName(list, name)
		if err := checkDeletePreconditions(target, force, stop); err != nil {
			return err
		}

		// Wire the contract's stop param: a running resource is stopped
		// through the lifecycle seam before remove, unless force is set
		// (the remove happens over the running resource) or stop=false
		// refused the delete above. The stop crosses the SSH seam only —
		// no nested engine lease — and its failure propagates so nothing
		// is removed.
		if parseStateFromStatus(target.Status) == "running" && !force {
			if err := s.applyUnitAction(ctx, userID, machineID, actionStop, resolveUnitName(target, name), LifecycleOptions{}); err != nil {
				return fmt.Errorf("failed to stop quadlet before remove: %w", err)
			}
		}

		report, err := e.Remove(name, force)
		if err != nil {
			if podmanerr.IsNotFound(err) {
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
				if podmanerr.IsNotFound(qErr) {
					return appErrors.ErrQuadletNotFound
				}
				if !force && isResourceRunningError(qErr) {
					return appErrors.ErrQuadletResourceRunning
				}
				return fmt.Errorf("failed to remove quadlet: %w", qErr)
			}
		}

		return nil
	})
}

// firstInstalledPath returns the destination path of the first installed
// quadlet reported by the install primitive, or the empty string when none is
// present. The install report's InstalledQuadlets is a path→path map whose
// ordering is non-deterministic, so "first" is arbitrary — callers only need
// *a* populated path to echo back, mirroring the install primitive's own
// reporting.
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
// the engine boundary.
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
// the managed resource is running and neither force nor the stop-before-remove
// waiver applies, or nil when the delete may proceed (with a stop first when
// the resource is running and stop is set).
func checkDeletePreconditions(target *entities.ListQuadlet, force, stop bool) error {
	if target == nil {
		return appErrors.ErrQuadletNotFound
	}
	if parseStateFromStatus(target.Status) == "running" && !force && !stop {
		return appErrors.ErrQuadletResourceRunning
	}
	return nil
}

// isResourceRunningError reports whether err is podman's refusal to remove
// a quadlet whose managed resource is still running. Podman phrases that
// refusal ("quadlet %s is running and force is not set, refusing to
// remove") outside its shared conflict vocabulary, so the running/active
// phrases stay quadlet-local — the same ruling as lifecycle.go's
// systemctl/stderr classifier. Everything the conflict vocabulary covers
// delegates to the podmanerr adapter.
func isResourceRunningError(err error) bool {
	if podmanerr.IsConflict(err) {
		return true
	}
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "running") || strings.Contains(msg, "active")
}

// GetLogs retrieves logs for the managed container of a quadlet: the
// service resolves the quadlet to its managed container name, then
// crosses the kit's buffered fetch (FetchLogs owns the options build,
// writer pair, combine, not-found mapping, and cancellation mapping).
// Returns `{id, logs}` on success; supports `since` and `tail` query
// params. See doku/description_quadlets_api.md.
func (s *QuadletService) GetLogs(ctx context.Context, userID, machineID, quadletName string, since time.Time, tail int64) (*QuadletLogsResponse, error) {
	var response *QuadletLogsResponse
	err := s.engines.WithEngine(ctx, userID, machineID, "quadlets.GetLogs", func(e QuadletEngine) error {
		containerName, err := s.resolveForLogs(e, quadletName)
		if err != nil {
			return err
		}

		logs, err := streaming.FetchLogs(ctx, streaming.FetchLogsOp{
			Target:   containerName,
			Since:    since,
			Tail:     tail,
			NotFound: appErrors.ErrQuadletNotFound,
			Logs:     e.Logs,
		})
		if err != nil {
			return err
		}

		response = &QuadletLogsResponse{
			ID:   quadletName,
			Logs: logs,
		}
		return nil
	})
	return response, err
}

// GetLogsWithStream retrieves logs for the managed container of a quadlet
// with streaming callback. When follow=true, this method streams log lines
// one at a time via the logCallback function. The follow ritual (options
// build, writer pair, not-found and cancellation mapping) belongs to the
// kit's FollowLogs; the service keeps engine acquisition and the
// quadlet-to-container resolution.
func (s *QuadletService) GetLogsWithStream(ctx context.Context, userID, machineID, quadletName string, since time.Time, tail int64, logCallback func(event streaming.LogEvent)) error {
	return s.engines.WithEngine(ctx, userID, machineID, "quadlets.GetLogsWithStream", func(e QuadletEngine) error {
		containerName, err := s.resolveForLogs(e, quadletName)
		if err != nil {
			return err
		}

		return streaming.FollowLogs(ctx, streaming.FollowLogsOp{
			Target:   containerName,
			Since:    since,
			Tail:     tail,
			NotFound: appErrors.ErrQuadletNotFound,
			Logs:     e.Logs,
			OnEvent:  logCallback,
		})
	})
}

// resolveForLogs performs the shared pre-work for GetLogs and GetLogsWithStream:
// it locates the quadlet (404 if absent) and resolves the managed container name.
// For container-type quadlets, the managed container name is the quadlet name;
// for pod-type quadlets, we use the Pod container name from the ListQuadlet result.
func (s *QuadletService) resolveForLogs(e QuadletEngine, quadletName string) (string, error) {
	target, err := s.resolveTarget(e, quadletName)
	if err != nil {
		return "", err
	}
	content, err := e.Print(quadletName)
	if err == nil {
		if name := explicitContainerNameFromQuadletContent(content); name != "" {
			return name, nil
		}
	}

	return resolveManagedContainerName(target, quadletName), nil
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

func (rt resourceType) inspectAndConvert(engine QuadletEngine, resourceID string) (string, string, error) {
	switch rt {
	case resourceTypeContainer:
		containerReports, _, err := engine.InspectContainer(resourceID)
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
		podReports, _, err := engine.InspectPod(resourceID)
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
	var response *QuadletGenerateResponse
	err := s.engines.WithEngine(ctx, userID, machineID, "quadlets.Generate", func(e QuadletEngine) error {
		rt := resourceType(req.ResourceType)
		quadletName, content, err := rt.inspectAndConvert(e, req.ResourceID)
		if err != nil {
			return err
		}

		response = &QuadletGenerateResponse{
			ID:       quadletName,
			Name:     quadletName,
			Resource: req.ResourceType,
			State:    "unknown",
			Content:  content,
		}
		return nil
	})
	return response, err
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

	fmt.Fprintf(&builder, "# Generated by Podland from container %s\n", quadletName)
	fmt.Fprintf(&builder, "[Unit]\n")
	fmt.Fprintf(&builder, "Description=Podman container %s\n\n", quadletName)

	fmt.Fprintf(&builder, "[Container]\n")
	fmt.Fprintf(&builder, "ContainerName=%s\n", quadletName)

	if inspect.Config.Image != "" {
		fmt.Fprintf(&builder, "Image=%s\n", inspect.Config.Image)
	}

	if len(inspect.Config.Env) > 0 {
		for _, env := range inspect.Config.Env {
			fmt.Fprintf(&builder, "Environment=%s\n", env)
		}
	}

	if inspect.HostConfig != nil {
		if inspect.HostConfig.RestartPolicy != nil {
			policy := inspect.HostConfig.RestartPolicy.Name
			if policy != "" && policy != "no" {
				fmt.Fprintf(&builder, "RestartPolicy=%s\n", policy)
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
				fmt.Fprintf(&builder, "Volume=%s\n", bind)
			}
		}

		if len(inspect.HostConfig.NetworkMode) > 0 {
			fmt.Fprintf(&builder, "Network=%s\n", inspect.HostConfig.NetworkMode)
		}
	}

	if inspect.Config.Labels != nil {
		for key, value := range inspect.Config.Labels {
			fmt.Fprintf(&builder, "Label=%s=%s\n", key, value)
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

	fmt.Fprintf(&builder, "# Generated by Podland from pod %s\n", podName)
	fmt.Fprintf(&builder, "[Unit]\n")
	fmt.Fprintf(&builder, "Description=Podman pod %s\n\n", podName)

	fmt.Fprintf(&builder, "[Pod]\n")
	fmt.Fprintf(&builder, "PodName=%s\n", podName)

	if len(inspect.CreateCommand) > 0 {
		builder.WriteString("PodmanArgs=")
		builder.WriteString(strings.Join(inspect.CreateCommand, " "))
		builder.WriteString("\n")
	}

	if len(inspect.Labels) > 0 {
		for key, value := range inspect.Labels {
			fmt.Fprintf(&builder, "Label=%s=%s\n", key, value)
		}
	}

	return builder.String(), nil
}

// Import installs multiple quadlets on the specified machine from uploaded content.
// Each entry is installed by name+resource with systemd reload enabled (the tar
// framing is the engine adapter's job). The method handles partial failures by
// collecting successful and failed imports separately.
//
// After the batch, one user-manager reload (reloadUserSystemd) covers every
// installed entry — the same manager-visibility guarantee Create gives a
// single quadlet. A reload failure fails the import with ErrConnectionFailed
// even though the installs themselves succeeded.
func (s *QuadletService) Import(ctx context.Context, userID, machineID string, req QuadletImportRequest) (*QuadletImportResponse, error) {
	var response *QuadletImportResponse
	err := s.engines.WithEngine(ctx, userID, machineID, "quadlets.Import", func(e QuadletEngine) error {
		response = &QuadletImportResponse{
			Imported: make([]QuadletImportResult, 0),
			Failed:   make([]QuadletImportError, 0),
		}

		for _, entry := range req.Quadlets {
			importResult, err := s.importSingleQuadlet(e, entry)
			if err != nil {
				response.Failed = append(response.Failed, QuadletImportError{
					Name:  entry.Name,
					Error: err.Error(),
				})
			} else {
				response.Imported = append(response.Imported, importResult)
			}
		}

		return nil
	})
	if err != nil {
		return nil, err
	}

	// One reload covers the whole batch — but only when something was
	// actually installed (an empty import touches nothing).
	if len(response.Imported) > 0 {
		if err := s.reloadUserSystemd(ctx, machineID, userID); err != nil {
			return nil, err
		}
	}
	return response, nil
}

// importSingleQuadlet imports a single quadlet entry.
// Returns the import result or an error explaining why the import failed.
func (s *QuadletService) importSingleQuadlet(e QuadletEngine, entry QuadletImportEntry) (QuadletImportResult, error) {
	fileName := entry.Name + resourceToExtension(entry.Resource)

	report, err := e.Install(fileName, []byte(entry.Content), false)
	if err != nil {
		if podmanerr.IsAlreadyExists(err) {
			return QuadletImportResult{}, fmt.Errorf("quadlet already exists")
		}
		return QuadletImportResult{}, fmt.Errorf("failed to install quadlet: %w", err)
	}

	if report != nil {
		for _, qErr := range report.QuadletErrors {
			if qErr != nil && podmanerr.IsAlreadyExists(qErr) {
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
