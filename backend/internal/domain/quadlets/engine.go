package quadlets

import (
	"archive/tar"
	"bytes"
	"context"
	"fmt"
	"io"
	"time"

	"github.com/Homerus25/podland/backend/internal/connections"
	"go.podman.io/podman/v6/pkg/bindings/quadlets"
	"go.podman.io/podman/v6/pkg/domain/entities"
	"go.podman.io/podman/v6/pkg/domain/infra/tunnel"
)

// QuadletEngine is the quadlets domain's view of one machine's podman: the
// exact set of quadlet and managed-container operations the service
// performs, in domain vocabulary (single names, plain flags, raw file
// content) and without a context parameter — the connection context lives
// only inside the production adapter below.
type QuadletEngine interface {
	List(opts entities.QuadletListOptions) ([]*entities.ListQuadlet, error)
	// Print returns the quadlet unit file's content.
	Print(name string) (string, error)
	// Remove uninstalls the quadlet; systemd is always reloaded.
	Remove(name string, force bool) (*entities.QuadletRemoveReport, error)
	// Install installs one quadlet file: the adapter owns the tar framing
	// podman's install primitive requires. replace overwrites an existing
	// installation; systemd is always reloaded.
	Install(fileName string, content []byte, replace bool) (*entities.QuadletInstallReport, error)
	// Logs writes the managed container's logs into the writers carried by
	// opts (follow or one-shot is the caller's choice via opts).
	Logs(containerName string, opts entities.ContainerLogsOptions) error
	InspectContainer(id string) ([]*entities.ContainerInspectReport, []error, error)
	InspectPod(id string) ([]*entities.PodInspectReport, []error, error)
}

// QuadletEngineProvider is the quadlets domain's slice of the connection
// seam: it hands a closure one machine's QuadletEngine. It delegates to
// connections.Manager.WithConnection, which owns acquisition, the
// warn-and-502-sentinel failure contract, and the op tag.
type QuadletEngineProvider interface {
	WithEngine(ctx context.Context, userID, machineID, op string, fn func(e QuadletEngine) error) error
}

// NewQuadletEngineProvider returns the production provider: it wraps the
// connection manager and builds the tunnel engine (plus the bindings-level
// install primitive) inside the connection lease. This file is the only one
// in the package importing the podman tunnel/bindings packages.
func NewQuadletEngineProvider(conn connections.Manager) QuadletEngineProvider {
	return connectionEngines{conn: conn}
}

// connectionEngines is the production QuadletEngineProvider.
type connectionEngines struct {
	conn connections.Manager
}

func (p connectionEngines) WithEngine(ctx context.Context, userID, machineID, op string, fn func(e QuadletEngine) error) error {
	return p.conn.WithConnection(ctx, userID, machineID, op, func(connCtx context.Context) error {
		return fn(&podmanQuadletEngine{
			tunnelEngine: &tunnel.ContainerEngine{ClientCtx: connCtx},
			ctx:          connCtx,
		})
	})
}

// podmanQuadletEngine implements QuadletEngine against one live connection.
type podmanQuadletEngine struct {
	tunnelEngine *tunnel.ContainerEngine
	ctx          context.Context
}

func (e *podmanQuadletEngine) List(opts entities.QuadletListOptions) ([]*entities.ListQuadlet, error) {
	return e.tunnelEngine.QuadletList(e.ctx, opts)
}

func (e *podmanQuadletEngine) Print(name string) (string, error) {
	return e.tunnelEngine.QuadletPrint(e.ctx, name)
}

func (e *podmanQuadletEngine) Remove(name string, force bool) (*entities.QuadletRemoveReport, error) {
	return e.tunnelEngine.QuadletRemove(e.ctx, []string{name}, entities.QuadletRemoveOptions{
		Force:         force,
		ReloadSystemd: true,
	})
}

func (e *podmanQuadletEngine) Install(fileName string, content []byte, replace bool) (*entities.QuadletInstallReport, error) {
	body, err := createTarBody(fileName, content)
	if err != nil {
		return nil, fmt.Errorf("failed to build quadlet tar body: %w", err)
	}

	// Call the podman bindings-level Install directly. The engine-level
	// QuadletInstall only accepts on-disk paths / URLs; we already hold the
	// content in memory, so we skip its path-resolution step.
	options := new(quadlets.QuadletInstallOptions).WithReloadSystemd(true)
	if replace {
		options = options.WithReplace(true)
	}
	return quadlets.Install(e.ctx, options, body)
}

func (e *podmanQuadletEngine) Logs(containerName string, opts entities.ContainerLogsOptions) error {
	return e.tunnelEngine.ContainerLogs(e.ctx, []string{containerName}, opts)
}

func (e *podmanQuadletEngine) InspectContainer(id string) ([]*entities.ContainerInspectReport, []error, error) {
	return e.tunnelEngine.ContainerInspect(e.ctx, []string{id}, entities.InspectOptions{})
}

func (e *podmanQuadletEngine) InspectPod(id string) ([]*entities.PodInspectReport, []error, error) {
	return e.tunnelEngine.PodInspect(e.ctx, []string{id}, entities.InspectOptions{})
}

// createTarBody builds an in-memory tar containing a single file with the given
// name and content. Mirrors the framing of podman's createTarFromReader
// (~/repos/podman/pkg/domain/infra/tunnel/quadlet.go:161) but operates on an
// in-memory byte slice instead of a pipe + goroutine. Writes target an
// in-memory bytes.Buffer and so should not fail in practice, but errors are
// propagated rather than swallowed.
func createTarBody(fileName string, content []byte) (io.Reader, error) {
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	hdr := &tar.Header{
		Name:    fileName,
		Mode:    0o644,
		Size:    int64(len(content)),
		ModTime: time.Now(),
	}
	if err := tw.WriteHeader(hdr); err != nil {
		return nil, fmt.Errorf("failed to write tar header: %w", err)
	}
	if _, err := tw.Write(content); err != nil {
		return nil, fmt.Errorf("failed to write tar content: %w", err)
	}
	if err := tw.Close(); err != nil {
		return nil, fmt.Errorf("failed to close tar: %w", err)
	}
	return &buf, nil
}
