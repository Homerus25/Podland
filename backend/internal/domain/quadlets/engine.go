package quadlets

import (
	"archive/tar"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/Homerus25/podland/backend/internal/connections"
	"github.com/Homerus25/podland/backend/internal/podmanerr"
	"go.podman.io/podman/v6/pkg/bindings"
	"go.podman.io/podman/v6/pkg/bindings/quadlets"
	podmanSecrets "go.podman.io/podman/v6/pkg/bindings/secrets"
	bindingsVolumes "go.podman.io/podman/v6/pkg/bindings/volumes"
	"go.podman.io/podman/v6/pkg/domain/entities"
	"go.podman.io/podman/v6/pkg/domain/infra/tunnel"
)

// QuadletEngine is the quadlets domain's view of one host's podman: the
// exact set of quadlet and managed-container operations the service
// performs, in domain vocabulary (single names, plain flags, raw file
// content) and without a context parameter — the connection context lives
// only inside the production adapter below.
type QuadletEngine interface {
	List(opts entities.QuadletListOptions) ([]*entities.ListQuadlet, error)
	// Print returns the quadlet unit file's content.
	Print(name string) (string, error)
	// Remove uninstalls the quadlet and answers one error; systemd is
	// always reloaded. The adapter folds the remove report's error map for
	// the named quadlet into the single error the service classifies — a
	// transport error, or the named quadlet's entry in the report's
	// Errors map (the race window where the quadlet vanished or its
	// resource started between the locate and the remove), becomes the
	// error; a clean removal answers nil.
	Remove(name string, force bool) error
	// RemoveAll is Remove's batch twin: one podman call for the whole
	// bulk-remove request, answering the per-name outcome. The failure's
	// cause does not cross the wire — podman's batch endpoint marshals
	// the report's error values as empty objects, so it carries WHICH
	// quadlet failed but never WHY (the adapter owns that fact); the
	// service answers the failed names with the generic entry.
	RemoveAll(names []string, force bool) (*QuadletBatchRemoveReport, error)
	// Install installs one quadlet file: the adapter owns the tar framing
	// podman's install primitive requires. replace overwrites an existing
	// installation; systemd is always reloaded.
	Install(fileName string, content []byte, replace bool) (*entities.QuadletInstallReport, error)
	// Logs writes the managed container's logs into the writers carried by
	// opts (follow or one-shot is the caller's choice via opts).
	Logs(containerName string, opts entities.ContainerLogsOptions) error
	InspectContainer(id string) ([]*entities.ContainerInspectReport, []error, error)
	InspectPod(id string) ([]*entities.PodInspectReport, []error, error)

	// Volume data migration (issue 214, architecture-decisions §18): the
	// volume operations the move/copy data phase performs, in domain
	// vocabulary. VolumeInspect resolves one volume by exact name; a
	// missing volume comes back as a nil report with a nil error (the same
	// contract the volumes domain's engine sets). VolumeCreate is
	// create-if-absent. Export/Import are the podman tar streams the
	// host-to-host pipe splices.
	VolumeInspect(name string) (*QuadletVolumeInspect, error)
	VolumeCreate(name string) error
	VolumeExport(name string, w io.Writer) error
	VolumeImport(name string, r io.Reader) error
}

// QuadletVolumeInspect is the volume facts the move/copy data phase and
// preview need about one podman volume. Size is the volume's content size
// in bytes WHEN the engine can report one — podman's service API reports
// no volume usage today, so production answers nil and the migration
// bound's pre-check has nothing to compare; the counting reader is the
// always-on backstop (issue 214). The dedicated DTO keeps that evolution
// local to this domain instead of teaching the whole entities type about
// sizes.
type QuadletVolumeInspect struct {
	Name string
	Size *int64
}

// QuadletBatchRemoveReport is the batch remove's per-name outcome in
// domain vocabulary: the names podman removed and the names it refused
// or failed to remove. The failure's cause is deliberately absent —
// podman's batch endpoint marshals the report's error values as empty
// objects, so the wire carries WHICH quadlet failed but never WHY.
type QuadletBatchRemoveReport struct {
	// Removed lists the quadlets the call removed.
	Removed []string
	// Failed lists the quadlets podman reported an error for.
	Failed []string
}

// QuadletEngineProvider is the quadlets domain's slice of the connection
// seam: it hands a closure one host's QuadletEngine. It delegates to
// connections.Manager.WithConnection, which owns acquisition, the
// warn-and-502-sentinel failure contract, and the op tag.
type QuadletEngineProvider interface {
	WithEngine(ctx context.Context, userID, hostID, op string, fn func(e QuadletEngine) error) error
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

func (p connectionEngines) WithEngine(ctx context.Context, userID, hostID, op string, fn func(e QuadletEngine) error) error {
	return p.conn.WithConnection(ctx, userID, hostID, op, func(connCtx context.Context) error {
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

// Remove narrows podman's report-shaped remove to the single error the
// service classifies: a transport error becomes the error, and the
// report's Errors map entry for the named quadlet — the race window
// where the quadlet vanished or its resource started between the locate
// and the remove — becomes the error too; a clean removal answers nil.
func (e *podmanQuadletEngine) Remove(name string, force bool) error {
	report, err := e.tunnelEngine.QuadletRemove(e.ctx, []string{name}, entities.QuadletRemoveOptions{
		Force:         force,
		ReloadSystemd: true,
	})
	if err != nil {
		return err
	}
	if report != nil {
		if qErr, ok := report.Errors[name]; ok && qErr != nil {
			return reportError{err: qErr}
		}
	}
	return nil
}

// reportError marks an error that arrived in the remove report's Errors
// map rather than at the call itself — the race window where the quadlet
// vanished or its removal failed between the locate and the remove. The
// adapter owns the report shape; the marker lets a caller that
// historically discarded the report keep doing so (Update's best-effort
// replace proceeds to its install) while classifying callers classify
// through it — podmanerr's predicates and the removal module's wrap both
// unwrap.
type reportError struct{ err error }

func (e reportError) Error() string { return e.err.Error() }
func (e reportError) Unwrap() error { return e.err }

// isReportError reports whether err arrived in the remove report rather
// than at the call.
func isReportError(err error) bool {
	var rErr reportError
	return errors.As(err, &rErr)
}

// RemoveAll is the batch twin of Remove: one podman call for the whole
// bulk-remove request (podman's DELETE /quadlets accepts the array). The
// bindings' own QuadletRemove decodes into entities.QuadletRemoveReport,
// whose Errors map[string]error cannot cross the wire — podman marshals
// the error values as empty objects, so the bindings' decode fails
// outright whenever any quadlet failed, discarding the whole report. The
// adapter therefore owns the call and decodes tolerantly: the error
// values are empty either way, but the KEYS name the failed quadlets,
// which is the per-name attribution the bulk report needs.
func (e *podmanQuadletEngine) RemoveAll(names []string, force bool) (*QuadletBatchRemoveReport, error) {
	options := new(quadlets.QuadletRemoveOptions).WithForce(force).WithReloadSystemd(true)
	params, err := options.ToParams()
	if err != nil {
		return nil, err
	}
	for _, name := range names {
		params.Add("quadlets", name)
	}
	conn, err := bindings.GetClient(e.ctx)
	if err != nil {
		return nil, err
	}
	response, err := conn.DoRequest(e.ctx, nil, http.MethodDelete, "/quadlets", params, nil)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if !response.IsSuccess() {
		return nil, response.Process(nil)
	}

	var wire struct {
		Removed []string
		Errors  map[string]json.RawMessage
	}
	if err := response.Process(&wire); err != nil {
		return nil, err
	}

	report := &QuadletBatchRemoveReport{Removed: wire.Removed, Failed: make([]string, 0, len(wire.Errors))}
	for name := range wire.Errors {
		report.Failed = append(report.Failed, name)
	}
	return report, nil
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

// VolumeInspect narrows podman's slice-shaped volume inspect to the
// single-name width the migration code wants, mirroring the volumes
// domain's adapter: a transport error or the item's own error becomes the
// error, an absent volume becomes a nil report with a nil error. Size
// stays nil — podman's service API reports no volume usage today
// (QuadletVolumeInspect documents the contract).
func (e *podmanQuadletEngine) VolumeInspect(name string) (*QuadletVolumeInspect, error) {
	inspected, itemErrs, err := e.tunnelEngine.VolumeInspect(e.ctx, []string{name}, entities.InspectOptions{})
	if err != nil {
		return nil, err
	}
	for _, itemErr := range itemErrs {
		if itemErr != nil {
			return nil, itemErr
		}
	}
	if len(inspected) == 0 || inspected[0] == nil {
		return nil, nil
	}
	return &QuadletVolumeInspect{Name: inspected[0].Name}, nil
}

// VolumeCreate creates-if-absent: IgnoreIfExists makes an existing volume
// a no-op, so the data phase can call it unconditionally before importing.
func (e *podmanQuadletEngine) VolumeCreate(name string) error {
	_, err := e.tunnelEngine.VolumeCreate(e.ctx, entities.VolumeCreateOptions{Name: name, IgnoreIfExists: true})
	return err
}

func (e *podmanQuadletEngine) VolumeExport(name string, w io.Writer) error {
	return bindingsVolumes.Export(e.ctx, name, w)
}

func (e *podmanQuadletEngine) VolumeImport(name string, r io.Reader) error {
	return bindingsVolumes.Import(e.ctx, name, r)
}

// moveSecretEngine is the quadlets domain's view of one host's podman
// secret store (issue 215): the exact set of secret operations the
// move/copy preview and transfer perform, in domain vocabulary, without a
// context parameter — the connection context lives only inside the
// production adapter below. Deliberately narrower than the secrets
// domain's engine: no Remove (secrets are copy-only, any mode), no
// metadata Inspect.
type moveSecretEngine interface {
	// Names lists the host's podman secret names — the preview's
	// existence read (both hosts).
	Names() ([]string, error)
	// Has answers whether the store holds a secret of this name — the
	// transfer's same-name check (target). A missing secret is a clean
	// false, never an error.
	Has(name string) (bool, error)
	// Reveal fetches one secret's value; a missing secret answers a
	// podman not-found error (the caller's admin gate is the service's).
	Reveal(name string) (string, error)
	// Create installs a secret whose value must reach podman
	// byte-identical (the adapter owns the body-reader wiring).
	Create(name, value string) error
}

// moveSecretEngineProvider is the quadlets domain's slice of the
// connection seam for the secret surface (issue 215): it hands a closure
// one host's moveSecretEngine, delegating to connections.Manager.
type moveSecretEngineProvider interface {
	WithEngine(ctx context.Context, userID, hostID, op string, fn func(e moveSecretEngine) error) error
}

// NewQuadletSecretEngineProvider returns the production secret provider
// for the move/copy's secret surface (issue 215): it wraps the connection
// manager and builds the secrets bindings calls inside the connection
// lease, like NewQuadletEngineProvider does for quadlets.
func NewQuadletSecretEngineProvider(conn connections.Manager) moveSecretEngineProvider {
	return connectionSecretEngines{conn: conn}
}

// connectionSecretEngines is the production moveSecretEngineProvider.
type connectionSecretEngines struct {
	conn connections.Manager
}

func (p connectionSecretEngines) WithEngine(ctx context.Context, userID, hostID, op string, fn func(e moveSecretEngine) error) error {
	return p.conn.WithConnection(ctx, userID, hostID, op, func(connCtx context.Context) error {
		return fn(&podmanMoveSecretEngine{ctx: connCtx})
	})
}

// podmanMoveSecretEngine implements moveSecretEngine against one live
// connection.
type podmanMoveSecretEngine struct {
	ctx context.Context
}

func (e *podmanMoveSecretEngine) Names() ([]string, error) {
	reports, err := podmanSecrets.List(e.ctx, nil)
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(reports))
	for _, report := range reports {
		if report != nil && report.Spec.Name != "" {
			names = append(names, report.Spec.Name)
		}
	}
	return names, nil
}

// Has answers the same-name check through the unrevealed Inspect — one
// store read for the one name the transfer asks about, no name list
// crossing the seam. A missing secret is a clean false; the presence
// answer is the same predicate the secrets domain's adapter uses (a
// non-nil report), so the kit's one question has one truth table.
func (e *podmanMoveSecretEngine) Has(name string) (bool, error) {
	report, err := podmanSecrets.Inspect(e.ctx, name, new(podmanSecrets.InspectOptions))
	if err != nil {
		if podmanerr.IsNotFound(err) {
			return false, nil
		}
		return false, err
	}
	return report != nil, nil
}

// Reveal asks podman for the value (WithShowSecret) and maps it to the
// plain string; the audit event is the service's, per the redaction
// discipline.
func (e *podmanMoveSecretEngine) Reveal(name string) (string, error) {
	report, err := podmanSecrets.Inspect(e.ctx, name, new(podmanSecrets.InspectOptions).WithShowSecret(true))
	if err != nil {
		return "", err
	}
	return report.SecretData, nil
}

func (e *podmanMoveSecretEngine) Create(name, value string) error {
	_, err := podmanSecrets.Create(e.ctx, strings.NewReader(value), new(podmanSecrets.CreateOptions).WithName(name))
	return err
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
