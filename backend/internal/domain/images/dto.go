package images

import (
	"strings"
	"time"

	"go.podman.io/podman/v6/pkg/domain/entities"
)

// ImagePullRequest represents a request to pull an image from a registry
type ImagePullRequest struct {
	// Image reference to pull (required)
	Image string `json:"image" validate:"required,min=1"`
	// Credentials for registry authentication (optional)
	Credentials *ImagePullCredentials `json:"credentials,omitempty"`
	// Platform for the image (optional, e.g., "linux/amd64")
	Platform string `json:"platform,omitempty"`
}

// ImagePullCredentials represents authentication credentials for pulling an image
type ImagePullCredentials struct {
	// Username for registry authentication
	Username string `json:"username,omitempty"`
	// Password for registry authentication
	Password string `json:"password,omitempty"`
}

// ImagePullResponse represents the response after pulling an image
type ImagePullResponse struct {
	// List of pulled image references
	Images []string `json:"images"`
}

// FromImagePullReport converts a podman ImagePullReport to ImagePullResponse
func FromImagePullReport(report *entities.ImagePullReport) ImagePullResponse {
	if report == nil || len(report.Images) == 0 {
		return ImagePullResponse{Images: []string{}}
	}
	return ImagePullResponse{Images: report.Images}
}

// ImageListResponse represents an image in the API list response.
// This DTO is used to serialize image information for the API.
type ImageListResponse struct {
	// Unique identifier for the image
	ID string `json:"id"`
	// Repository tags associated with the image
	RepoTags []string `json:"repoTags"`
	// Repository digests associated with the image
	RepoDigests []string `json:"repoDigests"`
	// Parent image ID
	ParentID string `json:"parentID,omitempty"`
	// Time when the image was created (ISO8601)
	CreatedAt string `json:"createdAt"`
	// Size of the image in bytes
	Size int64 `json:"size"`
	// Operating system (optional, Podman extension)
	OS string `json:"os,omitempty"`
	// Architecture (optional, Podman extension)
	Architecture string `json:"architecture,omitempty"`
	// Labels associated with the image (optional)
	Labels map[string]string `json:"labels,omitempty"`
}

// FromImageSummary converts a podman entities.ImageSummary to ImageListResponse.
func FromImageSummary(img *entities.ImageSummary) ImageListResponse {
	if img == nil {
		return ImageListResponse{}
	}
	return ImageListResponse{
		ID:           img.ID,
		RepoTags:     img.RepoTags,
		RepoDigests:  img.RepoDigests,
		ParentID:     img.ParentId,
		CreatedAt:    formatImageCreatedTime(img.Created),
		Size:         img.Size,
		OS:           img.Os,
		Architecture: img.Arch,
		Labels:       img.Labels,
	}
}

// ToImageListResponse converts a slice of podman ImageSummary to a slice of ImageListResponse.
func ToImageListResponse(images []*entities.ImageSummary) []ImageListResponse {
	result := make([]ImageListResponse, len(images))
	for i, img := range images {
		result[i] = FromImageSummary(img)
	}
	return result
}

// formatImageCreatedTime converts a Unix timestamp to an ISO8601 string.
func formatImageCreatedTime(created int64) string {
	if created == 0 {
		return ""
	}
	return time.Unix(created, 0).UTC().Format(time.RFC3339)
}

// ParsePlatform parses a platform string (e.g., "linux/amd64") into OS, Arch, and Variant
func ParsePlatform(platform string) (os, arch, variant string) {
	if platform == "" {
		return "", "", ""
	}
	parts := strings.Split(platform, "/")
	if len(parts) >= 1 {
		os = parts[0]
	}
	if len(parts) >= 2 {
		arch = parts[1]
	}
	if len(parts) >= 3 {
		variant = parts[2]
	}
	return os, arch, variant
}
