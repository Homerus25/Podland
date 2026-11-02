package quadlets

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"go.podman.io/podman/v6/pkg/domain/entities"

	"github.com/Homerus25/podland/backend/internal/sshexec"
)

// Auto-update per-container outcome vocabulary reported to clients.
const (
	AutoUpdateItemUpdated      = "updated"
	AutoUpdateItemFailed       = "failed"
	AutoUpdateItemPending      = "pending"
	AutoUpdateItemUpToDate     = "up-to-date"
	AutoUpdateCompleted        = "completed"
	AutoUpdateWithFailures     = "completed-with-failures"
	AutoUpdateDryRun           = "dry-run"
	AutoUpdateUnparsable       = "unparsable-report"
	AutoUpdateEventTypePlan    = "plan"
	AutoUpdateEventTypePulling = "pulling"
	AutoUpdateEventTypeLog     = "log"
	AutoUpdateEventTypeDone    = "complete"
	AutoUpdateEventTypeError   = "error"
)

// AutoUpdateItem is one container's outcome in an auto-update run.
type AutoUpdateItem struct {
	Unit          string `json:"unit,omitempty"`
	ContainerName string `json:"containerName"`
	ImageName     string `json:"imageName"`
	Updated       string `json:"updated"`
	Error         string `json:"error,omitempty"`
}

// AutoUpdateReport summarises a completed auto-update run.
type AutoUpdateReport struct {
	Containers []AutoUpdateItem `json:"containers"`
	Overall    string           `json:"overall"`
	ExitCode   int              `json:"exitCode"`
}

// AutoUpdateError carries an error code/message in a terminal error event.
type AutoUpdateError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// AutoUpdateEvent is one NDJSON event in the auto-update stream. The
// service builds the plan, pulling, and log events unstamped (the
// handler stamps the operation and machine identities on each, the move
// stream's arrangement); the complete and error events are built
// handler-side already stamped. The plan, unit, and image fields belong
// to the plan and pulling events (issue 448) and stay absent on the
// log, complete, and error events.
type AutoUpdateEvent struct {
	OperationID string               `json:"operationId"`
	Type        string               `json:"type"`
	MachineID   string               `json:"machineId"`
	Stream      string               `json:"stream,omitempty"`
	Line        string               `json:"line,omitempty"`
	Plan        []AutoUpdatePlanItem `json:"plan,omitempty"`
	Unit        string               `json:"unit,omitempty"`
	Image       string               `json:"image,omitempty"`
	Report      *AutoUpdateReport    `json:"report,omitempty"`
	Error       *AutoUpdateError     `json:"error,omitempty"`
	Timestamp   string               `json:"timestamp"`
}

// newAutoUpdatePlanEvent builds the run's opening plan event: every
// participating unit with its image, narrowed to the filter on a
// filtered run. The plan is never nil, so the event always carries a
// list.
func newAutoUpdatePlanEvent(plan []AutoUpdatePlanItem) AutoUpdateEvent {
	if plan == nil {
		plan = []AutoUpdatePlanItem{}
	}
	return AutoUpdateEvent{
		Type:      AutoUpdateEventTypePlan,
		Plan:      plan,
		Timestamp: time.Now().UTC().Format(time.RFC3339),
	}
}

// newAutoUpdatePullingEvent reports one planned unit entering the
// pulling state: podman's output named its image.
func newAutoUpdatePullingEvent(unit, image string) AutoUpdateEvent {
	return AutoUpdateEvent{
		Type:      AutoUpdateEventTypePulling,
		Unit:      unit,
		Image:     image,
		Timestamp: time.Now().UTC().Format(time.RFC3339),
	}
}

// newAutoUpdateLogEvent wraps one streamed remote output line as a log
// event.
func newAutoUpdateLogEvent(line sshexec.OutputLine) AutoUpdateEvent {
	return AutoUpdateEvent{
		Type:      AutoUpdateEventTypeLog,
		Stream:    string(line.Stream),
		Line:      line.Text,
		Timestamp: time.Now().UTC().Format(time.RFC3339),
	}
}

// NewAutoUpdateCompleteEvent builds the terminal complete event carrying the
// parsed report.
func NewAutoUpdateCompleteEvent(operationID, machineID string, report AutoUpdateReport) AutoUpdateEvent {
	return AutoUpdateEvent{
		OperationID: operationID,
		Type:        AutoUpdateEventTypeDone,
		MachineID:   machineID,
		Report:      &report,
		Timestamp:   time.Now().UTC().Format(time.RFC3339),
	}
}

// NewAutoUpdateErrorEvent builds the terminal error event.
func NewAutoUpdateErrorEvent(operationID, machineID, code, message string) AutoUpdateEvent {
	return AutoUpdateEvent{
		OperationID: operationID,
		Type:        AutoUpdateEventTypeError,
		MachineID:   machineID,
		Error:       &AutoUpdateError{Code: code, Message: message},
		Timestamp:   time.Now().UTC().Format(time.RFC3339),
	}
}

// rawAutoUpdateItem mirrors podman auto-update --format json entries.
type rawAutoUpdateItem struct {
	Unit          string `json:"Unit"`
	ContainerName string `json:"ContainerName"`
	ImageName     string `json:"ImageName"`
	Updated       string `json:"Updated"`
	Error         string `json:"Error"`
}

// AutoUpdate runs `podman auto-update --format json` on the machine through
// the streaming SSH seam, emitting the run's events as they occur: a plan
// event naming every participating unit before the run starts, one log
// event per remote output line, and a pulling event ahead of each log line
// whose "Trying to pull <image>" image matches a planned unit (issue 448).
// A non-zero exit code is data (failed per-container entries), not an error;
// only an SSH-level failure (dial/session/cancellation) returns an error.
// A non-empty filter is forwarded as podman's positional unit-name argument
// and narrows the plan to that unit; the value rides the exec args
// unvalidated, so a bogus unit surfaces podman's own error through the
// stream.
func (s *QuadletService) AutoUpdate(ctx context.Context, userID, machineID string, dryRun bool, filter string, onEvent func(AutoUpdateEvent)) (AutoUpdateReport, error) {
	plan := buildAutoUpdatePlan(s.collectAutoUpdatePlanSources(ctx, userID, machineID), filter)
	onEvent(newAutoUpdatePlanEvent(plan))

	args := []string{"auto-update", "--format", "json"}
	if dryRun {
		args = append(args, "--dry-run")
	}
	if filter != "" {
		args = append(args, filter)
	}

	result, err := s.sshExec.ExecStream(ctx, machineID, userID, "podman", sshexec.ExecOptions{Args: args}, func(line sshexec.OutputLine) {
		if image, ok := pullImageFromLine(line.Text); ok {
			if item, matched := matchPullingUnit(plan, image); matched {
				onEvent(newAutoUpdatePullingEvent(item.Unit, image))
			}
		}
		onEvent(newAutoUpdateLogEvent(line))
	})
	if err != nil {
		return AutoUpdateReport{}, err
	}

	return parseAutoUpdateReport(result.Stdout, result.ExitCode, dryRun), nil
}

// collectAutoUpdatePlanSources lists the machine's quadlet files and
// prints each .container file, answering the plan's per-unit sources. An
// unreadable file degrades to a unit-only source (the unit still appears,
// it just cannot be matched mid-run); a file without AutoUpdate=registry
// is podman's own silent skip and is filtered out by the plan build. A
// list-level failure degrades to no sources rather than failing the run:
// the run's own podman invocation is untouched and the report at the end
// still tells the truth.
func (s *QuadletService) collectAutoUpdatePlanSources(ctx context.Context, userID, machineID string) []autoUpdatePlanSource {
	sources := []autoUpdatePlanSource{}
	_ = s.engines.WithEngine(ctx, userID, machineID, "quadlets.AutoUpdatePlan", func(e QuadletEngine) error {
		list, err := e.List(entities.QuadletListOptions{})
		if err != nil {
			return err
		}
		for _, quadlet := range list {
			if quadlet == nil || !strings.HasSuffix(strings.ToLower(quadlet.Path), ".container") {
				continue
			}
			source := autoUpdatePlanSource{Unit: quadletSystemdUnit(quadlet)}
			content, err := e.Print(quadlet.Name)
			if err != nil {
				sources = append(sources, source)
				continue
			}
			source.Content = content
			source.Readable = true
			sources = append(sources, source)
		}
		return nil
	})
	return sources
}

// quadletSystemdUnit answers a .container quadlet's systemd unit name:
// podman's reported UnitName when systemd knows it, else the generated
// name (the file name minus .container plus .service).
func quadletSystemdUnit(quadlet *entities.ListQuadlet) string {
	if quadlet.UnitName != "" {
		return quadlet.UnitName
	}
	return strings.TrimSuffix(quadlet.Name, ".container") + ".service"
}

// parseAutoUpdateReport extracts the trailing JSON array from the command
// output and maps it onto our report vocabulary. Unparsable output is not an
// error: the raw output has already been streamed to the client, so the
// report simply says so.
func parseAutoUpdateReport(stdout string, exitCode int, dryRun bool) AutoUpdateReport {
	report := AutoUpdateReport{
		Overall:  AutoUpdateCompleted,
		ExitCode: exitCode,
	}

	raw := extractJSONReport(stdout)
	if raw == nil {
		report.Overall = AutoUpdateUnparsable
		return report
	}

	anyFailed := false
	for _, r := range raw {
		item := AutoUpdateItem{
			Unit:          r.Unit,
			ContainerName: r.ContainerName,
			ImageName:     r.ImageName,
			Updated:       mapUpdatedStatus(r.Updated, r.Error),
			Error:         r.Error,
		}
		if item.Updated == AutoUpdateItemFailed {
			anyFailed = true
		}
		report.Containers = append(report.Containers, item)
	}

	switch {
	case dryRun:
		report.Overall = AutoUpdateDryRun
	case anyFailed:
		report.Overall = AutoUpdateWithFailures
	}

	return report
}

// extractJSONReport finds the last JSON array in stdout and decodes it.
// podman prints human-readable lines before the JSON report when the output
// mixes both; the trailing array is the machine-readable report.
func extractJSONReport(stdout string) []rawAutoUpdateItem {
	start := strings.LastIndex(stdout, "[")
	end := strings.LastIndex(stdout, "]")
	if start == -1 || end == -1 || end < start {
		return nil
	}

	var items []rawAutoUpdateItem
	if err := json.Unmarshal([]byte(stdout[start:end+1]), &items); err != nil {
		return nil
	}
	return items
}

// mapUpdatedStatus translates podman's Updated values ("true"/"false"/
// "pending") onto our per-container outcome vocabulary. A non-empty Error
// field marks the container as failed.
func mapUpdatedStatus(updated, errText string) string {
	if errText != "" {
		return AutoUpdateItemFailed
	}
	switch strings.ToLower(updated) {
	case "true":
		return AutoUpdateItemUpdated
	case "pending":
		return AutoUpdateItemPending
	default:
		return AutoUpdateItemUpToDate
	}
}

// --- The plan and pulling vocabulary (issue 448) ---

// AutoUpdatePlanItem names one participating unit in the run's plan: the
// unit's systemd name plus the image its .container file declares. A
// unit-only entry (empty Image) is the degradation for a file whose
// Image= is missing or unparsable, or whose content could not be read:
// the unit still appears in the plan, it just cannot be matched mid-run.
type AutoUpdatePlanItem struct {
	Unit  string `json:"unit"`
	Image string `json:"image,omitempty"`
}

// autoUpdatePlanSource is one .container file's contribution to the plan:
// the unit's systemd name, the file's content when it could be printed,
// and whether it could be printed at all.
type autoUpdatePlanSource struct {
	Unit     string
	Content  string
	Readable bool
}

// parseAutoUpdateQuadlet reads one .container file's content for the
// plan: the unit participates when the file carries AutoUpdate=registry,
// and its image is the file's Image= value — empty when the line is
// missing or carries no value (the unit-only degradation). A file
// without the registry policy answers no image: only a participating
// unit carries one.
func parseAutoUpdateQuadlet(content string) (participates bool, image string) {
	autoUpdate := ""
	for _, rawLine := range strings.Split(content, "\n") {
		key, value, found := strings.Cut(strings.TrimSpace(rawLine), "=")
		if !found {
			continue
		}
		switch strings.TrimSpace(key) {
		case "AutoUpdate":
			if autoUpdate == "" {
				autoUpdate = strings.TrimSpace(value)
			}
		case "Image":
			if image == "" {
				image = strings.TrimSpace(value)
			}
		}
	}
	if autoUpdate != "registry" {
		return false, ""
	}
	return true, image
}

// buildAutoUpdatePlan narrows the machine's .container files to the run's
// participating units, in file order: a readable file participates when
// it carries AutoUpdate=registry; an unreadable file degrades to a
// unit-only entry (it cannot be verified, so the unit still appears); a
// readable file without the policy is podman's own silent skip and stays
// out. A non-empty filter narrows the plan to the named unit — the same
// unit name the filter forwards to podman. The answer is never nil, so
// the plan event always carries a list.
func buildAutoUpdatePlan(sources []autoUpdatePlanSource, filter string) []AutoUpdatePlanItem {
	plan := []AutoUpdatePlanItem{}
	for _, source := range sources {
		if !source.Readable {
			plan = append(plan, AutoUpdatePlanItem{Unit: source.Unit})
			continue
		}
		participates, image := parseAutoUpdateQuadlet(source.Content)
		if !participates {
			continue
		}
		plan = append(plan, AutoUpdatePlanItem{Unit: source.Unit, Image: image})
	}
	if filter == "" {
		return plan
	}
	narrowed := []AutoUpdatePlanItem{}
	for _, item := range plan {
		if item.Unit == filter {
			narrowed = append(narrowed, item)
		}
	}
	return narrowed
}

// pullLinePrefix is podman's live-output line naming the image being
// pulled ("Trying to pull <image>...", libimage's pull candidate line).
const pullLinePrefix = "Trying to pull "

// pullImageFromLine extracts the image from a "Trying to pull <image>"
// output line. podman prints the fully-qualified image followed by an
// ellipsis; a line without the prefix, or without an image, is not a
// pull line.
func pullImageFromLine(line string) (string, bool) {
	if !strings.HasPrefix(line, pullLinePrefix) {
		return "", false
	}
	image := strings.TrimSuffix(strings.TrimPrefix(line, pullLinePrefix), "...")
	if image == "" {
		return "", false
	}
	return image, true
}

// matchPullingUnit resolves a pulled image to the planned unit whose
// image it is: exact match first, then suffix — podman prints normalized
// fully-qualified names while quadlet files may carry short names. A
// suffix match counts only when the character preceding the suffix in
// the pulled image is "/" (the registry/repo boundary), so a planned
// "app" matches "docker.io/library/app:latest" but not "myapp:latest".
// A unit-only plan entry never matches (there is no image to match), and
// an image matching no planned unit answers false.
func matchPullingUnit(plan []AutoUpdatePlanItem, image string) (AutoUpdatePlanItem, bool) {
	for _, item := range plan {
		if item.Image != "" && item.Image == image {
			return item, true
		}
	}
	for _, item := range plan {
		if item.Image == "" || !strings.HasSuffix(image, item.Image) {
			continue
		}
		if cut := len(image) - len(item.Image); cut > 0 && image[cut-1] == '/' {
			return item, true
		}
	}
	return AutoUpdatePlanItem{}, false
}
