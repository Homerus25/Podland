package hosts

import (
	"context"
	"errors"

	"go.podman.io/podman/v6/pkg/bindings/system"
)

// fetchRemoteVersion is the production versionFetcher: it asks the remote
// Podman service API for its /version report over the handed connection
// context and answers the server component's Version and APIVersion. This
// file is the hosts package's only podman-bindings import (the
// per-domain engine.go ruling).
func fetchRemoteVersion(ctx context.Context) (versionReport, error) {
	report, err := system.Version(ctx, nil)
	if err != nil {
		return versionReport{}, err
	}
	if report.Server == nil {
		return versionReport{}, errors.New("version report carries no server component")
	}
	return versionReport{
		PodmanVersion: report.Server.Version,
		APIVersion:    report.Server.APIVersion,
	}, nil
}
