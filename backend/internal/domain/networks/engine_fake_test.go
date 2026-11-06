package networks

import (
	"context"

	"go.podman.io/common/libnetwork/types"
	"go.podman.io/podman/v6/pkg/domain/entities"
)

// fakeNetworkEngine is the function-field fake for the NetworkEngine
// seam. Each test arms only the operations its scenario needs; an
// unarmed operation panics, so an unexpected engine call fails the test
// loudly.
type fakeNetworkEngine struct {
	listFn    func() ([]types.Network, error)
	createFn  func(name string) (*types.Network, error)
	connectFn func(networkName, containerID string) error
	inspectFn func(name string) ([]entities.NetworkInspectReport, []error, error)
	removeFn  func(name string, force bool) error
	pruneFn   func() ([]*entities.NetworkPruneReport, error)
}

func (f *fakeNetworkEngine) List() ([]types.Network, error) {
	if f.listFn == nil {
		panic("unexpected NetworkEngine.List call")
	}
	return f.listFn()
}

func (f *fakeNetworkEngine) Create(name string) (*types.Network, error) {
	if f.createFn == nil {
		panic("unexpected NetworkEngine.Create call")
	}
	return f.createFn(name)
}

func (f *fakeNetworkEngine) Connect(networkName, containerID string) error {
	if f.connectFn == nil {
		panic("unexpected NetworkEngine.Connect call")
	}
	return f.connectFn(networkName, containerID)
}

func (f *fakeNetworkEngine) Inspect(name string) ([]entities.NetworkInspectReport, []error, error) {
	if f.inspectFn == nil {
		panic("unexpected NetworkEngine.Inspect call")
	}
	return f.inspectFn(name)
}

func (f *fakeNetworkEngine) Remove(name string, force bool) error {
	if f.removeFn == nil {
		panic("unexpected NetworkEngine.Remove call")
	}
	return f.removeFn(name, force)
}

func (f *fakeNetworkEngine) Prune() ([]*entities.NetworkPruneReport, error) {
	if f.pruneFn == nil {
		panic("unexpected NetworkEngine.Prune call")
	}
	return f.pruneFn()
}

// fakeEngineProvider arms the provider seam: engine is handed to the
// service closure; err simulates an acquisition failure (the 502 sentinel
// crossing) and short-circuits the closure, exactly like the production
// manager's warn-and-sentinel behaviour.
type fakeEngineProvider struct {
	engine NetworkEngine
	err    error
}

func (p fakeEngineProvider) WithEngine(ctx context.Context, userID, hostID, op string, fn func(e NetworkEngine) error) error {
	if p.err != nil {
		return p.err
	}
	return fn(p.engine)
}
