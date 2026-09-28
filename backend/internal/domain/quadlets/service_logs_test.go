package quadlets

import (
	"context"
	"net/http"
	"testing"
	"time"

	appErrors "github.com/Homerus25/podland/backend/internal/errors"
	"github.com/Homerus25/podland/backend/internal/streaming"
	"github.com/Homerus25/podland/backend/internal/testutils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.podman.io/podman/v6/pkg/domain/entities"
	errorhandling "go.podman.io/podman/v6/pkg/errorhandling"
)

// Service behaviour tests for the two logs modes: one-shot (the kit's
// FetchLogs owns the buffered ritual, so only the quadlet-to-container
// resolution and the not-found mapping are pinned here) and follow (the
// kit's FollowLogs owns the follow ritual, so only the resolution, the
// not-found mapping, and the promptness of the cancellation release are
// pinned here).

// TestQuadletService_GetLogs_ResolvesSystemdContainerName pins the payoff
// behaviour of the engine seam: for a container-type quadlet without an
// explicit name, logs are fetched from the systemd-spawned container
// (systemd-<basename>) and stdout/stderr combine into one multiline string.
func TestQuadletService_GetLogs_ResolvesSystemdContainerName(t *testing.T) {
	var loggedContainer string
	engine := &fakeQuadletEngine{
		listFn: func(entities.QuadletListOptions) ([]*entities.ListQuadlet, error) {
			return []*entities.ListQuadlet{{Name: "web.container", Status: "running"}}, nil
		},
		printFn: func(string) (string, error) {
			return "[Container]\nImage=nginx\n", nil
		},
		logsFn: func(containerName string, opts entities.ContainerLogsOptions) error {
			loggedContainer = containerName
			if opts.StdoutWriter != nil {
				_, _ = opts.StdoutWriter.Write([]byte("out line\n"))
			}
			if opts.StderrWriter != nil {
				_, _ = opts.StderrWriter.Write([]byte("err line\n"))
			}
			return nil
		},
	}
	service := NewQuadletService(fakeEngineProvider{engine: engine}, nil, nil)

	got, err := service.GetLogs(context.Background(), "user", "machine", "web.container", time.Time{}, 0)

	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "systemd-web", loggedContainer,
		"container-type quadlets must log from the systemd-spawned container")
	assert.Equal(t, "out line\nerr line", got.Logs, "stdout comes first, stderr appended, trailing newline trimmed")
	assert.Equal(t, "web.container", got.ID)
}

// TestQuadletService_GetLogs_ExplicitContainerNameWins pins that an
// explicit ContainerName= key in the unit content overrides the
// systemd-<basename> convention.
func TestQuadletService_GetLogs_ExplicitContainerNameWins(t *testing.T) {
	var loggedContainer string
	engine := &fakeQuadletEngine{
		listFn: func(entities.QuadletListOptions) ([]*entities.ListQuadlet, error) {
			return []*entities.ListQuadlet{{Name: "web.container"}}, nil
		},
		printFn: func(string) (string, error) {
			return "[Container]\nImage=nginx\nContainerName=my-explicit-container\n", nil
		},
		logsFn: func(containerName string, opts entities.ContainerLogsOptions) error {
			loggedContainer = containerName
			return nil
		},
	}
	service := NewQuadletService(fakeEngineProvider{engine: engine}, nil, nil)

	_, err := service.GetLogs(context.Background(), "user", "machine", "web.container", time.Time{}, 0)

	require.NoError(t, err)
	assert.Equal(t, "my-explicit-container", loggedContainer)
}

// TestQuadletService_GetLogs_PodQuadletUsesPodContainer pins the pod branch
// of the name resolution: a pod-type quadlet logs from the pod container
// reported by the list.
func TestQuadletService_GetLogs_PodQuadletUsesPodContainer(t *testing.T) {
	var loggedContainer string
	engine := &fakeQuadletEngine{
		listFn: func(entities.QuadletListOptions) ([]*entities.ListQuadlet, error) {
			return []*entities.ListQuadlet{{Name: "stack.pod", Pod: "stack-the-pod"}}, nil
		},
		printFn: func(string) (string, error) {
			return "[Pod]\n", nil
		},
		logsFn: func(containerName string, opts entities.ContainerLogsOptions) error {
			loggedContainer = containerName
			return nil
		},
	}
	service := NewQuadletService(fakeEngineProvider{engine: engine}, nil, nil)

	_, err := service.GetLogs(context.Background(), "user", "machine", "stack.pod", time.Time{}, 0)

	require.NoError(t, err)
	assert.Equal(t, "stack-the-pod", loggedContainer)
}

func TestQuadletService_GetLogs_MissingQuadletIs404BeforeLogs(t *testing.T) {
	engine := &fakeQuadletEngine{
		listFn: func(entities.QuadletListOptions) ([]*entities.ListQuadlet, error) {
			return nil, nil
		},
		// printFn/logsFn stay unarmed: a missing quadlet must not touch
		// the machine beyond the existence check.
	}
	service := NewQuadletService(fakeEngineProvider{engine: engine}, nil, nil)

	got, err := service.GetLogs(context.Background(), "user", "machine", "web.container", time.Time{}, 0)

	require.ErrorIs(t, err, appErrors.ErrQuadletNotFound)
	assert.Nil(t, got)
}

func TestQuadletService_GetLogs_MapsMissingContainerTo404(t *testing.T) {
	engine := &fakeQuadletEngine{
		listFn: func(entities.QuadletListOptions) ([]*entities.ListQuadlet, error) {
			return []*entities.ListQuadlet{{Name: "web.container"}}, nil
		},
		printFn: func(string) (string, error) {
			return "[Container]\n", nil
		},
		logsFn: func(string, entities.ContainerLogsOptions) error {
			return &errorhandling.ErrorModel{ResponseCode: http.StatusNotFound, Message: "no such container"}
		},
	}
	service := NewQuadletService(fakeEngineProvider{engine: engine}, nil, nil)

	got, err := service.GetLogs(context.Background(), "user", "machine", "web.container", time.Time{}, 0)

	require.ErrorIs(t, err, appErrors.ErrQuadletNotFound)
	assert.Nil(t, got)
}

func TestQuadletService_GetLogsWithStream_MapsMissingContainerTo404(t *testing.T) {
	engine := &fakeQuadletEngine{
		listFn: func(entities.QuadletListOptions) ([]*entities.ListQuadlet, error) {
			return []*entities.ListQuadlet{{Name: "web.container"}}, nil
		},
		printFn: func(string) (string, error) {
			return "[Container]\n", nil
		},
		logsFn: func(string, entities.ContainerLogsOptions) error {
			return &errorhandling.ErrorModel{ResponseCode: http.StatusNotFound, Message: "no such container"}
		},
	}
	service := NewQuadletService(fakeEngineProvider{engine: engine}, nil, nil)

	err := service.GetLogsWithStream(context.Background(), "user", "machine", "web.container", time.Time{}, 0,
		func(streaming.LogEvent) {})

	require.ErrorIs(t, err, appErrors.ErrQuadletNotFound)
}

// TestQuadletService_GetLogsWithStream_ReturnsPromptlyOnCancel pins the
// release regression: a blocking engine Logs call plus a cancelled
// context must return the operation-cancelled sentinel promptly (the
// shared promptness harness owns the ritual).
func TestQuadletService_GetLogsWithStream_ReturnsPromptlyOnCancel(t *testing.T) {
	started, block := testutils.CancelStubs(t)
	engine := &fakeQuadletEngine{
		listFn: func(entities.QuadletListOptions) ([]*entities.ListQuadlet, error) {
			return []*entities.ListQuadlet{{Name: "web.container"}}, nil
		},
		printFn: func(string) (string, error) {
			return "[Container]\nImage=nginx\n", nil
		},
		logsFn: func(string, entities.ContainerLogsOptions) error {
			close(started)
			<-block
			return nil
		},
	}
	service := NewQuadletService(fakeEngineProvider{engine: engine}, nil, nil)

	testutils.RequirePromptReturnOnCancel(t, "GetLogsWithStream", started,
		func(ctx context.Context) error {
			return service.GetLogsWithStream(ctx, "user", "machine", "web.container", time.Time{}, 0,
				func(streaming.LogEvent) {})
		})
}

// TestQuadletService_GetLogs_ResolveContainerName tests container name resolution
func TestQuadletService_GetLogs_ResolveContainerName(t *testing.T) {
	t.Run("container-type quadlet", func(t *testing.T) {
		quadlet := &entities.ListQuadlet{
			Name: "my-quadlet.container",
			Pod:  "", // No pod, so use the systemd-spawned container name
		}
		containerName := resolveManagedContainerName(quadlet, "my-quadlet.container")
		assert.Equal(t, "systemd-my-quadlet", containerName,
			"Container-type quadlet must resolve to the systemd-spawned container (podman names it systemd-<basename>)")
	})

	t.Run("pod-type quadlet with pod container", func(t *testing.T) {
		quadlet := &entities.ListQuadlet{
			Name: "my-pod",
			Pod:  "my-pod-container", // Use the pod container name
		}
		containerName := resolveManagedContainerName(quadlet, "my-pod")
		assert.Equal(t, "my-pod-container", containerName, "Pod-type quadlet should use pod container name")
	})

	t.Run("nil quadlet", func(t *testing.T) {
		containerName := resolveManagedContainerName(nil, "fallback-name")
		assert.Equal(t, "fallback-name", containerName, "Nil quadlet should fall back to provided name")
	})
}

// TestQuadletService_GetLogs_ExplicitContainerNameFromContent tests the
// ContainerName= extraction that overrides the systemd-<basename> convention.
func TestQuadletService_GetLogs_ExplicitContainerNameFromContent(t *testing.T) {
	t.Run("explicit ContainerName wins", func(t *testing.T) {
		content := `[Container]
Image=busybox
Exec=sh -c "while true; do echo hi; sleep 1; done"
ContainerName=my-explicit-container
`
		assert.Equal(t, "my-explicit-container", explicitContainerNameFromQuadletContent(content))
	})

	t.Run("absent key returns empty", func(t *testing.T) {
		content := `[Container]
Image=busybox
Exec=sleep 3600
`
		assert.Equal(t, "", explicitContainerNameFromQuadletContent(content))
	})

	t.Run("case-insensitive key, trimmed and unquoted value", func(t *testing.T) {
		content := "[Container]\ncontainername=  \"quoted-name\"  \n"
		assert.Equal(t, "quoted-name", explicitContainerNameFromQuadletContent(content))
	})

	t.Run("comments and lookalike keys are ignored", func(t *testing.T) {
		content := `[Container]
# ContainerName=commented-out
Label=this.looks=like=an=assignment
Exec=echo ContainerName=not-a-key
`
		assert.Equal(t, "", explicitContainerNameFromQuadletContent(content))
	})

	t.Run("empty value returns empty", func(t *testing.T) {
		content := "[Container]\nContainerName=\n"
		assert.Equal(t, "", explicitContainerNameFromQuadletContent(content))
	})
}
