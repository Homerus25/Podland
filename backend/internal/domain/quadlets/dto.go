package quadlets

import (
	"strings"
	"time"

	"github.com/Homerus25/podland/backend/internal/transfer"
	"go.podman.io/podman/v6/pkg/domain/entities"
)

// QuadletListResponse represents a quadlet in the API list response.
type QuadletListResponse struct {
	// Unique identifier for the quadlet (the quadlet name)
	ID string `json:"id"`
	// Name of the quadlet
	Name string `json:"name"`
	// Path to the quadlet file
	Path string `json:"path"`
	// Systemd unit name
	Unit string `json:"unit"`
	// Resource type (container, pod, image, volume, network)
	Resource string `json:"resource"`
	// State of the quadlet (unknown, stopped, running, error)
	State string `json:"state"`
	// Raw quadlet file content (optional)
	Raw string `json:"raw,omitempty"`
}

// QuadletGetResponse represents a quadlet in the API get response.
type QuadletGetResponse struct {
	// Unique identifier for the quadlet (the quadlet name)
	ID string `json:"id"`
	// Name of the quadlet
	Name string `json:"name"`
	// Path to the quadlet file
	Path string `json:"path"`
	// Systemd unit name
	Unit string `json:"unit"`
	// Resource type (container, pod, image, volume, network)
	Resource string `json:"resource"`
	// State of the quadlet (unknown, stopped, running, error)
	State string `json:"state"`
	// Raw quadlet file content
	Content string `json:"content"`
}

// QuadletUpdateRequest represents a request to replace an existing quadlet's
// content. Only the raw file content is sent; the quadlet name comes from the
// URL path and the resource type is derived from the existing quadlet's path
// during the existence check.
type QuadletUpdateRequest struct {
	// Raw quadlet file content. Required.
	Content string `json:"content" validate:"required,min=1"`
}

// QuadletCreateRequest represents a request to create a new quadlet.
type QuadletCreateRequest struct {
	// Name of the quadlet (without extension). Required.
	Name string `json:"name" validate:"required,min=1"`
	// Resource type. Must be one of: container, pod, image, volume, network. Required.
	Resource string `json:"resource" validate:"required,oneof=container pod image volume network"`
	// Raw quadlet file content. Required.
	Content string `json:"content" validate:"required,min=1"`
}

// QuadletCreateResponse represents a quadlet in the API create response.
// Per the epic's no-persistence ruling, generation/createdAt/updatedAt are
// omitted (documented deviation from the contract at doku/description_quadlets_api.md:89-110).
type QuadletCreateResponse struct {
	// Unique identifier for the quadlet (the quadlet name)
	ID string `json:"id"`
	// Name of the quadlet (without extension)
	Name string `json:"name"`
	// Path to the installed quadlet file (populated when podman reports it)
	Path string `json:"path,omitempty"`
	// Systemd unit name (often empty immediately after install before systemd reload)
	Unit string `json:"unit,omitempty"`
	// Resource type (container, pod, image, volume, network)
	Resource string `json:"resource"`
	// State of the quadlet. Newly installed quadlets are reported as "unknown"
	// until systemd reload completes and reports a status.
	State string `json:"state"`
	// The raw quadlet file content that was installed (echoed back from the request)
	Content string `json:"content"`
}

// LifecycleOptions carries the optional timeout parameter for stop and
// restart operations (doku/description_quadlets_api.md:199-212, 238-243). Start has
// no options per the contract.
//
// There is deliberately no force option: systemctl's --force is an
// enable/link/mask and halt-family flag that systemd accepts but ignores for
// stop and restart, so a force param would be a silently hollow affordance
// (issue 205). Aggressive teardown stays where podman actually has it —
// DELETE's force (podman rm --force) — and a slow stop is bounded by the
// timeout below.
type LifecycleOptions struct {
	// Timeout is the stop/restart grace period in seconds. 0 means unset.
	// Maps to the coreutils `timeout <n>` wrapper around the systemctl call
	// (no systemd version has a per-stop-call timeout flag); expiry answers
	// QUADLET_STOP_TIMED_OUT (504).
	Timeout int
}

// QuadletLifecycleResponse is the success body for the start/stop/restart
// endpoints (doku/description_quadlets_api.md:178-184, 207-213, 237-243). The shape
// is intentionally narrow: id + state + unit, with restartedAt on restart only.
type QuadletLifecycleResponse struct {
	// Unique identifier for the quadlet (the quadlet name)
	ID string `json:"id"`
	// State of the quadlet after the operation (running / stopped)
	State string `json:"state"`
	// Systemd unit name that was acted on
	Unit string `json:"unit"`
	// RestartedAt is the timestamp of a restart (restart only, ISO8601).
	RestartedAt string `json:"restartedAt,omitempty"`
}

// FromListQuadlet converts a podman entities.ListQuadlet to QuadletListResponse.
func FromListQuadlet(quadlet *entities.ListQuadlet) QuadletListResponse {
	if quadlet == nil {
		return QuadletListResponse{}
	}

	return QuadletListResponse{
		ID:       quadlet.Name,
		Name:     quadlet.Name,
		Path:     quadlet.Path,
		Unit:     quadlet.UnitName,
		Resource: deriveResourceFromPath(quadlet.Path),
		State:    parseStateFromStatus(quadlet.Status),
		Raw:      "", // Raw content not included in list response
	}
}

// ToQuadletListResponse converts a slice of podman ListQuadlet to a slice of QuadletListResponse.
func ToQuadletListResponse(quadlets []*entities.ListQuadlet) []QuadletListResponse {
	result := make([]QuadletListResponse, len(quadlets))
	for i, quadlet := range quadlets {
		result[i] = FromListQuadlet(quadlet)
	}
	return result
}

// deriveResourceFromPath derives the resource type from the quadlet file path.
func deriveResourceFromPath(path string) string {
	if path == "" {
		return "unknown"
	}

	lowerPath := strings.ToLower(path)
	switch {
	case strings.HasSuffix(lowerPath, ".container"):
		return "container"
	case strings.HasSuffix(lowerPath, ".pod"):
		return "pod"
	case strings.HasSuffix(lowerPath, ".image"):
		return "image"
	case strings.HasSuffix(lowerPath, ".volume"):
		return "volume"
	case strings.HasSuffix(lowerPath, ".network"):
		return "network"
	case strings.HasSuffix(lowerPath, ".kube"):
		return "container" // Map kube to closest contract value
	case strings.HasSuffix(lowerPath, ".build"):
		return "image" // Map build to closest contract value
	default:
		return "unknown"
	}
}

// parseStateFromStatus parses the systemd status string into a normalized state.
func parseStateFromStatus(status string) string {
	if status == "" {
		return "unknown"
	}

	lowerStatus := strings.ToLower(status)
	switch {
	case strings.Contains(lowerStatus, "running"):
		return "running"
	case strings.Contains(lowerStatus, "stopped"):
		return "stopped"
	case strings.Contains(lowerStatus, "failed") || strings.Contains(lowerStatus, "error"):
		return "error"
	default:
		return "unknown"
	}
}

// QuadletGenerateRequest represents a request to generate a quadlet from an existing
// container or pod on the remote host.
type QuadletGenerateRequest struct {
	// ID of the container or pod to generate a quadlet from. Required.
	ResourceID string `json:"resourceId" validate:"required,min=1"`
	// Type of resource to generate from. Must be either "container" or "pod". Required.
	ResourceType string `json:"resourceType" validate:"required,oneof=container pod"`
}

// QuadletGenerateResponse represents the response from generating a quadlet.
// The response includes the generated quadlet content with state "unknown" since
// the quadlet is not installed or started by this endpoint.
type QuadletGenerateResponse struct {
	// Unique identifier for the quadlet (the quadlet name)
	ID string `json:"id"`
	// Name of the quadlet
	Name string `json:"name"`
	// Path to the quadlet file (empty since not installed)
	Path string `json:"path,omitempty"`
	// Systemd unit name (empty since not installed)
	Unit string `json:"unit,omitempty"`
	// Resource type (container or pod)
	Resource string `json:"resource"`
	// State of the quadlet (always "unknown" for generated quadlets)
	State string `json:"state"`
	// Generated quadlet file content
	Content string `json:"content"`
}

// resourceToExtension maps a resource type to its quadlet filename extension.
// This is the inverse of deriveResourceFromPath for the contract's resource enum
// (container, pod, image, volume, network). Unknown resources default to ".container".
func resourceToExtension(resource string) string {
	switch strings.ToLower(resource) {
	case "container", "pod", "image", "volume", "network":
		return "." + strings.ToLower(resource)
	default:
		return ".container"
	}
}

// QuadletLogsResponse represents the response for quadlet logs (non-streaming)
type QuadletLogsResponse struct {
	// Quadlet ID (the quadlet name)
	ID string `json:"id"`
	// Quadlet logs content (multiline string)
	Logs string `json:"logs"`
}

// resolveManagedContainerName resolves the managed container name for a quadlet.
// For container-type quadlets, the managed container is the systemd-spawned
// container, which podman names `systemd-<basename>` — the quadlet file name
// without its resource extension (podman quadlet generator: `--name systemd-%N`).
// For pod-type quadlets, we use the Pod container name from the ListQuadlet result.
// An explicit `ContainerName=` key in the quadlet content overrides this
// convention (see explicitContainerNameFromQuadletContent); callers that have
// the content at hand should check that first.
func resolveManagedContainerName(quadlet *entities.ListQuadlet, quadletName string) string {
	if quadlet == nil {
		return quadletName
	}

	// For pod-type quadlets, use the Pod container name if available
	if quadlet.Pod != "" {
		return quadlet.Pod
	}

	// For container-type quadlets and others, derive the systemd-spawned
	// container name: strip the resource extension ("foo.container" → "foo")
	// and apply podman quadlet's "systemd-" naming convention.
	base := quadletName
	if idx := strings.LastIndex(quadletName, "."); idx > 0 {
		base = quadletName[:idx]
	}
	return "systemd-" + base
}

// explicitContainerNameFromQuadletContent extracts the value of the
// `ContainerName=` key from quadlet file content. Podman's quadlet generator
// passes it to `podman run --name`, so when present the managed container is
// named verbatim instead of the default `systemd-<basename>`. Returns "" when
// the key is absent (or has an empty value), meaning the default applies.
// Key matching is case-insensitive, matching systemd's unit-file parser; the
// value is used verbatim apart from surrounding whitespace and quotes.
func explicitContainerNameFromQuadletContent(content string) string {
	for line := range strings.Lines(content) {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}
		key, value, found := strings.Cut(line, "=")
		if !found || !strings.EqualFold(strings.TrimSpace(key), "ContainerName") {
			continue
		}
		value = strings.TrimSpace(value)
		value = strings.Trim(value, `"'`)
		if value != "" {
			return value
		}
		return ""
	}
	return ""
}

// QuadletImportRequest represents a request to import quadlets from uploaded content.
// Each entry contains the quadlet name, resource type, and file content.
type QuadletImportRequest struct {
	// List of quadlets to import. At least one entry is required.
	Quadlets []QuadletImportEntry `json:"quadlets" validate:"required,min=1,dive"`
}

// QuadletImportEntry represents a single quadlet to import.
type QuadletImportEntry struct {
	// Name of the quadlet (without extension). Required.
	Name string `json:"name" validate:"required,min=1"`
	// Resource type. Must be one of: container, pod, image, volume, network. Required.
	Resource string `json:"resource" validate:"required,oneof=container pod image volume network"`
	// Raw quadlet file content. Required.
	Content string `json:"content" validate:"required,min=1"`
}

// QuadletImportResponse represents the response from importing quadlets.
// Returns separate arrays for successfully imported and failed quadlets.
type QuadletImportResponse struct {
	// Successfully imported quadlets
	Imported []QuadletImportResult `json:"imported"`
	// Failed imports with error details
	Failed []QuadletImportError `json:"failed"`
}

// QuadletImportResult represents a successfully imported quadlet.
type QuadletImportResult struct {
	// Unique identifier for the quadlet (the quadlet name)
	ID string `json:"id"`
	// Name of the quadlet
	Name string `json:"name"`
	// Path to the installed quadlet file
	Path string `json:"path"`
	// State of the quadlet (reported as "unknown" after import)
	State string `json:"state"`
}

// QuadletImportError represents a failed quadlet import with error details.
type QuadletImportError struct {
	// Name of the quadlet that failed to import
	Name string `json:"name"`
	// Error message explaining why the import failed
	Error string `json:"error"`
}

// MaxComposeContentBytes caps the raw request body the convert-compose
// endpoint accepts: 1 MiB (1 << 20 = 1,048,576 bytes), no slack. Enforced
// at the HTTP layer — the handler wraps r.Body in http.MaxBytesReader
// before web.Decode — so an oversized body is never fully buffered and
// answers 400 INVALID_REQUEST. With the raw cap at exactly 1 MiB, the
// decoded content string can be at most 1,048,562 characters, so no
// post-decode size tag is needed.
const MaxComposeContentBytes = 1 << 20

// QuadletConvertComposeRequest represents a request to convert compose file
// content into quadlet entries (spec #336). The endpoint is global — not
// host-scoped — so the body carries only the compose text.
type QuadletConvertComposeRequest struct {
	// Compose file content. Required; the 1 MiB body cap is enforced at
	// the HTTP layer (see MaxComposeContentBytes).
	Content string `json:"content" validate:"required"`
}

// QuadletConvertComposeResponse carries the generated quadlet entries in
// the import endpoint's {name, resource, content} shape, so the frontend
// can forward the body to the host-scoped import endpoint unchanged.
type QuadletConvertComposeResponse struct {
	// Generated quadlet entries, sorted by name.
	Quadlets []QuadletImportEntry `json:"quadlets"`
}

// Kind labels for QuadletMovePreviewVolume.Source: a volume source
// containing no path separator is a named podman volume, anything else is
// a host bind path (issue 209).
const (
	MovePreviewVolumeKindVolume   = "volume"
	MovePreviewVolumeKindHostPath = "hostPath"
)

// Target-state labels for QuadletMovePreviewVolume.TargetState (issue 217):
// how the companion .volume quadlet file already named by quadletFile sits
// on the TARGET host — absent, byte-identical, or with different
// content. An identical companion is skipped by the transfer (no install,
// no error); a differing one fails it with 409 QUADLET_ALREADY_EXISTS.
const (
	MovePreviewCompanionAbsent    = "absent"
	MovePreviewCompanionIdentical = "identical"
	MovePreviewCompanionDiffers   = "differs"
)

// QuadletMovePreviewVolume is one `Volume=` reference in the move/copy
// preview: the piece of data the transfer leaves behind on the source
// host — or, since issue 214, the podman volume whose data may follow
// the file when the request opts into migration.
type QuadletMovePreviewVolume struct {
	// Volume source — the named podman volume or host path, i.e. the
	// part of the `Volume=` value before the first ":".
	Source string `json:"source"`
	// Kind labels the source: "volume" (named podman volume) or
	// "hostPath" (host bind path).
	Kind string `json:"kind"`
	// QuadletFile names the companion .volume quadlet file on the
	// source host the reference resolves to — present exactly when a
	// same-named quadlet file exists on the source (the reference is
	// dangling otherwise, and the entry stays ordinary). Empty means no
	// companion (issue 217).
	QuadletFile string `json:"quadletFile,omitempty"`
	// TargetState labels the companion's state on the target host:
	// "absent", "identical", or "differs". Present exactly when
	// QuadletFile is.
	TargetState string `json:"targetState,omitempty"`
	// PodmanVolume is the podman volume the reference resolves to on the
	// source host (issue 214): the bare name itself, or
	// `systemd-<name>` for a .volume reference unless the companion file
	// sets VolumeName=. Present only on volume-kind entries; host paths
	// never resolve.
	PodmanVolume string `json:"podmanVolume,omitempty"`
	// SizeBytes is the source volume's content size in bytes when the
	// engine can report one (source volume inspect, issue 214). Podman's
	// service API reports no volume usage today, so this is omitted until
	// an engine that reports sizes answers; it is a pointer so an unknown
	// size is distinguishable from zero.
	SizeBytes *int64 `json:"sizeBytes,omitempty"`
	// ExistsOnTarget reports whether the resolved podman volume already
	// exists on the target host (target volume inspect, issue 214).
	// Volume-kind entries always carry it — false is meaningful (a fresh
	// target); host-path entries render it false too, it just says
	// nothing there.
	ExistsOnTarget bool `json:"existsOnTarget"`
	// TargetSizeBytes is the target volume's content size when the
	// engine can report one and the volume exists (issue 214). Same
	// unknown-today contract as SizeBytes.
	TargetSizeBytes *int64 `json:"targetSizeBytes,omitempty"`
	// Migratable reports whether migrateVolumeData can carry this
	// reference's data: true for a volume-kind reference whose source
	// volume exists, false for host bind paths (no podman export path)
	// and dangling references (no source volume) (issue 214).
	Migratable bool `json:"migratable"`
	// Reason explains a non-migratable entry. Present only when
	// Migratable is false (issue 214).
	Reason string `json:"reason,omitempty"`
}

// ToMovePreviewVolumes projects one volume resolution into the preview's
// wire rows (issue 230, §18): one row per REFERENCE, in file order — two
// references resolving to one volume stay two rows sharing podmanVolume —
// carrying the companion fields where discovery resolved a source file (a
// bare name or a dangling .volume reference stays an ordinary entry,
// issue 217) and the migration facts where the reference resolves to a
// podman volume at all (issue 214): host bind paths are labeled not
// migratable, dangling references carry their reason, existing source
// volumes are migratable with whatever size the engine reported (nil
// until an engine reports one). targetStates maps a discovered companion
// file to its state on the target host; targetVolumes maps a resolved
// volume name to the target's inspect (nil inspect = absent). Used once,
// at MovePreview's response assembly.
func ToMovePreviewVolumes(resolution volumeResolution, targetStates map[string]string, targetVolumes map[string]*QuadletVolumeInspect) []QuadletMovePreviewVolume {
	sourceVolumes := make(map[string]resolvedVolume, len(resolution.Volumes))
	for _, volume := range resolution.Volumes {
		sourceVolumes[volume.Name] = volume
	}

	volumes := []QuadletMovePreviewVolume{}
	for _, ref := range resolution.Refs {
		entry := QuadletMovePreviewVolume{Source: ref.Source, Kind: ref.Kind}
		if state, resolved := targetStates[ref.QuadletFile]; resolved {
			entry.QuadletFile = ref.QuadletFile
			entry.TargetState = state
		}
		if !ref.isPodmanVolume() {
			entry.Reason = MovePreviewReasonHostPath
			volumes = append(volumes, entry)
			continue
		}
		entry.PodmanVolume = ref.PodmanVolume
		if src := sourceVolumes[ref.PodmanVolume]; src.Inspect != nil {
			entry.SizeBytes = src.Inspect.Size
			entry.Migratable = true
		} else {
			entry.Reason = MovePreviewReasonDangling
		}
		if tgt := targetVolumes[ref.PodmanVolume]; tgt != nil {
			entry.ExistsOnTarget = true
			entry.TargetSizeBytes = tgt.Size
		}
		volumes = append(volumes, entry)
	}
	return volumes
}

// QuadletMovePreviewResponse is the body of the move/copy preview
// endpoint (GET .../quadlets/{quadletId}/move-preview): everything the
// transfer dialog must show before confirming. See
// doku/description_quadlets_api.md.
type QuadletMovePreviewResponse struct {
	// Every `Volume=` reference in the quadlet content, in file order.
	// Empty (not null) when the quadlet references none.
	Volumes []QuadletMovePreviewVolume `json:"volumes"`
	// Every `Secret=` reference in the quadlet content, in file order
	// (issue 215). Always computed, flag-independent — the volume facts'
	// discipline (issue 214). Empty (not null) when the quadlet
	// references none.
	Secrets []QuadletMovePreviewSecret `json:"secrets"`
	// Collision is true exactly when the target host already has a
	// quadlet of the same name and resource.
	Collision bool `json:"collision"`
	// Running reflects the source quadlet's live state.
	Running bool `json:"running"`
}

// Mode labels for the move/copy transfer endpoint (issue 207): "copy"
// installs on the target only; "move" (issue 211) additionally removes the
// source quadlet after the install succeeded.
const (
	MoveModeCopy = "copy"
	MoveModeMove = "move"
)

// Transfer status labels for the move/copy endpoint (issue 207): copy
// answers "copied"; move mode answers "moved", or "partial" when the target
// install succeeded but the source removal failed (the quadlet then exists
// on both hosts).
const (
	MoveStatusCopied  = "copied"
	MoveStatusMoved   = "moved"
	MoveStatusPartial = "partial"
)

// QuadletMoveRequest is the body of the move/copy transfer endpoint
// (POST .../quadlets/{quadletId}/move, issues 210 and 211). Copy mode
// installs the source quadlet's file on the target without overwriting and
// never touches the source; move mode removes the source quadlet after the
// install succeeded, honouring the delete flow's stop/force flags.
type QuadletMoveRequest struct {
	// TargetHostID is the host the quadlet is transferred to. Required.
	TargetHostID string `json:"targetHostId" validate:"required,min=1"`
	// Mode selects the transfer semantics. "copy" installs on the target
	// only; "move" additionally removes the source after the install. Required.
	Mode string `json:"mode" validate:"required,oneof=copy move"`
	// Stop removes a RUNNING source quadlet by stopping its unit first —
	// the delete flow's stop-before-remove. Move mode only; copy never
	// stops anything and ignores the flag. Defaults to false, so a running
	// source quadlet refuses the move (QUADLET_RESOURCE_RUNNING) unless
	// stop or force is set.
	Stop bool `json:"stop,omitempty"`
	// Force removes the source quadlet over its running resource — the
	// delete flow's force. Move mode only; copy ignores the flag.
	// Defaults to false.
	Force bool `json:"force,omitempty"`
	// TransferVolumeQuadlets carries the companion .volume quadlet files
	// the quadlet references along with the transfer (installed on the
	// target before the main quadlet, issue 217). A pointer so ABSENT can
	// mean true — the default — and only an explicit false opts out;
	// read it through WantsVolumeQuadletTransfer, never directly.
	TransferVolumeQuadlets *bool `json:"transferVolumeQuadlets,omitempty"`
	// MigrateVolumeData opts the transfer into carrying the referenced
	// podman volumes' DATA along (issue 214): every volume-kind reference
	// is resolved from the source content at execution time and exported
	// from the source host into the target's volume. Default false —
	// data movement is the heavy, consequential act. Setting it also
	// switches the response transport to NDJSON streaming.
	MigrateVolumeData bool `json:"migrateVolumeData,omitempty"`
	// OverwriteVolumes opts into MERGING the imported data into target
	// volumes that already hold content (issue 214) — podman import is
	// merge, never replace. Default false: a non-empty target volume is
	// refused with 409 VOLUME_MIGRATION_TARGET_NOT_EMPTY before anything
	// is installed. Applies to both modes' data phase.
	OverwriteVolumes bool `json:"overwriteVolumes,omitempty"`
	// TransferSecrets opts the transfer into carrying the referenced
	// podman secrets' VALUES along (issue 215): every `Secret=` reference
	// is re-derived from the source content at execution time, revealed
	// on the source host, and created on the target. Default false —
	// a value movement is the consequential act. Requires admin on the
	// source (the reveal gate stays the single owner of value exposure:
	// a write-level caller must never move values off a host) and
	// write on the target; orthogonal to transferVolumeQuadlets and
	// migrateVolumeData. Never removes a secret from the source, any
	// mode.
	TransferSecrets bool `json:"transferSecrets,omitempty"`
}

// WantsVolumeQuadletTransfer answers whether the transfer carries the
// companion .volume quadlet files along: true unless the request said
// false explicitly (nil counts as true — companions ride along by
// default, issue 217).
func (r QuadletMoveRequest) WantsVolumeQuadletTransfer() bool {
	return r.TransferVolumeQuadlets == nil || *r.TransferVolumeQuadlets
}

// QuadletMoveResponse is the transfer envelope both modes answer with
// (issue 210). Status carries the outcome: "copied" for a completed copy,
// "moved" for a completed move, and "partial" when a move's target install
// succeeded but the source removal failed — the quadlet then exists on both
// hosts, and Error carries the removal failure's detail (issue 211).
type QuadletMoveResponse struct {
	// Unique identifier for the quadlet (the quadlet name, extension
	// included — the transfer keeps the source's file name verbatim).
	ID string `json:"id"`
	// Name of the transferred quadlet (identical to ID).
	Name string `json:"name"`
	// Resource type derived from the quadlet's install path (container,
	// pod, image, volume, network) — the same derivation every list
	// response uses.
	Resource string `json:"resource"`
	// Status of the transfer: "copied", "moved", or "partial".
	Status string `json:"status"`
	// SourceHostID is the host the quadlet was transferred from.
	SourceHostID string `json:"sourceHostId"`
	// TargetHostID is the host the quadlet was installed on.
	TargetHostID string `json:"targetHostId"`
	// Path of the installed file on the target (populated when podman
	// reports it back).
	Path string `json:"path,omitempty"`
	// Error details why the source removal failed when Status is
	// "partial" — the quadlet exists on both hosts and the source side
	// must be cleaned up manually. Empty otherwise.
	Error string `json:"error,omitempty"`
	// VolumeMigration reports the data phase's per-volume outcomes when
	// the request opted into migrateVolumeData (issue 214). Absent for
	// file-only transfers. Present even when every reference was skipped,
	// so skipped danglings stay visible on success.
	VolumeMigration *QuadletMoveVolumeMigration `json:"volumeMigration,omitempty"`
	// SecretTransfer reports the secrets phase's per-reference outcomes
	// when the request opted into transferSecrets (issue 215), beside the
	// volumeMigration block. Present even when every reference was
	// skipped, so skipped danglings and host paths stay visible. Absent
	// otherwise; no progress events and no NDJSON transport ride on it
	// alone.
	SecretTransfer *QuadletMoveSecretTransfer `json:"secretTransfer,omitempty"`
}

// Volume-migration outcome labels for QuadletMoveVolumeOutcome.Outcome
// (issue 214): "migrated" — the volume's data was exported from the
// source and imported into the target; "skipped" — the reference carried
// no data (host paths are not in the set; a dangling reference has no
// source volume), with Reason saying why.
const (
	MoveVolumeOutcomeMigrated = "migrated"
	MoveVolumeOutcomeSkipped  = "skipped"
)

// Preview not-migratable reasons (issue 214): the honest label a
// non-migratable preview entry carries.
const (
	MovePreviewReasonHostPath = "host bind paths are not migrated"
	MovePreviewReasonDangling = "source volume does not exist"
)

// QuadletMoveVolumeOutcome is one reference's data-phase result (issue
// 214). Ref is the `Volume=` source as written in the quadlet;
// PodmanVolume is the podman volume it resolved to; Outcome is
// "migrated" or "skipped"; Reason explains a skip.
type QuadletMoveVolumeOutcome struct {
	Ref          string `json:"ref"`
	PodmanVolume string `json:"podmanVolume"`
	Outcome      string `json:"outcome"`
	Reason       string `json:"reason,omitempty"`
}

// QuadletMoveVolumeMigration is the transfer envelope's additive
// volumeMigration block (issue 214): one outcome per referenced
// volume-kind source, in file order (a source set deduplicated by
// resolved podman volume).
type QuadletMoveVolumeMigration struct {
	Volumes []QuadletMoveVolumeOutcome `json:"volumes"`
}

// Stream-event type labels for the move/copy transfer's NDJSON transport
// (issue 214): per-volume progress events, then exactly one terminal
// event — the transfer envelope on success, the mapped error otherwise.
const (
	MoveEventTypeVolume   = "volume"
	MoveEventTypeComplete = "complete"
	MoveEventTypeError    = "error"
)

// Volume progress phase labels for QuadletMoveEvent.Phase (issue 214):
// "start" when a volume's stream begins, "bytes" as the counting reader
// crosses its reporting increment, "done" when the import finished,
// "skip" when the reference carried no data.
const (
	MoveVolumePhaseStart = "start"
	MoveVolumePhaseBytes = "bytes"
	MoveVolumePhaseDone  = "done"
	MoveVolumePhaseSkip  = "skip"
)

// MoveError carries an error code/message in a terminal move event.
type MoveError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// QuadletMoveEvent is one NDJSON event in the move/copy transfer stream
// (issue 214). Volume events carry Ref/PodmanVolume/Phase (plus Bytes and
// Reason where the phase has them); the terminal complete event carries
// the transfer envelope; the terminal error event carries the mapped
// code/message. All events share operationId, hostId, and timestamp —
// the auto-update event shape.
type QuadletMoveEvent struct {
	OperationID  string               `json:"operationId"`
	Type         string               `json:"type"`
	HostID       string               `json:"hostId"`
	Ref          string               `json:"ref,omitempty"`
	PodmanVolume string               `json:"podmanVolume,omitempty"`
	Phase        string               `json:"phase,omitempty"`
	Bytes        int64                `json:"bytes,omitempty"`
	Reason       string               `json:"reason,omitempty"`
	Result       *QuadletMoveResponse `json:"result,omitempty"`
	Error        *MoveError           `json:"error,omitempty"`
	Timestamp    string               `json:"timestamp"`
}

// NewQuadletMoveVolumeEvent wraps one data-phase progress event.
func NewQuadletMoveVolumeEvent(operationID, hostID, phase, ref, podmanVolume string, bytes int64, reason string) QuadletMoveEvent {
	return QuadletMoveEvent{
		OperationID:  operationID,
		Type:         MoveEventTypeVolume,
		HostID:       hostID,
		Ref:          ref,
		PodmanVolume: podmanVolume,
		Phase:        phase,
		Bytes:        bytes,
		Reason:       reason,
		Timestamp:    time.Now().UTC().Format(time.RFC3339),
	}
}

// NewQuadletMoveCompleteEvent builds the terminal complete event carrying
// the transfer envelope.
func NewQuadletMoveCompleteEvent(operationID, hostID string, result QuadletMoveResponse) QuadletMoveEvent {
	return QuadletMoveEvent{
		OperationID: operationID,
		Type:        MoveEventTypeComplete,
		HostID:      hostID,
		Result:      &result,
		Timestamp:   time.Now().UTC().Format(time.RFC3339),
	}
}

// NewQuadletMoveErrorEvent builds the terminal error event.
func NewQuadletMoveErrorEvent(operationID, hostID, code, message string) QuadletMoveEvent {
	return QuadletMoveEvent{
		OperationID: operationID,
		Type:        MoveEventTypeError,
		HostID:      hostID,
		Error:       &MoveError{Code: code, Message: message},
		Timestamp:   time.Now().UTC().Format(time.RFC3339),
	}
}

// Kind labels for QuadletMovePreviewSecret.Kind (issue 215): a `Secret=`
// reference whose resolved source is an absolute path is read off the
// host filesystem at run time — the secret store can never resolve it and
// existence is unknowable through the podman API; anything else names a
// podman secret.
const (
	MovePreviewSecretKindSecret   = "secret"
	MovePreviewSecretKindHostPath = "hostPath"
)

// QuadletMovePreviewSecret is one `Secret=` reference in the move/copy
// preview (issue 215). The scan is type-generic — every `Secret=` line of
// the content counts, whatever the quadlet's resource type. The existence
// claims mean "in the host's podman secret store" and are only
// meaningful for kind secret entries; a hostPath entry renders both false
// (no existence claims), the same way host-path volume entries do.
type QuadletMovePreviewSecret struct {
	// Secret name — the token before the first comma of the `Secret=`
	// value, or the `source=` option when that token is the all-options
	// form.
	Name string `json:"name"`
	// Kind labels the reference: "secret" (a podman secret) or "hostPath"
	// (an absolute-path source read off the host filesystem).
	Kind string `json:"kind"`
	// ExistsOnSource reports whether the source host's podman secret
	// store holds this name. A false here is an honest dangling entry,
	// not an error.
	ExistsOnSource bool `json:"existsOnSource"`
	// ExistsOnTarget reports whether the target host's podman secret
	// store holds this name (read through the connection seam, like the
	// collision check).
	ExistsOnTarget bool `json:"existsOnTarget"`
}

// Secret-transfer outcome labels for QuadletMoveSecretOutcome.Outcome
// (issue 215): "transferred" — the value was revealed on the source and
// the secret created on the target; "skipped" — the reference carried
// nothing, with Reason saying why.
const (
	MoveSecretOutcomeTransferred = "transferred"
	MoveSecretOutcomeSkipped     = "skipped"
)

// Secret skip reasons (issue 215): the honest label a skipped `Secret=`
// reference's outcome carries. A same-name skip keeps re-runs safe (the
// identical-companion argument) while touching no existing target state;
// values are never compared, so a skip never means "equal".
const (
	MoveSecretSkipHostPath = "host path references are not transferred"
	MoveSecretSkipDangling = "source secret does not exist"
	MoveSecretSkipOnTarget = "a secret with this name already exists on the target"
)

// QuadletMoveSecretOutcome is one `Secret=` reference's phase result
// (issue 215). Name is the resolved secret name; Outcome is "transferred"
// or "skipped"; Reason explains a skip.
type QuadletMoveSecretOutcome struct {
	Name    string `json:"name"`
	Outcome string `json:"outcome"`
	Reason  string `json:"reason,omitempty"`
}

// QuadletMoveSecretTransfer is the transfer envelope's additive
// secretTransfer block (issue 215): one outcome per `Secret=` reference,
// in file order (deduplicated by resolved name). Present whenever the
// request opted into transferSecrets — even when every reference was
// skipped, so skipped danglings and host paths stay visible.
type QuadletMoveSecretTransfer struct {
	Secrets []QuadletMoveSecretOutcome `json:"secrets"`
}

// parseQuadletSecrets maps the shared transfer kit's quadlet reference
// parse (§19, issue 242) onto the preview's wire rows: the kit owns the
// `Secret=` line ritual — the same parse the secrets move preview reads —
// and this maps its kind labels onto the preview's vocabulary. The scan
// is type-generic — podman honors the key in .container and .build files
// alike, and other resource types simply carry no `Secret=` lines. A
// `Secret=` line that resolves to no name (an empty value, or an
// all-options token without `source=`) yields no entry. The result is
// file-ordered and never nil, so a quadlet without `Secret=` lines
// renders as an empty JSON array.
func parseQuadletSecrets(content string) []QuadletMovePreviewSecret {
	parsed := transfer.ParseQuadletSecrets(content)
	secrets := make([]QuadletMovePreviewSecret, 0, len(parsed))
	for _, p := range parsed {
		kind := MovePreviewSecretKindSecret
		if p.Kind == transfer.QuadletRefKindHostPath {
			kind = MovePreviewSecretKindHostPath
		}
		secrets = append(secrets, QuadletMovePreviewSecret{Name: p.Name, Kind: kind})
	}
	return secrets
}
