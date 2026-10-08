package networks

import (
	"context"

	"go.podman.io/common/libnetwork/types"
)

// fakeNetworkEngine is the function-field fake for the NetworkEngine
// seam. Each test arms only the operations its scenario needs; an
// unarmed operation panics, so an unexpected engine call fails the test
// loudly.
type fakeNetworkEngine struct {
	listFn func() ([]types.Network, error)
}

func (f *fakeNetworkEngine) List() ([]types.Network, error) {
	if f.listFn == nil {
		panic("unexpected NetworkEngine.List call")
	}
	return f.listFn()
}

// fakeEngineProvider arms the provider seam: engine is handed to the
// service closure; err simulates an acquisition failure (the 502 sentinel
// crossing) and short-circuits the closure, exactly like the production
// manager's warn-and-sentinel behaviour.
type fakeEngineProvider struct {
	engine NetworkEngine
	err    error
}

func (p fakeEngineProvider) WithEngine(ctx context.Context, userID, machineID, op string, fn func(e NetworkEngine) error) error {
	if p.err != nil {
		return p.err
	}
	return fn(p.engine)
}
