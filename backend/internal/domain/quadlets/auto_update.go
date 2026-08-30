package quadlets

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/Homerus25/podland/backend/internal/sshexec"
)

// Auto-update per-container outcome vocabulary reported to clients.
const (
	AutoUpdateItemUpdated    = "updated"
	AutoUpdateItemFailed     = "failed"
	AutoUpdateItemPending    = "pending"
	AutoUpdateItemUpToDate   = "up-to-date"
	AutoUpdateCompleted      = "completed"
	AutoUpdateWithFailures   = "completed-with-failures"
	AutoUpdateDryRun         = "dry-run"
	AutoUpdateUnparsable     = "unparsable-report"
	AutoUpdateEventTypeLog   = "log"
	AutoUpdateEventTypeDone  = "complete"
	AutoUpdateEventTypeError = "error"
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

// AutoUpdateEvent is one NDJSON event in the auto-update stream.
type AutoUpdateEvent struct {
	OperationID string            `json:"operationId"`
	Type        string            `json:"type"`
	MachineID   string            `json:"machineId"`
	Stream      string            `json:"stream,omitempty"`
	Line        string            `json:"line,omitempty"`
	Report      *AutoUpdateReport `json:"report,omitempty"`
	Error       *AutoUpdateError  `json:"error,omitempty"`
	Timestamp   string            `json:"timestamp"`
}

// NewAutoUpdateLogEvent wraps one streamed remote output line as a log event.
func NewAutoUpdateLogEvent(operationID, machineID string, line sshexec.OutputLine) AutoUpdateEvent {
	return AutoUpdateEvent{
		OperationID: operationID,
		Type:        AutoUpdateEventTypeLog,
		MachineID:   machineID,
		Stream:      string(line.Stream),
		Line:        line.Text,
		Timestamp:   time.Now().UTC().Format(time.RFC3339),
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
// the streaming SSH seam, forwarding each output line as it arrives. A
// non-zero exit code is data (failed per-container entries), not an error;
// only an SSH-level failure (dial/session/cancellation) returns an error.
func (s *QuadletService) AutoUpdate(ctx context.Context, userID, machineID string, dryRun bool, onLine func(sshexec.OutputLine)) (AutoUpdateReport, error) {
	args := []string{"auto-update", "--format", "json"}
	if dryRun {
		args = append(args, "--dry-run")
	}

	result, err := s.sshExec.ExecStream(ctx, machineID, userID, "podman", sshexec.ExecOptions{Args: args}, onLine)
	if err != nil {
		return AutoUpdateReport{}, err
	}

	return parseAutoUpdateReport(result.Stdout, result.ExitCode, dryRun), nil
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
