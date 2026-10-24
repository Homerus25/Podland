package pods

import (
	"context"

	"go.podman.io/podman/v6/pkg/domain/entities"
)

// fakePodEngine is the function-field fake for the PodEngine seam. Each
// test arms only the operations its scenario needs; an unarmed operation
// panics, so an unexpected engine call fails the test loudly.
type fakePodEngine struct {
	listFn    func(opts entities.PodPSOptions) ([]*entities.ListPodsReport, error)
	createFn  func(spec entities.PodSpec) (*entities.PodCreateReport, error)
	inspectFn func(nameOrID string) ([]*entities.PodInspectReport, []error, error)
	pauseFn   func(nameOrID string) error
	unpauseFn func(nameOrID string) error
	killFn    func(nameOrID string) error
	restartFn func(nameOrID string) error
	removeFn  func(nameOrID string, force bool) error
	pruneFn   func() ([]*entities.PodPruneReport, error)
}

func (f *fakePodEngine) List(opts entities.PodPSOptions) ([]*entities.ListPodsReport, error) {
	if f.listFn == nil {
		panic("unexpected PodEngine.List call")
	}
	return f.listFn(opts)
}

func (f *fakePodEngine) Create(spec entities.PodSpec) (*entities.PodCreateReport, error) {
	if f.createFn == nil {
		panic("unexpected PodEngine.Create call")
	}
	return f.createFn(spec)
}

func (f *fakePodEngine) Inspect(nameOrID string) ([]*entities.PodInspectReport, []error, error) {
	if f.inspectFn == nil {
		panic("unexpected PodEngine.Inspect call")
	}
	return f.inspectFn(nameOrID)
}

func (f *fakePodEngine) Pause(nameOrID string) error {
	if f.pauseFn == nil {
		panic("unexpected PodEngine.Pause call")
	}
	return f.pauseFn(nameOrID)
}

func (f *fakePodEngine) Unpause(nameOrID string) error {
	if f.unpauseFn == nil {
		panic("unexpected PodEngine.Unpause call")
	}
	return f.unpauseFn(nameOrID)
}

func (f *fakePodEngine) Kill(nameOrID string) error {
	if f.killFn == nil {
		panic("unexpected PodEngine.Kill call")
	}
	return f.killFn(nameOrID)
}

func (f *fakePodEngine) Restart(nameOrID string) error {
	if f.restartFn == nil {
		panic("unexpected PodEngine.Restart call")
	}
	return f.restartFn(nameOrID)
}

func (f *fakePodEngine) Remove(nameOrID string, force bool) error {
	if f.removeFn == nil {
		panic("unexpected PodEngine.Remove call")
	}
	return f.removeFn(nameOrID, force)
}

func (f *fakePodEngine) Prune() ([]*entities.PodPruneReport, error) {
	if f.pruneFn == nil {
		panic("unexpected PodEngine.Prune call")
	}
	return f.pruneFn()
}

// fakeEngineProvider arms the provider seam: engine is handed to the
// service closure; err simulates an acquisition failure (the 502 sentinel
// crossing) and short-circuits the closure, exactly like the production
// manager's warn-and-sentinel behaviour. opRecorder, when set, observes
// the op tag the service hands the seam.
type fakeEngineProvider struct {
	engine     PodEngine
	err        error
	opRecorder func(op string)
}

func (p fakeEngineProvider) WithEngine(ctx context.Context, userID, machineID, op string, fn func(e PodEngine) error) error {
	if p.opRecorder != nil {
		p.opRecorder(op)
	}
	if p.err != nil {
		return p.err
	}
	return fn(p.engine)
}
