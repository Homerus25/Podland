package quadlets

import (
	"context"
	"errors"
	"testing"

	appErrors "github.com/Homerus25/podland/backend/internal/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.podman.io/podman/v6/libpod/define"
	"go.podman.io/podman/v6/pkg/domain/entities"
)

// The generate module's own suite (architecture review 2026-10-19,
// candidate C7): the entry-point tests pin the dispatch's error vocabulary
// over the fake engine, the service Generate tests pin the seam crossing
// and DTO assembly, and the pure converter tests pin the inspect-field to
// quadlet-key mapping rules over inspect reports, without the engine seam.
// The handler contract stays pinned in handler_test.go; the endpoint
// contract stays pinned in test/integration/quadlets_generate_test.go.

// TestGenerate_ErrorMapping pins the module entry point's error vocabulary:
// the mapping lives in the module and keeps the domain's sentinels — an
// inspect failure or an empty report answers the resource's not-found
// sentinel, a conversion failure answers ErrQuadletGenerationFailed, and
// an unknown resource type answers ErrInvalidRequest. No new codes.
func TestConvertToQuadlet_ErrorMapping(t *testing.T) {
	t.Run("container inspect failure answers the container not-found sentinel", func(t *testing.T) {
		engine := &fakeQuadletEngine{
			inspectContainerFn: func(string) ([]*entities.ContainerInspectReport, []error, error) {
				return nil, nil, errors.New("no such container")
			},
		}

		_, _, err := ConvertToQuadlet(engine, resourceTypeContainer, "web")

		require.ErrorIs(t, err, appErrors.ErrContainerNotFound)
	})

	t.Run("empty container report answers the container not-found sentinel", func(t *testing.T) {
		engine := &fakeQuadletEngine{
			inspectContainerFn: func(string) ([]*entities.ContainerInspectReport, []error, error) {
				return nil, nil, nil
			},
		}

		_, _, err := ConvertToQuadlet(engine, resourceTypeContainer, "web")

		require.ErrorIs(t, err, appErrors.ErrContainerNotFound)
	})

	t.Run("container conversion failure answers the generation-failed sentinel", func(t *testing.T) {
		engine := &fakeQuadletEngine{
			inspectContainerFn: func(string) ([]*entities.ContainerInspectReport, []error, error) {
				// A report without a Config fails the converter.
				return []*entities.ContainerInspectReport{{InspectContainerData: &define.InspectContainerData{Name: "web"}}}, nil, nil
			},
		}

		_, _, err := ConvertToQuadlet(engine, resourceTypeContainer, "web")

		require.ErrorIs(t, err, appErrors.ErrQuadletGenerationFailed)
	})

	t.Run("pod inspect failure answers the pod not-found sentinel", func(t *testing.T) {
		engine := &fakeQuadletEngine{
			inspectPodFn: func(string) ([]*entities.PodInspectReport, []error, error) {
				return nil, nil, errors.New("no such pod")
			},
		}

		_, _, err := ConvertToQuadlet(engine, resourceTypePod, "stack")

		require.ErrorIs(t, err, appErrors.ErrPodNotFound)
	})

	t.Run("empty pod report answers the pod not-found sentinel", func(t *testing.T) {
		engine := &fakeQuadletEngine{
			inspectPodFn: func(string) ([]*entities.PodInspectReport, []error, error) {
				return nil, nil, nil
			},
		}

		_, _, err := ConvertToQuadlet(engine, resourceTypePod, "stack")

		require.ErrorIs(t, err, appErrors.ErrPodNotFound)
	})

	t.Run("pod conversion failure answers the generation-failed sentinel", func(t *testing.T) {
		engine := &fakeQuadletEngine{
			inspectPodFn: func(string) ([]*entities.PodInspectReport, []error, error) {
				// A report with an empty name fails the converter.
				return []*entities.PodInspectReport{{InspectPodData: &define.InspectPodData{}}}, nil, nil
			},
		}

		_, _, err := ConvertToQuadlet(engine, resourceTypePod, "stack")

		require.ErrorIs(t, err, appErrors.ErrQuadletGenerationFailed)
	})

	t.Run("unknown resource type answers the invalid-request sentinel without touching the engine", func(t *testing.T) {
		// The engine stays unarmed: an unknown resource type must be
		// refused without an inspect call.
		engine := &fakeQuadletEngine{}

		_, _, err := ConvertToQuadlet(engine, resourceType("invalid"), "x")

		require.ErrorIs(t, err, appErrors.ErrInvalidRequest)
	})
}

func TestQuadletService_Generate_FromContainer(t *testing.T) {
	engine := &fakeQuadletEngine{
		inspectContainerFn: func(string) ([]*entities.ContainerInspectReport, []error, error) {
			return []*entities.ContainerInspectReport{{
				InspectContainerData: &define.InspectContainerData{
					Name:   "web",
					Config: &define.InspectContainerConfig{Image: "nginx:latest"},
				},
			}}, nil, nil
		},
	}
	service := NewQuadletService(fakeEngineProvider{engine: engine}, nil, nil)

	got, err := service.Generate(context.Background(), "user", "host", QuadletGenerateRequest{
		ResourceID: "web", ResourceType: "container",
	})

	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "web", got.ID)
	assert.Contains(t, got.Content, "ContainerName=web")
	assert.Contains(t, got.Content, "Image=nginx:latest")
}

func TestQuadletService_Generate_FromPod(t *testing.T) {
	engine := &fakeQuadletEngine{
		inspectPodFn: func(string) ([]*entities.PodInspectReport, []error, error) {
			return []*entities.PodInspectReport{{
				InspectPodData: &define.InspectPodData{Name: "stack"},
			}}, nil, nil
		},
	}
	service := NewQuadletService(fakeEngineProvider{engine: engine}, nil, nil)

	got, err := service.Generate(context.Background(), "user", "host", QuadletGenerateRequest{
		ResourceID: "stack", ResourceType: "pod",
	})

	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "stack", got.ID)
	assert.Contains(t, got.Content, "PodName=stack")
}

func TestQuadletService_Generate_InvalidResourceType(t *testing.T) {
	// The engine stays unarmed: an invalid resource type must be refused
	// without touching the host.
	service := NewQuadletService(fakeEngineProvider{engine: &fakeQuadletEngine{}}, nil, nil)

	got, err := service.Generate(context.Background(), "user", "host", QuadletGenerateRequest{
		ResourceID: "x", ResourceType: "invalid",
	})

	require.ErrorIs(t, err, appErrors.ErrInvalidRequest)
	assert.Nil(t, got)
}

// --- Pure converters: the inspect-field to quadlet-key mapping rules ---

func TestGenerateContainerQuadlet(t *testing.T) {
	t.Run("generates valid container quadlet with all fields", func(t *testing.T) {
		inspect := &entities.ContainerInspectReport{
			InspectContainerData: &define.InspectContainerData{
				Name: "my-container",
				Config: &define.InspectContainerConfig{
					Image: "nginx:latest",
					Env:   []string{"PORT=8080", "DEBUG=false"},
					Labels: map[string]string{
						"version": "1.0",
						"app":     "web",
					},
				},
				HostConfig: &define.InspectContainerHostConfig{
					RestartPolicy: &define.InspectRestartPolicy{
						Name: "always",
					},
					PortBindings: map[string][]define.InspectHostPort{
						"80/tcp": {{HostPort: "8080"}},
					},
					Binds:       []string{"/host/path:/container/path"},
					NetworkMode: "bridge",
				},
			},
		}

		content, err := generateContainerQuadlet(inspect)

		assert.NoError(t, err, "generateContainerQuadlet should succeed for valid input")
		assert.Contains(t, content, "# Generated by Podland from container my-container")
		assert.Contains(t, content, "[Unit]")
		assert.Contains(t, content, "Description=Podman container my-container")
		assert.Contains(t, content, "[Container]")
		assert.Contains(t, content, "ContainerName=my-container")
		assert.Contains(t, content, "Image=nginx:latest")
		assert.Contains(t, content, "Environment=PORT=8080")
		assert.Contains(t, content, "Environment=DEBUG=false")
		assert.Contains(t, content, "Label=version=1.0")
		assert.Contains(t, content, "Label=app=web")
		assert.Contains(t, content, "RestartPolicy=always")
		assert.Contains(t, content, "PublishPort=8080:80/tcp")
		assert.Contains(t, content, "Volume=/host/path:/container/path")
		assert.Contains(t, content, "Network=bridge")
	})

	t.Run("handles minimal container config", func(t *testing.T) {
		inspect := &entities.ContainerInspectReport{
			InspectContainerData: &define.InspectContainerData{
				Name: "simple-container",
				Config: &define.InspectContainerConfig{
					Image: "alpine:latest",
				},
			},
		}

		content, err := generateContainerQuadlet(inspect)

		assert.NoError(t, err)
		assert.Contains(t, content, "ContainerName=simple-container")
		assert.Contains(t, content, "Image=alpine:latest")
		assert.NotContains(t, content, "Environment=")
		assert.NotContains(t, content, "RestartPolicy=")
		assert.NotContains(t, content, "PublishPort=")
		assert.NotContains(t, content, "Volume=")
		assert.NotContains(t, content, "Network=")
		assert.NotContains(t, content, "Label=")
	})

	t.Run("handles nil config", func(t *testing.T) {
		inspect := &entities.ContainerInspectReport{
			InspectContainerData: &define.InspectContainerData{
				Name:   "broken-container",
				Config: nil,
			},
		}

		content, err := generateContainerQuadlet(inspect)

		assert.Error(t, err, "generateContainerQuadlet should fail with nil config")
		assert.Empty(t, content)
	})

	t.Run("handles nil inspect report", func(t *testing.T) {
		content, err := generateContainerQuadlet(nil)

		assert.Error(t, err, "generateContainerQuadlet should fail with nil inspect report")
		assert.Empty(t, content)
	})

	t.Run("handles empty container name", func(t *testing.T) {
		inspect := &entities.ContainerInspectReport{
			InspectContainerData: &define.InspectContainerData{
				Name: "",
				Config: &define.InspectContainerConfig{
					Image: "nginx:latest",
				},
			},
		}

		content, err := generateContainerQuadlet(inspect)

		assert.Error(t, err, "generateContainerQuadlet should fail with empty name")
		assert.Empty(t, content)
	})

	t.Run("handles restart policy 'no'", func(t *testing.T) {
		inspect := &entities.ContainerInspectReport{
			InspectContainerData: &define.InspectContainerData{
				Name: "no-restart-container",
				Config: &define.InspectContainerConfig{
					Image: "nginx:latest",
				},
				HostConfig: &define.InspectContainerHostConfig{
					RestartPolicy: &define.InspectRestartPolicy{
						Name: "no",
					},
				},
			},
		}

		content, err := generateContainerQuadlet(inspect)

		assert.NoError(t, err)
		assert.NotContains(t, content, "RestartPolicy=")
	})

	t.Run("handles multiple port bindings", func(t *testing.T) {
		inspect := &entities.ContainerInspectReport{
			InspectContainerData: &define.InspectContainerData{
				Name: "multi-port-container",
				Config: &define.InspectContainerConfig{
					Image: "nginx:latest",
				},
				HostConfig: &define.InspectContainerHostConfig{
					PortBindings: map[string][]define.InspectHostPort{
						"80/tcp":  {{HostPort: "8080"}},
						"443/tcp": {{HostPort: "8443"}},
					},
				},
			},
		}

		content, err := generateContainerQuadlet(inspect)

		assert.NoError(t, err)
		assert.Contains(t, content, "PublishPort=")
	})

	t.Run("handles multiple volumes", func(t *testing.T) {
		inspect := &entities.ContainerInspectReport{
			InspectContainerData: &define.InspectContainerData{
				Name: "multi-volume-container",
				Config: &define.InspectContainerConfig{
					Image: "nginx:latest",
				},
				HostConfig: &define.InspectContainerHostConfig{
					Binds: []string{
						"/host1:/container1",
						"/host2:/container2",
					},
				},
			},
		}

		content, err := generateContainerQuadlet(inspect)

		assert.NoError(t, err)
		assert.Contains(t, content, "Volume=/host1:/container1")
		assert.Contains(t, content, "Volume=/host2:/container2")
	})
}

func TestGeneratePodQuadlet(t *testing.T) {
	t.Run("generates valid pod quadlet with all fields", func(t *testing.T) {
		inspect := &entities.PodInspectReport{
			InspectPodData: &define.InspectPodData{
				Name: "my-pod",
				CreateCommand: []string{
					"--name=my-pod",
					"--publish=8080:80",
				},
				Labels: map[string]string{
					"version": "1.0",
					"app":     "backend",
				},
			},
		}

		content, err := generatePodQuadlet(inspect)

		assert.NoError(t, err, "generatePodQuadlet should succeed for valid input")
		assert.Contains(t, content, "# Generated by Podland from pod my-pod")
		assert.Contains(t, content, "[Unit]")
		assert.Contains(t, content, "Description=Podman pod my-pod")
		assert.Contains(t, content, "[Pod]")
		assert.Contains(t, content, "PodName=my-pod")
		assert.Contains(t, content, "PodmanArgs=--name=my-pod --publish=8080:80")
		assert.Contains(t, content, "Label=version=1.0")
		assert.Contains(t, content, "Label=app=backend")
	})

	t.Run("handles minimal pod config", func(t *testing.T) {
		inspect := &entities.PodInspectReport{
			InspectPodData: &define.InspectPodData{
				Name: "simple-pod",
			},
		}

		content, err := generatePodQuadlet(inspect)

		assert.NoError(t, err)
		assert.Contains(t, content, "PodName=simple-pod")
		assert.NotContains(t, content, "PodmanArgs=")
		assert.NotContains(t, content, "Label=")
	})

	t.Run("handles nil inspect report", func(t *testing.T) {
		content, err := generatePodQuadlet(nil)

		assert.Error(t, err, "generatePodQuadlet should fail with nil inspect report")
		assert.Empty(t, content)
	})

	t.Run("handles empty pod name", func(t *testing.T) {
		inspect := &entities.PodInspectReport{
			InspectPodData: &define.InspectPodData{
				Name: "",
			},
		}

		content, err := generatePodQuadlet(inspect)

		assert.Error(t, err, "generatePodQuadlet should fail with empty name")
		assert.Empty(t, content)
	})

	t.Run("handles empty create command", func(t *testing.T) {
		inspect := &entities.PodInspectReport{
			InspectPodData: &define.InspectPodData{
				Name:          "no-command-pod",
				CreateCommand: []string{},
			},
		}

		content, err := generatePodQuadlet(inspect)

		assert.NoError(t, err)
		assert.NotContains(t, content, "PodmanArgs=")
	})

	t.Run("handles nil create command", func(t *testing.T) {
		inspect := &entities.PodInspectReport{
			InspectPodData: &define.InspectPodData{
				Name:          "nil-command-pod",
				CreateCommand: nil,
			},
		}

		content, err := generatePodQuadlet(inspect)

		assert.NoError(t, err)
		assert.NotContains(t, content, "PodmanArgs=")
	})

	t.Run("handles empty labels", func(t *testing.T) {
		inspect := &entities.PodInspectReport{
			InspectPodData: &define.InspectPodData{
				Name:   "no-labels-pod",
				Labels: map[string]string{},
			},
		}

		content, err := generatePodQuadlet(inspect)

		assert.NoError(t, err)
		assert.NotContains(t, content, "Label=")
	})

	t.Run("handles nil labels", func(t *testing.T) {
		inspect := &entities.PodInspectReport{
			InspectPodData: &define.InspectPodData{
				Name:   "nil-labels-pod",
				Labels: nil,
			},
		}

		content, err := generatePodQuadlet(inspect)

		assert.NoError(t, err)
		assert.NotContains(t, content, "Label=")
	})
}
