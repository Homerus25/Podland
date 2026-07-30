package images

import (
	"net/http"
)

// ImageHandler handles image requests
type ImageHandler struct {
	service        ImageServiceInterface
	machineService any
}

// NewImageHandler creates a new ImageHandler
func NewImageHandler(service ImageServiceInterface, machineService any) *ImageHandler {
	return &ImageHandler{
		service:        service,
		machineService: machineService,
	}
}

// List handles listing images
func (h *ImageHandler) List(w http.ResponseWriter, r *http.Request) {
	http.Error(w, "Not implemented", http.StatusNotImplemented)
}

// Pull handles pulling an image
func (h *ImageHandler) Pull(w http.ResponseWriter, r *http.Request) {
	http.Error(w, "Not implemented", http.StatusNotImplemented)
}

// Delete handles deleting an image
func (h *ImageHandler) Delete(w http.ResponseWriter, r *http.Request) {
	http.Error(w, "Not implemented", http.StatusNotImplemented)
}
