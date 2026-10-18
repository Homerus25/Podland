package images

import (
	"context"

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

// fakeEngineProvider arms the provider seam: engine is handed to the
// service closure; err simulates an acquisition failure (the 502 sentinel
// crossing) and short-circuits the closure, exactly like the production
// manager's warn-and-sentinel behaviour.
type fakeEngineProvider struct {
	engine ImageEngine
	err    error
}

func (p fakeEngineProvider) WithEngine(ctx context.Context, userID, machineID, op string, fn func(e ImageEngine) error) error {
	if p.err != nil {
		return p.err
	}
	return fn(p.engine)
}
