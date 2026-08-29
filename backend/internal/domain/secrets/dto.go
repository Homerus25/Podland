package secrets

import (
	"time"

	"go.podman.io/podman/v6/pkg/domain/entities"
)

// SecretListResponse represents a secret's metadata in the API list
// response. It deliberately carries NO value field: podman's
// SecretInfoReport.SecretData is dropped in mapping and is only ever
// exposed by the admin-only reveal path (160-02).
type SecretListResponse struct {
	// Unique identifier for the secret
	ID string `json:"id"`
	// Name of the secret
	Name string `json:"name"`
	// Name of the driver that stores the secret (e.g. "file")
	Driver string `json:"driver"`
	// Time when the secret was created (ISO8601)
	CreatedAt string `json:"createdAt"`
	// Time when the secret was last updated (ISO8601)
	UpdatedAt string `json:"updatedAt"`
	// Labels associated with the secret (omitted when empty)
	Labels map[string]string `json:"labels,omitempty"`
}

// ToSecretListResponse converts a slice of podman SecretInfoReport to a
// slice of SecretListResponse. SecretData is never copied: the DTO has no
// field for it, so a value that somehow arrived in a list report cannot
// reach the response.
func ToSecretListResponse(reports []*entities.SecretInfoReport) []SecretListResponse {
	result := make([]SecretListResponse, len(reports))
	for i, report := range reports {
		result[i] = FromSecretInfoReport(report)
	}
	return result
}

// FromSecretInfoReport converts a podman entities.SecretInfoReport to the
// metadata-only SecretListResponse.
func FromSecretInfoReport(report *entities.SecretInfoReport) SecretListResponse {
	if report == nil {
		return SecretListResponse{}
	}

	return SecretListResponse{
		ID:        report.ID,
		Name:      report.Spec.Name,
		Driver:    report.Spec.Driver.Name,
		CreatedAt: formatSecretTime(report.CreatedAt),
		UpdatedAt: formatSecretTime(report.UpdatedAt),
		Labels:    report.Spec.Labels,
	}
}

// SecretInspectResponse is the single-secret inspect response: the list
// DTO's metadata fields plus the value. SecretData is present ONLY on an
// admin reveal — mapping never copies it otherwise, and the omitempty tag
// drops an empty value from the JSON entirely, so a metadata inspect
// answers with no secretData field at all.
type SecretInspectResponse struct {
	SecretListResponse
	// SecretData carries the secret's value. Populated exclusively by the
	// admin reveal path; must never be logged at any level.
	SecretData string `json:"secretData,omitempty"`
}

// ToSecretInspectResponse converts a podman entities.SecretInfoReport to
// the inspect response. SecretData is copied ONLY when includeValue is
// true (the admin reveal path); a metadata inspect cannot leak the value
// because it is simply never copied.
func ToSecretInspectResponse(report *entities.SecretInfoReport, includeValue bool) *SecretInspectResponse {
	resp := &SecretInspectResponse{
		SecretListResponse: FromSecretInfoReport(report),
	}
	if includeValue && report != nil {
		resp.SecretData = report.SecretData
	}
	return resp
}

// SecretCreateRequest is the body of POST /api/machines/{id}/secrets.
//
// The Name rules are podman's secret name rules; the handler trims the
// name before validating (padded-but-valid names must not be rejected for
// their padding). The Value must NEVER be trimmed or otherwise mutated —
// leading/trailing whitespace and newlines are significant — and it is
// never logged at any level.
//
// Note: go-playground/validator has no regex tag, so the name's character
// whitelist (^[a-zA-Z0-9][a-zA-Z0-9_.-]*$) is enforced by an explicit
// check in the handler next to validate.Struct, mirroring the frontend's
// SECRET_NAME_PATTERN.
type SecretCreateRequest struct {
	// Name of the secret. Required, at most 255 characters, must match
	// podman's secret name pattern (letter or digit first, then letters,
	// digits, underscores, dots, dashes).
	Name string `json:"name" validate:"required,max=255"`
	// Value is the secret's raw payload. Required, 1..1048576 BYTES
	// (1 MiB) — enforced as an explicit len() check in the handler
	// because validator's min/max count runes for strings, and podman
	// limits the payload in bytes. Sent byte-identical; never trimmed,
	// never logged.
	Value string `json:"value" validate:"required"`
}

// SecretCreateResponse is the create response: the new secret's podman ID
// only. The value is deliberately absent — it travelled once, in the
// request, and is only ever returned by the admin-only reveal path.
type SecretCreateResponse struct {
	// Unique identifier podman assigned to the created secret
	ID string `json:"id"`
}

// formatSecretTime renders a podman timestamp as an ISO8601 string,
// yielding "" for the zero time (mirroring the images DTO convention).
func formatSecretTime(ts time.Time) string {
	if ts.IsZero() {
		return ""
	}
	return ts.UTC().Format(time.RFC3339)
}
