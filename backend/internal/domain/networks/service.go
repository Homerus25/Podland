package networks

import (
	"context"
	"fmt"

	appErrors "github.com/Homerus25/podland/backend/internal/errors"
	"github.com/Homerus25/podland/backend/internal/inspect"
	"github.com/Homerus25/podland/backend/internal/podmanerr"
	"go.podman.io/common/libnetwork/types"
	"go.podman.io/podman/v6/pkg/domain/entities"
)

// NetworkService provides business logic for networks. Networks live
// only in the remote machine's podman; this service proxies podman
// directly and persists nothing locally, matching VolumeService/
// SecretService. Every machine-scoped podman call crosses the
// NetworkEngineProvider (the connection seam); the service itself owns
// only domain mapping.
type NetworkService struct {
	engines NetworkEngineProvider
}

// NewNetworkService creates a new NetworkService
func NewNetworkService(engines NetworkEngineProvider) *NetworkService {
	return &NetworkService{
		engines: engines,
	}
}

// List returns every network on the specified machine — everything
// podman reports, including the default bridge network; no filtering.
func (s *NetworkService) List(ctx context.Context, userID, machineID string) ([]types.Network, error) {
	var networks []types.Network
	err := s.engines.WithEngine(ctx, userID, machineID, "networks.List", func(e NetworkEngine) error {
		n, err := e.List()
		if err != nil {
			return fmt.Errorf("failed to list networks: %w", err)
		}
		networks = n
		return nil
	})
	return networks, err
}

// Create makes a network by name on the specified machine and returns
// the created network; podman's defaults fill driver and subnet. When
// the request lists containers, each is attached after the create — an
// attach failure fails the call naming the failing container, and the
// network stays created (no rollback). A duplicate name surfaces as 409
// NETWORK_ALREADY_EXISTS.
func (s *NetworkService) Create(ctx context.Context, userID, machineID string, req NetworkCreateRequest) (*types.Network, error) {
	var created *types.Network
	err := s.engines.WithEngine(ctx, userID, machineID, "networks.Create", func(e NetworkEngine) error {
		network, err := e.Create(req.Name)
		if err != nil {
			if appErr := mapCreateError(err); appErr != nil {
				return appErr
			}
			return fmt.Errorf("failed to create network: %w", err)
		}
		for _, containerID := range req.Containers {
			if err := e.Connect(network.Name, containerID); err != nil {
				return mapAttachError(err, containerID)
			}
		}
		created = network
		return nil
	})
	return created, err
}

// Connect attaches a container (by ID) to a network by name on the
// specified machine. An unknown network surfaces as 404
// NETWORK_NOT_FOUND; an unknown container as 404 CONTAINER_NOT_FOUND.
func (s *NetworkService) Connect(ctx context.Context, userID, machineID, networkName, containerID string) error {
	return s.engines.WithEngine(ctx, userID, machineID, "networks.Connect", func(e NetworkEngine) error {
		// Resolve the network first: podman answers an unknown network
		// and an unknown container with the same 404 shape, and the
		// contract gives each its own code.
		networks, err := e.List()
		if err != nil {
			return fmt.Errorf("failed to list networks: %w", err)
		}
		found := false
		for _, network := range networks {
			if network.Name == networkName {
				found = true
				break
			}
		}
		if !found {
			return appErrors.ErrNetworkNotFound
		}
		if err := e.Connect(networkName, containerID); err != nil {
			if appErr := mapConnectError(err); appErr != nil {
				return appErr
			}
			return fmt.Errorf("failed to connect container to network: %w", err)
		}
		return nil
	})
}

// Inspect returns the raw podman inspect report for one network on the
// specified machine, unmapped — the endpoint answers podman's raw
// inspect JSON. A missing network surfaces as 404 NETWORK_NOT_FOUND.
func (s *NetworkService) Inspect(ctx context.Context, userID, machineID, networkName string) (entities.NetworkInspectReport, error) {
	var report entities.NetworkInspectReport
	err := s.engines.WithEngine(ctx, userID, machineID, "networks.Inspect", func(e NetworkEngine) error {
		inspectReport, err := inspectNetwork(e, networkName)
		if err != nil {
			return err
		}

		report = inspectReport
		return nil
	})
	return report, err
}

// inspectNetwork resolves one network's inspect report through the
// shared single-report ritual, classifying podman's not-found shapes
// into ErrNetworkNotFound.
func inspectNetwork(e NetworkEngine, name string) (entities.NetworkInspectReport, error) {
	reports, inspectErrs, err := e.Inspect(name)
	return inspect.Resolve(reports, inspectErrs, err, appErrors.ErrNetworkNotFound, "network", name)
}

// mapCreateError converts podman's create rejections into app errors.
// The shape mirrors the volumes domain's create mapping, but the two
// deliberately diverge on the duplicate-name branch: volumes folds a
// duplicate name into 400 INVALID_REQUEST, while the networks contract
// pins 409 NETWORK_ALREADY_EXISTS (spec #251, decision 3), so the
// already-exists classification runs most specific first. Any other
// podman-side 4xx means the request itself was bad (reserved or
// invalid name), so it surfaces as 400 INVALID_REQUEST carrying
// podman's message.
func mapCreateError(err error) *appErrors.AppError {
	if podmanerr.IsAlreadyExists(err) {
		return appErrors.ErrNetworkAlreadyExists
	}
	if podmanerr.IsClientError(err) || podmanerr.IsConflict(err) || podmanerr.IsNotFound(err) {
		return appErrors.ErrInvalidRequest.WithMessage(podmanerr.Message(err))
	}
	return nil
}

// mapConnectError converts podman's connect rejections into app errors,
// mirroring the volumes domain's create mapping. An unknown container
// is the 404 the contract promises; any other podman-side 4xx means
// the attach request itself was bad, so it surfaces as 400
// INVALID_REQUEST carrying podman's message.
func mapConnectError(err error) *appErrors.AppError {
	if podmanerr.IsNotFound(err) {
		return appErrors.ErrContainerNotFound
	}
	if podmanerr.IsClientError(err) || podmanerr.IsConflict(err) {
		return appErrors.ErrInvalidRequest.WithMessage(podmanerr.Message(err))
	}
	return nil
}

// mapAttachError converts a podman attach rejection on the
// create-with-attach path into the envelope's answer. The network is
// already created at this point and stays created (no rollback); the
// answer names the failing container so the dialog can say what
// happened. An unknown container maps to the shared 404; any other
// rejection becomes 409 NETWORK_ATTACH_FAILED carrying podman's
// message.
func mapAttachError(err error, containerID string) *appErrors.AppError {
	if podmanerr.IsNotFound(err) {
		return appErrors.ErrContainerNotFound.WithMessage(fmt.Sprintf("container %q: %s", containerID, podmanerr.Message(err)))
	}
	return appErrors.ErrNetworkAttachFailed.WithMessage(fmt.Sprintf("failed to attach container %q: %s", containerID, podmanerr.Message(err)))
}
