package quadlets

import (
	"context"

	"go.podman.io/podman/v6/pkg/domain/entities"
)

// fakeQuadletEngine is the function-field fake for the QuadletEngine seam.
// Each test arms only the operations its scenario needs; an unarmed
// operation panics, so an unexpected engine call fails the test loudly.
type fakeQuadletEngine struct {
	listFn             func(opts entities.QuadletListOptions) ([]*entities.ListQuadlet, error)
	printFn            func(name string) (string, error)
	removeFn           func(name string, force bool) (*entities.QuadletRemoveReport, error)
	installFn          func(fileName string, content []byte, replace bool) (*entities.QuadletInstallReport, error)
	logsFn             func(containerName string, opts entities.ContainerLogsOptions) error
	inspectContainerFn func(id string) ([]*entities.ContainerInspectReport, []error, error)
	inspectPodFn       func(id string) ([]*entities.PodInspectReport, []error, error)
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

func (f *fakeQuadletEngine) Remove(name string, force bool) (*entities.QuadletRemoveReport, error) {
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

// fakeEngineProvider arms the provider seam: engine is handed to the
// service closure; err simulates an acquisition failure (the 502 sentinel
// crossing) and short-circuits the closure, exactly like the production
// manager's warn-and-sentinel behaviour.
type fakeEngineProvider struct {
	engine QuadletEngine
	err    error
}

func (p fakeEngineProvider) WithEngine(ctx context.Context, userID, machineID, op string, fn func(e QuadletEngine) error) error {
	if p.err != nil {
		return p.err
	}
	return fn(p.engine)
}
