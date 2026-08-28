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

// formatSecretTime renders a podman timestamp as an ISO8601 string,
// yielding "" for the zero time (mirroring the images DTO convention).
func formatSecretTime(ts time.Time) string {
	if ts.IsZero() {
		return ""
	}
	return ts.UTC().Format(time.RFC3339)
}
