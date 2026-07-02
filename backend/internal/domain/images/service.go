package images

import (
	"github.com/yourorg/podland/backend/internal/connections"
)

// ImageService provides business logic for images
type ImageService struct {
	connManager *connections.ConnectionManager
}

// NewImageService creates a new ImageService
func NewImageService(connManager *connections.ConnectionManager) *ImageService {
	return &ImageService{
		connManager: connManager,
	}
}
