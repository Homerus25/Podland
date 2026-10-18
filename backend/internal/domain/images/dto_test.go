package images

import (
	"testing"
	"time"

	v1 "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/stretchr/testify/assert"
	"go.podman.io/podman/v6/pkg/domain/entities"
	"go.podman.io/podman/v6/pkg/inspect"
)

// TestFromImageInspectReport_FullReportMapsEveryField pins the mapping
// contract: the list fields come from the report's ImageData, the inspect
// fields from its Config, env is parsed from KEY=VALUE strings, and
// createdAt is RFC3339 UTC.
func TestFromImageInspectReport_FullReportMapsEveryField(t *testing.T) {
	created := time.Date(2026, 10, 18, 12, 0, 0, 0, time.UTC)
	report := &entities.ImageInspectReport{ImageData: &inspect.ImageData{
		ID:           "sha256:abc123",
		RepoTags:     []string{"nginx:latest", "nginx:1.27"},
		RepoDigests:  []string{"nginx@sha256:digest1"},
		Created:      &created,
		Size:         102400000,
		Os:           "linux",
		Architecture: "amd64",
		Config: &v1.ImageConfig{
			User:       "nginx",
			Env:        []string{"PATH=/usr/bin", "NGINX_VERSION=1.27", "EMPTY="},
			Entrypoint: []string{"/docker-entrypoint.sh"},
			Cmd:        []string{"nginx", "-g", "daemon off;"},
			Labels:     map[string]string{"maintainer": "NGINX"},
		},
	}}

	got := FromImageInspectReport(report)

	assert.Equal(t, ImageDetailResponse{
		ID:           "sha256:abc123",
		RepoTags:     []string{"nginx:latest", "nginx:1.27"},
		RepoDigests:  []string{"nginx@sha256:digest1"},
		CreatedAt:    "2026-10-18T12:00:00Z",
		Size:         102400000,
		OS:           "linux",
		Architecture: "amd64",
		User:         "nginx",
		Env:          map[string]string{"PATH": "/usr/bin", "NGINX_VERSION": "1.27", "EMPTY": ""},
		Entrypoint:   []string{"/docker-entrypoint.sh"},
		Cmd:          []string{"nginx", "-g", "daemon off;"},
		Labels:       map[string]string{"maintainer": "NGINX"},
	}, got)
}

// TestFromImageInspectReport_NilReportAnswersZero pins the nil case: a
// nil report maps to the zero DTO, never a panic.
func TestFromImageInspectReport_NilReportAnswersZero(t *testing.T) {
	assert.Equal(t, ImageDetailResponse{}, FromImageInspectReport(nil))
}

// TestFromImageInspectReport_NilImageDataAnswersZero pins the empty
// case: a report without its embedded ImageData maps to the zero DTO.
func TestFromImageInspectReport_NilImageDataAnswersZero(t *testing.T) {
	assert.Equal(t, ImageDetailResponse{}, FromImageInspectReport(&entities.ImageInspectReport{}))
}

// TestFromImageInspectReport_EmptyReportAnswersZero pins the empty
// case: a report with an empty ImageData (no Config, no Created) maps to
// the zero DTO — nil slices and maps stay nil so the optional fields
// omit from the wire.
func TestFromImageInspectReport_EmptyReportAnswersZero(t *testing.T) {
	got := FromImageInspectReport(&entities.ImageInspectReport{ImageData: &inspect.ImageData{}})

	assert.Equal(t, ImageDetailResponse{}, got)
	assert.Nil(t, got.RepoTags)
	assert.Nil(t, got.Env)
	assert.Nil(t, got.Labels)
	assert.Nil(t, got.Entrypoint)
	assert.Nil(t, got.Cmd)
}

// TestFromImageInspectReport_EnvParsing pins the KEY=VALUE parse: the
// first "=" splits key from value, an empty value is kept, and an entry
// without "=" is skipped.
func TestFromImageInspectReport_EnvParsing(t *testing.T) {
	report := &entities.ImageInspectReport{ImageData: &inspect.ImageData{
		Config: &v1.ImageConfig{
			Env: []string{"PATH=/usr/bin", "FLAGS=--a=1", "EMPTY=", "MALFORMED"},
		},
	}}

	got := FromImageInspectReport(report)

	assert.Equal(t, map[string]string{
		"PATH":  "/usr/bin",
		"FLAGS": "--a=1",
		"EMPTY": "",
	}, got.Env)
}

// TestFromImageInspectReport_NilConfigOmitsInspectFields pins that a
// report without a Config still maps the list fields and leaves the
// inspect fields zero.
func TestFromImageInspectReport_NilConfigOmitsInspectFields(t *testing.T) {
	created := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	report := &entities.ImageInspectReport{ImageData: &inspect.ImageData{
		ID:      "sha256:abc123",
		Created: &created,
		Size:    42,
	}}

	got := FromImageInspectReport(report)

	assert.Equal(t, "sha256:abc123", got.ID)
	assert.Equal(t, "2026-01-02T03:04:05Z", got.CreatedAt)
	assert.Equal(t, int64(42), got.Size)
	assert.Empty(t, got.User)
	assert.Nil(t, got.Env)
	assert.Nil(t, got.Entrypoint)
	assert.Nil(t, got.Cmd)
	assert.Nil(t, got.Labels)
}
