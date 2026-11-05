package secrets

import (
	"context"
	"strings"

	"github.com/Homerus25/podland/backend/internal/connections"
	"github.com/Homerus25/podland/backend/internal/transfer"
	podmanSecrets "go.podman.io/podman/v6/pkg/bindings/secrets"
	"go.podman.io/podman/v6/pkg/domain/entities"
	"go.podman.io/podman/v6/pkg/domain/infra/tunnel"
)

// SecretEngine is the secrets domain's view of one host's podman: the
// exact set of secret operations the service performs, in domain vocabulary
// (plain name/value strings, a reveal flag) and without a context parameter
// — the connection context lives only inside the production adapter below.
type SecretEngine interface {
	List() ([]*entities.SecretInfoReport, error)
	// Inspect fetches one secret by name or ID; reveal asks podman to
	// include the value (the admin gate is the service's).
	Inspect(name string, reveal bool) (*entities.SecretInfoReport, error)
	// Create installs a secret whose value must reach podman
	// byte-identical (the adapter owns the body-reader wiring).
	Create(name, value string) (*entities.SecretCreateReport, error)
	Remove(name string) error
}

// SecretEngineProvider is the secrets domain's slice of the connection
// seam: it hands a closure one host's SecretEngine. It delegates to
// connections.Manager.WithConnection, which owns acquisition, the
// warn-and-502-sentinel failure contract, and the op tag.
type SecretEngineProvider interface {
	WithEngine(ctx context.Context, userID, hostID, op string, fn func(e SecretEngine) error) error
}

// SecretUsageEngineProvider is the secrets domain's slice of the
// connection seam for the quadlet surface (issue 239): it hands a
// closure one host's quadlet reader — the kit's
// transfer.QuadletReader (issue 260, §19), the exact reads the
// copy/move preview's referencing-quadlet scan needs — delegating to
// connections.Manager. The domain declares no usage-engine interface
// of its own: the kit's reader IS the width, and the podman adapter
// below satisfies it structurally.
type SecretUsageEngineProvider interface {
	WithEngine(ctx context.Context, userID, hostID, op string, fn func(e transfer.QuadletReader) error) error
}

// NewSecretUsageEngineProvider returns the production usage provider: it
// wraps the connection manager and builds the tunnel engine inside the
// connection lease, like NewSecretEngineProvider does for secrets.
func NewSecretUsageEngineProvider(conn connections.Manager) SecretUsageEngineProvider {
	return connectionUsageEngines{conn: conn}
}

// connectionUsageEngines is the production SecretUsageEngineProvider.
type connectionUsageEngines struct {
	conn connections.Manager
}

func (p connectionUsageEngines) WithEngine(ctx context.Context, userID, hostID, op string, fn func(e transfer.QuadletReader) error) error {
	return p.conn.WithConnection(ctx, userID, hostID, op, func(connCtx context.Context) error {
		return fn(&podmanSecretUsageEngine{ctx: connCtx})
	})
}

// podmanSecretUsageEngine satisfies the kit's transfer.QuadletReader
// against one live connection (issue 260, §19).
type podmanSecretUsageEngine struct {
	ctx context.Context
}

func (e *podmanSecretUsageEngine) tunnelEngine() *tunnel.ContainerEngine {
	return &tunnel.ContainerEngine{ClientCtx: e.ctx}
}

func (e *podmanSecretUsageEngine) ListQuadletNames() ([]string, error) {
	list, err := e.tunnelEngine().QuadletList(e.ctx, entities.QuadletListOptions{})
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(list))
	for _, quadlet := range list {
		names = append(names, quadlet.Name)
	}
	return names, nil
}

func (e *podmanSecretUsageEngine) PrintQuadlet(name string) (string, error) {
	return e.tunnelEngine().QuadletPrint(e.ctx, name)
}

// NewSecretEngineProvider returns the production provider: it wraps the
// connection manager and builds the bindings calls inside the connection
// lease. This file is the only one in the package importing the podman
// tunnel/bindings packages.
func NewSecretEngineProvider(conn connections.Manager) SecretEngineProvider {
	return connectionEngines{conn: conn}
}

// connectionEngines is the production SecretEngineProvider.
type connectionEngines struct {
	conn connections.Manager
}

func (p connectionEngines) WithEngine(ctx context.Context, userID, hostID, op string, fn func(e SecretEngine) error) error {
	return p.conn.WithConnection(ctx, userID, hostID, op, func(connCtx context.Context) error {
		return fn(&podmanSecretEngine{ctx: connCtx})
	})
}

// podmanSecretEngine implements SecretEngine against one live connection.
type podmanSecretEngine struct {
	ctx context.Context
}

func (e *podmanSecretEngine) List() ([]*entities.SecretInfoReport, error) {
	return podmanSecrets.List(e.ctx, nil)
}

func (e *podmanSecretEngine) Inspect(name string, reveal bool) (*entities.SecretInfoReport, error) {
	opts := new(podmanSecrets.InspectOptions)
	if reveal {
		opts = opts.WithShowSecret(true)
	}
	return podmanSecrets.Inspect(e.ctx, name, opts)
}

func (e *podmanSecretEngine) Create(name, value string) (*entities.SecretCreateReport, error) {
	opts := new(podmanSecrets.CreateOptions).WithName(name)
	return podmanSecrets.Create(e.ctx, strings.NewReader(value), opts)
}

func (e *podmanSecretEngine) Remove(name string) error {
	return podmanSecrets.Remove(e.ctx, name)
}
