package images

import (
	"time"

	"go.podman.io/podman/v6/pkg/domain/entities"
)

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
