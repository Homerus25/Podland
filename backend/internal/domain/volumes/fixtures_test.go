package volumes

import (
	"time"

	"go.podman.io/podman/v6/libpod/define"
	"go.podman.io/podman/v6/pkg/domain/entities"
)

func mustParseCreatedAt(value string) time.Time {
	created, err := time.Parse(time.RFC3339, value)
	if err != nil {
		panic("bad fixture timestamp: " + err.Error())
	}
	return created
}

func namedVolumeFixture(createdAt string) entities.VolumeConfigResponse {
	return entities.VolumeConfigResponse{InspectVolumeData: define.InspectVolumeData{
		Name:       "my-volume",
		Driver:     "local",
		CreatedAt:  mustParseCreatedAt(createdAt),
		Mountpoint: "/var/lib/containers/storage/volumes/my-volume/_data",
		Labels:     map[string]string{"app": "test"},
		Options:    map[string]string{"type": "tmpfs"},
	}}
}

func anonymousVolumeFixture() entities.VolumeConfigResponse {
	return entities.VolumeConfigResponse{InspectVolumeData: define.InspectVolumeData{
		Name:      "9f2a8b1c3d4e5f6a7b8c9d0e1f2a3b4c5d6e7f8a9b0c1d2e3f4a5b6c7d8e9f0a",
		Driver:    "local",
		CreatedAt: mustParseCreatedAt("2026-08-30T11:00:00Z"),
		Anonymous: true,
	}}
}

