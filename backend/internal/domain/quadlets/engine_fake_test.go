package quadlets

import (
	"context"
	"io"

	"github.com/Homerus25/podland/backend/internal/models"
	"go.podman.io/podman/v6/pkg/domain/entities"
)

// fakeQuadletEngine is the function-field fake for the QuadletEngine seam.
// Each test arms only the operations its scenario needs; an unarmed
// operation panics, so an unexpected engine call fails the test loudly.
type fakeQuadletEngine struct {
	listFn             func(opts entities.QuadletListOptions) ([]*entities.ListQuadlet, error)
	printFn            func(name string) (string, error)
	removeFn           func(name string, force bool) error
	installFn          func(fileName string, content []byte, replace bool) (*entities.QuadletInstallReport, error)
	logsFn             func(containerName string, opts entities.ContainerLogsOptions) error
	inspectContainerFn func(id string) ([]*entities.ContainerInspectReport, []error, error)
	inspectPodFn       func(id string) ([]*entities.PodInspectReport, []error, error)
	volumeInspectFn    func(name string) (*QuadletVolumeInspect, error)
	volumeCreateFn     func(name string) error
	volumeExportFn     func(name string, w io.Writer) error
	volumeImportFn     func(name string, r io.Reader) error
}

func (f *fakeQuadletEngine) List(opts entities.QuadletListOptions) ([]*entities.ListQuadlet, error) {
	if f.listFn == nil {
		panic("unexpected QuadletEngine.List call")
	}
	return f.listFn(opts)
}

func (f *fakeQuadletEngine) Print(name string) (string, error) {
	if f.printFn == nil {
		panic("unexpected QuadletEngine.Print call")
	}
	return f.printFn(name)
}

func (f *fakeQuadletEngine) Remove(name string, force bool) error {
	if f.removeFn == nil {
		panic("unexpected QuadletEngine.Remove call")
	}
	return f.removeFn(name, force)
}

func (f *fakeQuadletEngine) Install(fileName string, content []byte, replace bool) (*entities.QuadletInstallReport, error) {
	if f.installFn == nil {
		panic("unexpected QuadletEngine.Install call")
	}
	return f.installFn(fileName, content, replace)
}

func (f *fakeQuadletEngine) Logs(containerName string, opts entities.ContainerLogsOptions) error {
	if f.logsFn == nil {
		panic("unexpected QuadletEngine.Logs call")
	}
	return f.logsFn(containerName, opts)
}

func (f *fakeQuadletEngine) InspectContainer(id string) ([]*entities.ContainerInspectReport, []error, error) {
	if f.inspectContainerFn == nil {
		panic("unexpected QuadletEngine.InspectContainer call")
	}
	return f.inspectContainerFn(id)
}

func (f *fakeQuadletEngine) InspectPod(id string) ([]*entities.PodInspectReport, []error, error) {
	if f.inspectPodFn == nil {
		panic("unexpected QuadletEngine.InspectPod call")
	}
	return f.inspectPodFn(id)
}

func (f *fakeQuadletEngine) VolumeInspect(name string) (*QuadletVolumeInspect, error) {
	if f.volumeInspectFn == nil {
		// No panic here: MovePreview inspects volumes unconditionally since
		// issue 214, so an unarmed inspect answers the production contract
		// for an absent volume — nil report, nil error — instead of failing
		// every preview-shaped test that doesn't care about volumes. Tests
		// that DO care arm volumeInspectFn explicitly.
		return nil, nil
	}
	return f.volumeInspectFn(name)
}

func (f *fakeQuadletEngine) VolumeCreate(name string) error {
	if f.volumeCreateFn == nil {
		panic("unexpected QuadletEngine.VolumeCreate call")
	}
	return f.volumeCreateFn(name)
}

func (f *fakeQuadletEngine) VolumeExport(name string, w io.Writer) error {
	if f.volumeExportFn == nil {
		panic("unexpected QuadletEngine.VolumeExport call")
	}
	return f.volumeExportFn(name, w)
}

func (f *fakeQuadletEngine) VolumeImport(name string, r io.Reader) error {
	if f.volumeImportFn == nil {
		panic("unexpected QuadletEngine.VolumeImport call")
	}
	return f.volumeImportFn(name, r)
}

// fakeEngineProvider arms the provider seam: engine is handed to the
// service closure; err simulates an acquisition failure (the 502 sentinel
// crossing) and short-circuits the closure, exactly like the production
// manager's warn-and-sentinel behaviour.
type fakeEngineProvider struct {
	engine QuadletEngine
	err    error
	// engines serves per-host engines keyed by host ID — the shape
	// the cross-host operations (move-preview) exercise. When non-nil
	// it takes precedence over engine; a lookup miss panics, so a closure
	// crossing an unexpected host fails the test loudly.
	engines map[string]QuadletEngine
}

func (p fakeEngineProvider) WithEngine(ctx context.Context, userID, hostID, op string, fn func(e QuadletEngine) error) error {
	if p.err != nil {
		return p.err
	}
	if p.engines != nil {
		e, ok := p.engines[hostID]
		if !ok {
			panic("unexpected WithEngine host: " + hostID)
		}
		return fn(e)
	}
	return fn(p.engine)
}

// fakeMoveSecretEngine is the function-field fake for the moveSecretEngine
// seam (issue 215). Each test arms only the operations its scenario needs;
// an unarmed operation panics, so an unexpected secret-store call fails
// the test loudly — the shape that pins "no reveal when the flag is
// absent" and "no store consult for hostPath references".
type fakeMoveSecretEngine struct {
	namesFn  func() ([]string, error)
	hasFn    func(name string) (bool, error)
	revealFn func(name string) (string, error)
	createFn func(name, value string) error
}

func (f *fakeMoveSecretEngine) Names() ([]string, error) {
	if f.namesFn == nil {
		panic("unexpected moveSecretEngine.Names call")
	}
	return f.namesFn()
}

func (f *fakeMoveSecretEngine) Has(name string) (bool, error) {
	if f.hasFn == nil {
		panic("unexpected moveSecretEngine.Has call")
	}
	return f.hasFn(name)
}

func (f *fakeMoveSecretEngine) Reveal(name string) (string, error) {
	if f.revealFn == nil {
		panic("unexpected moveSecretEngine.Reveal call")
	}
	return f.revealFn(name)
}

func (f *fakeMoveSecretEngine) Create(name, value string) error {
	if f.createFn == nil {
		panic("unexpected moveSecretEngine.Create call")
	}
	return f.createFn(name, value)
}

// fakeSecretEngineProvider arms the secret provider seam per host ID
// (the two-lease shape the secret surface exercises); a lookup miss
// panics, so a closure crossing an unexpected host fails the test
// loudly.
type fakeSecretEngineProvider struct {
	engines map[string]moveSecretEngine
}

func (p fakeSecretEngineProvider) WithEngine(ctx context.Context, userID, hostID, op string, fn func(e moveSecretEngine) error) error {
	e, ok := p.engines[hostID]
	if !ok {
		panic("unexpected secret WithEngine host: " + hostID)
	}
	return fn(e)
}

// fakeHostLookup is the function-field fake for the hostLookup seam
// behind systemctl scoping: host is the row GetByID answers (nil
// simulates a host deleted concurrently with the request), err a store
// failure. lastID records what the service asked for so tests can pin that
// scoping resolves per host.
type fakeHostLookup struct {
	host   *models.Host
	err    error
	lastID string
}

func (f *fakeHostLookup) GetByID(id string) (*models.Host, error) {
	f.lastID = id
	return f.host, f.err
}

// lookupForUser arms a fakeHostLookup whose host row registers the
// given SSH user — the input that decides the `--user` scope.
func lookupForUser(sshUser string) *fakeHostLookup {
	return &fakeHostLookup{host: &models.Host{ID: "host", SSHUsername: sshUser}}
}
