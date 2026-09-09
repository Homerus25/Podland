package volumes

import (
	"context"
	"io"

	"go.podman.io/podman/v6/pkg/domain/entities"
	"go.podman.io/podman/v6/pkg/domain/entities/reports"
)

// fakeVolumeEngine is the function-field fake for the VolumeEngine seam.
// Each test arms only the operations its scenario needs; an unarmed
// operation panics, so an unexpected engine call fails the test loudly.
type fakeVolumeEngine struct {
	listFn    func(opts entities.VolumeListOptions) ([]*entities.VolumeListReport, error)
	inspectFn func(name string) (*entities.VolumeInspectReport, error)
	createFn  func(opts entities.VolumeCreateOptions) (*entities.IDOrNameResponse, error)
	removeFn  func(name string, force bool) ([]*entities.VolumeRmReport, error)
	pruneFn   func() ([]*reports.PruneReport, error)
	exportFn  func(name string, w io.Writer) error
	importFn  func(name string, r io.Reader) error
}

func (f *fakeVolumeEngine) List(opts entities.VolumeListOptions) ([]*entities.VolumeListReport, error) {
	if f.listFn == nil {
		panic("unexpected VolumeEngine.List call")
	}
	return f.listFn(opts)
}

func (f *fakeVolumeEngine) Inspect(name string) (*entities.VolumeInspectReport, error) {
	if f.inspectFn == nil {
		panic("unexpected VolumeEngine.Inspect call")
	}
	return f.inspectFn(name)
}

func (f *fakeVolumeEngine) Create(opts entities.VolumeCreateOptions) (*entities.IDOrNameResponse, error) {
	if f.createFn == nil {
		panic("unexpected VolumeEngine.Create call")
	}
	return f.createFn(opts)
}

func (f *fakeVolumeEngine) Remove(name string, force bool) ([]*entities.VolumeRmReport, error) {
	if f.removeFn == nil {
		panic("unexpected VolumeEngine.Remove call")
	}
	return f.removeFn(name, force)
}

func (f *fakeVolumeEngine) Prune() ([]*reports.PruneReport, error) {
	if f.pruneFn == nil {
		panic("unexpected VolumeEngine.Prune call")
	}
	return f.pruneFn()
}

func (f *fakeVolumeEngine) Export(name string, w io.Writer) error {
	if f.exportFn == nil {
		panic("unexpected VolumeEngine.Export call")
	}
	return f.exportFn(name, w)
}

func (f *fakeVolumeEngine) Import(name string, r io.Reader) error {
	if f.importFn == nil {
		panic("unexpected VolumeEngine.Import call")
	}
	return f.importFn(name, r)
}

// fakeEngineProvider arms the provider seam: engine is handed to the
// service closure; err simulates an acquisition failure (the 502 sentinel
// crossing) and short-circuits the closure, exactly like the production
// manager's warn-and-sentinel behaviour.
type fakeEngineProvider struct {
	engine VolumeEngine
	err    error
}

func (p fakeEngineProvider) WithEngine(ctx context.Context, userID, machineID, op string, fn func(e VolumeEngine) error) error {
	if p.err != nil {
		return p.err
	}
	return fn(p.engine)
}
