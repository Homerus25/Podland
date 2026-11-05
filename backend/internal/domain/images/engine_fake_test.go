package images

import (
	"context"
	"io"

	"go.podman.io/podman/v6/pkg/domain/entities"
	"go.podman.io/podman/v6/pkg/domain/entities/reports"
)

// fakeImageEngine is the function-field fake for the ImageEngine seam.
// Each test arms only the operations its scenario needs; an unarmed
// operation panics, so an unexpected engine call fails the test loudly.
type fakeImageEngine struct {
	listFn    func(opts entities.ImageListOptions) ([]*entities.ImageSummary, error)
	removeFn  func(id string, force bool) (*entities.ImageRemoveReport, []error)
	pullFn    func(rawImage string, opts entities.ImagePullOptions) (*entities.ImagePullReport, error)
	pruneFn   func() ([]*reports.PruneReport, error)
	inspectFn func(id string) ([]*entities.ImageInspectReport, []error, error)
	existsFn  func(id string) (bool, error)
	exportFn  func(ref string, w io.Writer) error
	loadFn    func(r io.Reader) ([]string, error)
}

func (f *fakeImageEngine) List(opts entities.ImageListOptions) ([]*entities.ImageSummary, error) {
	if f.listFn == nil {
		panic("unexpected ImageEngine.List call")
	}
	return f.listFn(opts)
}

func (f *fakeImageEngine) Remove(id string, force bool) (*entities.ImageRemoveReport, []error) {
	if f.removeFn == nil {
		panic("unexpected ImageEngine.Remove call")
	}
	return f.removeFn(id, force)
}

func (f *fakeImageEngine) Pull(rawImage string, opts entities.ImagePullOptions) (*entities.ImagePullReport, error) {
	if f.pullFn == nil {
		panic("unexpected ImageEngine.Pull call")
	}
	return f.pullFn(rawImage, opts)
}

func (f *fakeImageEngine) Prune() ([]*reports.PruneReport, error) {
	if f.pruneFn == nil {
		panic("unexpected ImageEngine.Prune call")
	}
	return f.pruneFn()
}

func (f *fakeImageEngine) Inspect(id string) ([]*entities.ImageInspectReport, []error, error) {
	if f.inspectFn == nil {
		panic("unexpected ImageEngine.Inspect call")
	}
	return f.inspectFn(id)
}

func (f *fakeImageEngine) Exists(id string) (bool, error) {
	if f.existsFn == nil {
		panic("unexpected ImageEngine.Exists call")
	}
	return f.existsFn(id)
}

func (f *fakeImageEngine) Export(ref string, w io.Writer) error {
	if f.exportFn == nil {
		panic("unexpected ImageEngine.Export call")
	}
	return f.exportFn(ref, w)
}

func (f *fakeImageEngine) Load(r io.Reader) ([]string, error) {
	if f.loadFn == nil {
		panic("unexpected ImageEngine.Load call")
	}
	return f.loadFn(r)
}

// fakeEngineProvider arms the provider seam: engine is handed to the
// service closure; err simulates an acquisition failure (the 502 sentinel
// crossing) and short-circuits the closure, exactly like the production
// manager's warn-and-sentinel behaviour.
type fakeEngineProvider struct {
	engine ImageEngine
	err    error
	// engines serves per-host engines keyed by host ID — the shape
	// the cross-host copy exercises. When non-nil it takes precedence
	// over engine; a lookup miss panics, so a closure crossing an
	// unexpected host fails the test loudly.
	engines map[string]ImageEngine
	// engineErrs serves per-host acquisition failures keyed by
	// host ID — the shape a target-lease failure (one host
	// reachable, the other not) exercises. Checked before the engines
	// lookup.
	engineErrs map[string]error
}

func (p fakeEngineProvider) WithEngine(ctx context.Context, userID, hostID, op string, fn func(e ImageEngine) error) error {
	if p.err != nil {
		return p.err
	}
	if p.engineErrs != nil {
		if err, ok := p.engineErrs[hostID]; ok {
			return err
		}
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
