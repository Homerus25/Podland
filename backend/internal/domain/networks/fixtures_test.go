package networks

import (
	"time"

	"go.podman.io/common/libnetwork/types"
)

func mustParseNetworkTime(value string) time.Time {
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		panic("bad fixture timestamp: " + err.Error())
	}
	return parsed
}

// networkFixture builds a libnetwork Network like podman's list answer:
// name, driver, creation time, and subnets as CIDR strings.
func networkFixture(name, driver, createdAt string, subnets ...string) types.Network {
	parsed := make([]types.Subnet, 0, len(subnets))
	for _, cidr := range subnets {
		subnet, err := types.ParseCIDR(cidr)
		if err != nil {
			panic("bad fixture CIDR: " + err.Error())
		}
		parsed = append(parsed, types.Subnet{Subnet: subnet})
	}
	return types.Network{
		Name:    name,
		Driver:  driver,
		Created: mustParseNetworkTime(createdAt),
		Subnets: parsed,
	}
}

// newTestHandler builds the handler around the real service running over
// the given fake engine; tests arm only the operations their scenario
// needs (an unarmed operation panics if the handler reaches it).
func newTestHandler(engine *fakeNetworkEngine) *NetworkHandler {
	return NewNetworkHandler(NewNetworkService(fakeEngineProvider{engine: engine}))
}
