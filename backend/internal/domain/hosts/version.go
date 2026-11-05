package hosts

import (
	"context"
	"log/slog"

	"github.com/Homerus25/podland/backend/internal/contextutils"
)

// versionReport is the remote Podman's /version answer reduced to the two
// fields the host header shows: the server component's Version and
// APIVersion.
type versionReport struct {
	PodmanVersion string
	APIVersion    string
}

// versionFetcher asks a live podman connection context for the remote
// service's version report. Production wires fetchRemoteVersion (engine.go,
// the package's only podman-bindings import); tests swap the service field
// (the connections.Manager dial seam pattern).
type versionFetcher func(ctx context.Context) (versionReport, error)

// GetHostVersion asks the remote Podman service API for its /version
// report over the cached per-host connection (the same acquisition seam
// as every other host-scoped podman call) and answers the server
// component's Version and APIVersion. Access rules match
// GetHostByID (404 for unknown host, 403 without access; read level
// suffices).
//
// Unavailable is a valid answer, not an error: any failure — connection
// acquisition or the version call itself — answers nil for both fields.
// The failure is logged server-side (with the requestId when present) and
// never returned to the caller, because connection-level errors describe
// the owner's infrastructure and hosts can be shared with read-only
// users (the status endpoint's rule).
//
// The log below fires for acquisition failures too, although the
// connection seam already warn-logs those (its standard cause log). That
// is deliberate: the issue's contract requires a request-ID-carrying
// record for every failure, and the seam's log carries none — so this is
// the one endpoint whose service logs an error the seam has already
// logged, not silent drift from the "service call sites never log the
// acquisition error" rule.
func (s *HostService) GetHostVersion(ctx context.Context, userID, hostID string) (*string, *string, error) {
	if _, err := s.GetHostByID(userID, hostID); err != nil {
		return nil, nil, err
	}

	var report versionReport
	err := s.conn.WithConnection(ctx, userID, hostID, "hosts.GetVersion", func(connCtx context.Context) error {
		var fetchErr error
		report, fetchErr = s.fetchVersion(connCtx)
		return fetchErr
	})
	if err != nil {
		slog.ErrorContext(ctx, "host version fetch failed",
			"error", err,
			"hostID", hostID,
			"userID", userID,
			"requestID", contextutils.GetRequestID(ctx))
		return nil, nil, nil
	}
	return &report.PodmanVersion, &report.APIVersion, nil
}
