package volumes

import (
	"time"

	"go.podman.io/podman/v6/pkg/domain/entities"
	"go.podman.io/podman/v6/pkg/domain/entities/reports"
)

// VolumeResponse represents a volume in the list API response
type VolumeResponse struct {
	// Volume name
	Name string `json:"name"`
	// Volume driver
	Driver string `json:"driver"`
	// Time when the volume was created (RFC3339)
	CreatedAt string `json:"createdAt"`
	// Whether the volume was created anonymously
	Anonymous bool `json:"anonymous"`
	// User-defined key/value metadata; always present, {} when empty
	// (no omitempty: the frontend renders the map unconditionally)
	Labels map[string]string `json:"labels"`
	// Driver options used when creating the volume; always present, {} when empty
	Options map[string]string `json:"options"`
	// Mountpoint of the volume on the machine
	Mountpoint string `json:"mountpoint"`
	// Size/usage, when podman reports it
	Size *int64 `json:"size,omitempty"`
}

// FromPodmanVolume converts a podman VolumeListReport to the API response
func FromPodmanVolume(report *entities.VolumeListReport) VolumeResponse {
	if report == nil {
		return VolumeResponse{}
	}
	response := VolumeResponse{
		Name:       report.Name,
		Driver:     report.Driver,
		CreatedAt:  report.CreatedAt.UTC().Format(time.RFC3339),
		Anonymous:  report.Anonymous,
		Labels:     report.Labels,
		Options:    report.Options,
		Mountpoint: report.Mountpoint,
	}
	if response.Labels == nil {
		response.Labels = map[string]string{}
	}
	if response.Options == nil {
		response.Options = map[string]string{}
	}
	return response
}

// VolumeCreateRequest is the POST /api/machines/{id}/volumes body.
// Name is optional: an empty name creates an anonymous volume whose
// generated name podman reports back. Name-format validation is podman's
// job — invalid names surface as client errors from the service.
type VolumeCreateRequest struct {
	// Volume name; empty means podman generates one (anonymous volume)
	Name string `json:"name"`
	// User-defined key/value metadata
	Labels map[string]string `json:"labels,omitempty"`
	// Volume driver; podman defaults to its "local" driver
	Driver string `json:"driver,omitempty"`
	// Mapping of driver options and values
	Options map[string]string `json:"options,omitempty"`
}

// ToPodmanVolumeCreateOptions maps the API request to podman's create
// options.
func ToPodmanVolumeCreateOptions(req VolumeCreateRequest) entities.VolumeCreateOptions {
	return entities.VolumeCreateOptions{
		Name:    req.Name,
		Labels:  req.Labels,
		Driver:  req.Driver,
		Options: req.Options,
	}
}

// VolumeInspectResponse represents a single volume's full config in the API response
type VolumeInspectResponse struct {
	// Volume name
	Name string `json:"name"`
	// Volume driver
	Driver string `json:"driver"`
	// Time when the volume was created (RFC3339)
	CreatedAt string `json:"createdAt"`
	// Whether the volume was created anonymously
	Anonymous bool `json:"anonymous"`
	// User-defined key/value metadata; always present, {} when empty
	// (no omitempty: the frontend renders the map unconditionally)
	Labels map[string]string `json:"labels"`
	// Driver options used when creating the volume; always present, {} when empty
	Options map[string]string `json:"options"`
	// Mountpoint of the volume on the machine
	Mountpoint string `json:"mountpoint"`
	// Size/usage, when podman reports it
	Size *int64 `json:"size,omitempty"`
}

// FromPodmanVolumeInspect converts a podman VolumeInspectReport to the API response
func FromPodmanVolumeInspect(report *entities.VolumeInspectReport) VolumeInspectResponse {
	if report == nil || report.VolumeConfigResponse == nil {
		return VolumeInspectResponse{}
	}
	cfg := report.VolumeConfigResponse
	response := VolumeInspectResponse{
		Name:       cfg.Name,
		Driver:     cfg.Driver,
		CreatedAt:  cfg.CreatedAt.UTC().Format(time.RFC3339),
		Anonymous:  cfg.Anonymous,
		Labels:     cfg.Labels,
		Options:    cfg.Options,
		Mountpoint: cfg.Mountpoint,
	}
	if response.Labels == nil {
		response.Labels = map[string]string{}
	}
	if response.Options == nil {
		response.Options = map[string]string{}
	}
	return response
}

// ToVolumeListResponse converts podman volume reports to the API array
func ToVolumeListResponse(reports []*entities.VolumeListReport) []VolumeResponse {
	out := make([]VolumeResponse, 0, len(reports))
	for _, report := range reports {
		out = append(out, FromPodmanVolume(report))
	}
	return out
}

// VolumePruneResponse is one entry in the prune API response: the
// removed volume's name and the reclaimed size (bytes), plus any
// per-volume error podman reported while removing it.
type VolumePruneResponse struct {
	// Name of the removed volume
	Name string `json:"name"`
	// Reclaimed size in bytes, when podman reports it
	Size uint64 `json:"size"`
	// Per-volume error podman reported, if the removal failed
	Error string `json:"error,omitempty"`
}

// FromPodmanPruneReports converts podman's prune reports to the API array.
func FromPodmanPruneReports(reports []*reports.PruneReport) []VolumePruneResponse {
	out := make([]VolumePruneResponse, 0, len(reports))
	for _, report := range reports {
		if report == nil {
			continue
		}
		entry := VolumePruneResponse{
			Name: report.Id,
			Size: report.Size,
		}
		if report.Err != nil {
			entry.Error = report.Err.Error()
		}
		out = append(out, entry)
	}
	return out
}

// Mode labels for the volume copy/move endpoint (issue 238): "copy"
// creates the target volume and carries the data; "move" additionally
// removes the source volume after the data landed.
const (
	VolumeMoveModeCopy = "copy"
	VolumeMoveModeMove = "move"
)

// Transfer status labels for the volume copy/move endpoint (issue 238):
// copy answers "copied"; move answers "moved", or "partial" when the data
// landed but the source removal failed (the volume then exists on both
// machines).
const (
	VolumeMoveStatusCopied  = "copied"
	VolumeMoveStatusMoved   = "moved"
	VolumeMoveStatusPartial = "partial"
)

// VolumeMoveRequest is the body of the cross-machine volume copy/move
// endpoint (POST .../volumes/{volumeName}/move, issue 238). The name is
// preserved: the target volume carries the source's name, no rename, no
// within-machine duplicate.
type VolumeMoveRequest struct {
	// TargetMachineID is the machine the volume is transferred to. Required.
	TargetMachineID string `json:"targetMachineId" validate:"required,min=1"`
	// Mode selects the transfer semantics. "copy" creates the target
	// volume and carries the data; "move" additionally removes the source
	// after the data landed. Required.
	Mode string `json:"mode" validate:"required,oneof=copy move"`
	// Force removes the source volume even while a container uses it —
	// the delete flow's force, passed through to podman's remove. Move
	// mode only; copy ignores the flag. Defaults to false, so an in-use
	// source volume refuses the move (VOLUME_IN_USE) unless force is set.
	Force bool `json:"force,omitempty"`
}

// VolumeMoveResponse is the transfer envelope both modes answer with
// (issue 238). Status carries the outcome: "copied" for a completed copy,
// "moved" for a completed move, and "partial" when a move's data landed
// but the source removal failed — the volume then exists on both
// machines, and Error carries the removal failure's detail.
type VolumeMoveResponse struct {
	// Name of the transferred volume (identical on both machines — the
	// transfer carries no rename).
	Name string `json:"name"`
	// Status of the transfer: "copied", "moved", or "partial".
	Status string `json:"status"`
	// SourceMachineID is the machine the volume was transferred from.
	SourceMachineID string `json:"sourceMachineId"`
	// TargetMachineID is the machine the volume was created on.
	TargetMachineID string `json:"targetMachineId"`
	// Bytes is the volume data actually streamed across machines.
	Bytes int64 `json:"bytes"`
	// Error details why the source removal failed when Status is
	// "partial" — the volume exists on both machines and the source side
	// must be cleaned up manually. Empty otherwise.
	Error string `json:"error,omitempty"`
}

// VolumeMovePreviewResponse answers everything the copy/move dialog must
// show before confirming (GET .../volumes/{volumeName}/move-preview,
// issue 238): whether the target machine already holds a same-named
// volume (a collision blocks the confirm), whether a container on the
// source machine uses the volume (a move needs force), and which source
// quadlets reference it (a warning — the confirm stays allowed).
type VolumeMovePreviewResponse struct {
	// Collision reports a same-named volume on the target machine. The
	// transfer never overwrites: the backend re-checks at execution time
	// and answers 409 VOLUME_ALREADY_EXISTS.
	Collision bool `json:"collision"`
	// InUse reports a container on the SOURCE machine mounting the
	// volume. A move refuses with 409 VOLUME_IN_USE unless force is set;
	// a copy proceeds regardless.
	InUse bool `json:"inUse"`
	// ReferencingQuadlets lists the source machine's quadlet files whose
	// `Volume=` lines resolve to the volume — a warning, never a block.
	ReferencingQuadlets []VolumeMoveQuadletRef `json:"referencingQuadlets"`
}

// VolumeMoveQuadletRef names one source quadlet file referencing the
// volume (issue 238).
type VolumeMoveQuadletRef struct {
	// Name is the quadlet's file name, extension included.
	Name string `json:"name"`
}

// toVolumeMoveQuadletRefs maps the shared transfer kit's quadlet scan
// result onto the preview's wire rows (§19, issue 259): the kit owns the
// list → print → match ritual and answers file-ordered quadlet file
// names, never nil; this maps them onto the wire DTO. The result is
// never nil, so a machine without referencing quadlets renders an empty
// JSON array, not null.
func toVolumeMoveQuadletRefs(names []string) []VolumeMoveQuadletRef {
	refs := make([]VolumeMoveQuadletRef, 0, len(names))
	for _, name := range names {
		refs = append(refs, VolumeMoveQuadletRef{Name: name})
	}
	return refs
}

// Stream-event type labels for the volume copy/move's NDJSON transport
// (issue 238): progress events, then exactly one terminal event — the
// transfer envelope on success, the mapped error otherwise. The quadlet
// move's vocabulary (issue 214).
const (
	VolumeMoveEventTypeVolume   = "volume"
	VolumeMoveEventTypeComplete = "complete"
	VolumeMoveEventTypeError    = "error"
)

// Progress phase labels for VolumeMoveEvent.Phase (issue 238): "start"
// when the data stream begins, "bytes" as the counting reader crosses
// its reporting increment, "done" when the import finished.
const (
	VolumeMovePhaseStart = "start"
	VolumeMovePhaseBytes = "bytes"
	VolumeMovePhaseDone  = "done"
)

// VolumeMoveError carries an error code/message in a terminal move event.
type VolumeMoveError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// VolumeMoveEvent is one NDJSON event in the volume copy/move stream
// (issue 238). Progress events carry Phase (plus Bytes where the phase
// has it); the terminal complete event carries the transfer envelope;
// the terminal error event carries the mapped code/message. All events
// share operationId, machineId, and timestamp — the quadlet move event
// shape (issue 214).
type VolumeMoveEvent struct {
	OperationID string              `json:"operationId"`
	Type        string              `json:"type"`
	MachineID   string              `json:"machineId"`
	Volume      string              `json:"volume,omitempty"`
	Phase       string              `json:"phase,omitempty"`
	Bytes       int64               `json:"bytes,omitempty"`
	Result      *VolumeMoveResponse `json:"result,omitempty"`
	Error       *VolumeMoveError    `json:"error,omitempty"`
	Timestamp   string              `json:"timestamp"`
}

// NewVolumeMoveProgressEvent wraps one data-phase progress event.
func NewVolumeMoveProgressEvent(operationID, machineID, volume, phase string, bytes int64) VolumeMoveEvent {
	return VolumeMoveEvent{
		OperationID: operationID,
		Type:        VolumeMoveEventTypeVolume,
		MachineID:   machineID,
		Volume:      volume,
		Phase:       phase,
		Bytes:       bytes,
		Timestamp:   time.Now().UTC().Format(time.RFC3339),
	}
}

// NewVolumeMoveCompleteEvent builds the terminal complete event carrying
// the transfer envelope.
func NewVolumeMoveCompleteEvent(operationID, machineID string, result VolumeMoveResponse) VolumeMoveEvent {
	return VolumeMoveEvent{
		OperationID: operationID,
		Type:        VolumeMoveEventTypeComplete,
		MachineID:   machineID,
		Result:      &result,
		Timestamp:   time.Now().UTC().Format(time.RFC3339),
	}
}

// NewVolumeMoveErrorEvent builds the terminal error event.
func NewVolumeMoveErrorEvent(operationID, machineID, code, message string) VolumeMoveEvent {
	return VolumeMoveEvent{
		OperationID: operationID,
		Type:        VolumeMoveEventTypeError,
		MachineID:   machineID,
		Error:       &VolumeMoveError{Code: code, Message: message},
		Timestamp:   time.Now().UTC().Format(time.RFC3339),
	}
}
