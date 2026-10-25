package containers

import (
	"context"
	"io"

	"go.podman.io/podman/v6/pkg/domain/entities"
	"go.podman.io/podman/v6/pkg/specgen"
)

// fakeContainerEngine is the function-field fake for the ContainerEngine
// seam. Each test arms only the operations its scenario needs; an unarmed
// operation panics, so an unexpected engine call fails the test loudly.
type fakeContainerEngine struct {
	listFn    func(opts entities.ContainerListOptions) ([]entities.ListContainer, error)
	createFn  func(spec *specgen.SpecGenerator) (*entities.ContainerCreateReport, error)
	inspectFn func(id string) ([]*entities.ContainerInspectReport, []error, error)
	startFn   func(id string) error
	stopFn    func(id string) error
	removeFn  func(id string, force bool) error
	logsFn    func(name string, opts entities.ContainerLogsOptions) error
}

func (f *fakeContainerEngine) List(opts entities.ContainerListOptions) ([]entities.ListContainer, error) {
	if f.listFn == nil {
		panic("unexpected ContainerEngine.List call")
	}
	return f.listFn(opts)
}

func (f *fakeContainerEngine) Create(spec *specgen.SpecGenerator) (*entities.ContainerCreateReport, error) {
	if f.createFn == nil {
		panic("unexpected ContainerEngine.Create call")
	}
	return f.createFn(spec)
}

func (f *fakeContainerEngine) Inspect(id string) ([]*entities.ContainerInspectReport, []error, error) {
	if f.inspectFn == nil {
		panic("unexpected ContainerEngine.Inspect call")
	}
	return f.inspectFn(id)
}

func (f *fakeContainerEngine) Start(id string) error {
	if f.startFn == nil {
		panic("unexpected ContainerEngine.Start call")
	}
	return f.startFn(id)
}

func (f *fakeContainerEngine) Stop(id string) error {
	if f.stopFn == nil {
		panic("unexpected ContainerEngine.Stop call")
	}
	return f.stopFn(id)
}

func (f *fakeContainerEngine) Remove(id string, force bool) error {
	if f.removeFn == nil {
		panic("unexpected ContainerEngine.Remove call")
	}
	return f.removeFn(id, force)
}

func (f *fakeContainerEngine) Logs(name string, opts entities.ContainerLogsOptions) error {
	if f.logsFn == nil {
		panic("unexpected ContainerEngine.Logs call")
	}
	return f.logsFn(name, opts)
}

// fakeEngineProvider arms the provider seam: engine is handed to the
// service closure; err simulates an acquisition failure (the 502 sentinel
// crossing) and short-circuits the closure, exactly like the production
// manager's warn-and-sentinel behaviour. opRecorder, when set, observes
// every crossed op, so a test can pin that a path never acquired the
// lease at all.
type fakeEngineProvider struct {
	engine     ContainerEngine
	err        error
	opRecorder func(op string)
}

func (p fakeEngineProvider) WithEngine(ctx context.Context, userID, machineID, op string, fn func(e ContainerEngine) error) error {
	if p.err != nil {
		return p.err
	}
	if p.opRecorder != nil {
		p.opRecorder(op)
	}
	return fn(p.engine)
}

// writeLogsInto simulates podman's log delivery: podman streams each line
// into the writers the service attached to the logs options.
func writeLogsInto(opts entities.ContainerLogsOptions, stdout, stderr string) {
	if stdout != "" && opts.StdoutWriter != nil {
		_, _ = io.WriteString(opts.StdoutWriter, stdout)
	}
	if stderr != "" && opts.StderrWriter != nil {
		_, _ = io.WriteString(opts.StderrWriter, stderr)
	}
}
