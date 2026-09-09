package quadlets

import (
	"archive/tar"
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"testing"

	appErrors "github.com/Homerus25/podland/backend/internal/errors"
	"github.com/Homerus25/podland/backend/internal/testutils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.podman.io/podman/v6/libpod/define"
	"go.podman.io/podman/v6/pkg/domain/entities"
	errorhandling "go.podman.io/podman/v6/pkg/errorhandling"
)

// Service behaviour tests run the real QuadletService against the fake
// engine, so the composition rules (Update's remove+install, Delete's
// precondition ordering, GetLogs' name resolution) are pinned as behaviour,
// not invocation shape. Connection acquisition (and its 502 mapping) is the
// provider seam: the connections package owns that contract and pins it in
// its own tests. The domain logic on pure helpers keeps its direct tests
// further below.

// TestNewQuadletService_WithEngineProvider verifies that QuadletService can
// be constructed with the production engine provider, enabling unit testing.
func TestNewQuadletService_WithEngineProvider(t *testing.T) {
	mockConnManager := testutils.NewMockConnectionManager()

	service := NewQuadletService(NewQuadletEngineProvider(mockConnManager), nil)

	assert.NotNil(t, service)
	assert.NotNil(t, service.engines)
}

// TestQuadletService_ImplementsInterface is a compile-time check that
// *QuadletService satisfies QuadletServiceInterface.
func TestQuadletService_ImplementsInterface(t *testing.T) {
	var _ QuadletServiceInterface = (*QuadletService)(nil)
}

func TestQuadletService_List_ReturnsFilteredQuadlets(t *testing.T) {
	engine := &fakeQuadletEngine{
		listFn: func(entities.QuadletListOptions) ([]*entities.ListQuadlet, error) {
			return []*entities.ListQuadlet{
				{Name: "alpha.container", UnitName: "alpha.service", Path: "/x/alpha.container", Status: "running"},
				{Name: "bravo.pod", UnitName: "bravo.service", Path: "/x/bravo.pod", Status: "stopped"},
			}, nil
		},
	}
	service := NewQuadletService(fakeEngineProvider{engine: engine}, nil)

	got, err := service.List(context.Background(), "user", "machine", ListFilters{Resource: "pod"})

	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, "bravo.pod", got[0].Name)
	assert.Equal(t, "stopped", got[0].State)
}

func TestQuadletService_List_WrapsEngineFailure(t *testing.T) {
	engine := &fakeQuadletEngine{
		listFn: func(entities.QuadletListOptions) ([]*entities.ListQuadlet, error) {
			return nil, errors.New("boom")
		},
	}
	service := NewQuadletService(fakeEngineProvider{engine: engine}, nil)

	got, err := service.List(context.Background(), "user", "machine", ListFilters{})

	require.Error(t, err)
	assert.Nil(t, got)
	assert.Contains(t, err.Error(), "failed to list quadlets", "engine failures must be wrapped consistently")
}

func TestQuadletService_Get_ReturnsContentWithMetadata(t *testing.T) {
	engine := &fakeQuadletEngine{
		printFn: func(string) (string, error) {
			return "[Container]\nImage=nginx\n", nil
		},
		listFn: func(entities.QuadletListOptions) ([]*entities.ListQuadlet, error) {
			return []*entities.ListQuadlet{{Name: "web.container", UnitName: "web.service", Path: "/x/web.container", Status: "running"}}, nil
		},
	}
	service := NewQuadletService(fakeEngineProvider{engine: engine}, nil)

	got, err := service.Get(context.Background(), "user", "machine", "web.container")

	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "[Container]\nImage=nginx\n", got.Content)
	assert.Equal(t, "web.service", got.Unit)
	assert.Equal(t, "container", got.Resource)
	assert.Equal(t, "running", got.State)
}

func TestQuadletService_Get_MissingQuadletIs404(t *testing.T) {
	engine := &fakeQuadletEngine{
		printFn: func(string) (string, error) {
			return "", nil
		},
		listFn: func(entities.QuadletListOptions) ([]*entities.ListQuadlet, error) {
			return nil, nil
		},
	}
	service := NewQuadletService(fakeEngineProvider{engine: engine}, nil)

	got, err := service.Get(context.Background(), "user", "machine", "web.container")

	require.ErrorIs(t, err, appErrors.ErrQuadletNotFound)
	assert.Nil(t, got)
}

func TestQuadletService_Create_InstallsUnderResourceExtension(t *testing.T) {
	var gotFile, gotContent string
	var gotReplace bool
	engine := &fakeQuadletEngine{
		installFn: func(fileName string, content []byte, replace bool) (*entities.QuadletInstallReport, error) {
			gotFile, gotContent, gotReplace = fileName, string(content), replace
			return &entities.QuadletInstallReport{
				InstalledQuadlets: map[string]string{"/tmp/web.container": "/etc/containers/systemd/web.container"},
			}, nil
		},
	}
	service := NewQuadletService(fakeEngineProvider{engine: engine}, nil)

	got, err := service.Create(context.Background(), "user", "machine", QuadletCreateRequest{
		Name:     "web",
		Resource: "container",
		Content:  "[Container]\nImage=nginx\n",
	})

	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "web.container", gotFile, "the file must be installed under the resource extension")
	assert.Equal(t, "[Container]\nImage=nginx\n", gotContent)
	assert.False(t, gotReplace, "create must not replace an existing installation")
	assert.Equal(t, "/etc/containers/systemd/web.container", got.Path)
	assert.Equal(t, "unknown", got.State)
}

func TestQuadletService_Create_MapsAlreadyExists(t *testing.T) {
	t.Run("transport-level refusal", func(t *testing.T) {
		engine := &fakeQuadletEngine{
			installFn: func(string, []byte, bool) (*entities.QuadletInstallReport, error) {
				return nil, &errorhandling.ErrorModel{
					ResponseCode: http.StatusInternalServerError,
					Message:      "web.container: conflict",
					Because:      "name in use",
				}
			},
		}
		service := NewQuadletService(fakeEngineProvider{engine: engine}, nil)

		got, err := service.Create(context.Background(), "user", "machine", QuadletCreateRequest{
			Name: "web", Resource: "container", Content: "c",
		})

		require.ErrorIs(t, err, appErrors.ErrQuadletAlreadyExists)
		assert.Nil(t, got)
	})

	t.Run("per-file refusal in the install report", func(t *testing.T) {
		engine := &fakeQuadletEngine{
			installFn: func(string, []byte, bool) (*entities.QuadletInstallReport, error) {
				return &entities.QuadletInstallReport{
					QuadletErrors: map[string]error{
						"web.container": errors.New("web.container: already exists"),
					},
				}, nil
			},
		}
		service := NewQuadletService(fakeEngineProvider{engine: engine}, nil)

		got, err := service.Create(context.Background(), "user", "machine", QuadletCreateRequest{
			Name: "web", Resource: "container", Content: "c",
		})

		require.ErrorIs(t, err, appErrors.ErrQuadletAlreadyExists)
		assert.Nil(t, got)
	})
}

func TestQuadletService_Update_ReplacesViaRemoveThenInstall(t *testing.T) {
	var order []string
	var removedName string
	var removedForce bool
	var installedFile, installedContent string
	var installedReplace bool
	engine := &fakeQuadletEngine{
		listFn: func(entities.QuadletListOptions) ([]*entities.ListQuadlet, error) {
			order = append(order, "list")
			return []*entities.ListQuadlet{{
				Name:     "web.container",
				UnitName: "web.service",
				Path:     "/etc/containers/systemd/web.container",
				Status:   "stopped",
			}}, nil
		},
		removeFn: func(name string, force bool) (*entities.QuadletRemoveReport, error) {
			order = append(order, "remove")
			removedName, removedForce = name, force
			return &entities.QuadletRemoveReport{}, nil
		},
		installFn: func(fileName string, content []byte, replace bool) (*entities.QuadletInstallReport, error) {
			order = append(order, "install")
			installedFile, installedContent, installedReplace = fileName, string(content), replace
			return &entities.QuadletInstallReport{
				InstalledQuadlets: map[string]string{"/tmp/web.container": "/etc/containers/systemd/web.container"},
			}, nil
		},
	}
	service := NewQuadletService(fakeEngineProvider{engine: engine}, nil)

	got, err := service.Update(context.Background(), "user", "machine", "web.container", QuadletUpdateRequest{
		Content: "[Container]\nImage=nginx:1.25\n",
	})

	require.NoError(t, err)
	assert.Equal(t, []string{"list", "remove", "install"}, order, "update must locate, remove, then install")
	assert.Equal(t, "web.container", removedName)
	assert.True(t, removedForce, "the remove half of an update must be forced (PUT has no running-conflict answer)")
	assert.Equal(t, "web.container", installedFile, "the rebuilt file must keep the original resource extension")
	assert.Equal(t, "[Container]\nImage=nginx:1.25\n", installedContent)
	assert.True(t, installedReplace, "the install half must replace any leftover from a partial state")
	require.NotNil(t, got)
	assert.Equal(t, "web.service", got.Unit)
	assert.Equal(t, "[Container]\nImage=nginx:1.25\n", got.Content)
	assert.Equal(t, "unknown", got.State)
}

// TestQuadletService_Update_InstallFailureLeavesQuadletRemoved pins the
// documented non-atomicity of the remove+install composition: when the
// install half fails after the remove half succeeded, the quadlet is gone
// and the service surfaces the install failure without attempting any
// rollback. Making Update atomic is a future decision.
func TestQuadletService_Update_InstallFailureLeavesQuadletRemoved(t *testing.T) {
	engine := &fakeQuadletEngine{
		listFn: func(entities.QuadletListOptions) ([]*entities.ListQuadlet, error) {
			return []*entities.ListQuadlet{{
				Name:     "web.container",
				UnitName: "web.service",
				Path:     "/etc/containers/systemd/web.container",
				Status:   "stopped",
			}}, nil
		},
		removeFn: func(string, bool) (*entities.QuadletRemoveReport, error) {
			return &entities.QuadletRemoveReport{}, nil
		},
		installFn: func(string, []byte, bool) (*entities.QuadletInstallReport, error) {
			return nil, errors.New("systemd reload failed")
		},
	}
	service := NewQuadletService(fakeEngineProvider{engine: engine}, nil)

	got, err := service.Update(context.Background(), "user", "machine", "web.container", QuadletUpdateRequest{
		Content: "[Container]\nImage=nginx:1.25\n",
	})

	require.Error(t, err)
	assert.Nil(t, got)
	assert.Contains(t, err.Error(), "failed to install updated quadlet")
}

func TestQuadletService_Update_MissingQuadletIs404BeforeAnyMutation(t *testing.T) {
	engine := &fakeQuadletEngine{
		listFn: func(entities.QuadletListOptions) ([]*entities.ListQuadlet, error) {
			return nil, nil
		},
		// removeFn/installFn stay unarmed: a missing quadlet must not
		// touch the machine.
	}
	service := NewQuadletService(fakeEngineProvider{engine: engine}, nil)

	got, err := service.Update(context.Background(), "user", "machine", "web.container", QuadletUpdateRequest{
		Content: "c",
	})

	require.ErrorIs(t, err, appErrors.ErrQuadletNotFound)
	assert.Nil(t, got)
}

func TestQuadletService_Delete_PreconditionsRunBeforeRemove(t *testing.T) {
	t.Run("missing quadlet is 404 before remove", func(t *testing.T) {
		engine := &fakeQuadletEngine{
			listFn: func(entities.QuadletListOptions) ([]*entities.ListQuadlet, error) {
				return nil, nil
			},
			// removeFn stays unarmed.
		}
		service := NewQuadletService(fakeEngineProvider{engine: engine}, nil)

		err := service.Delete(context.Background(), "user", "machine", "web.container", false, true)

		require.ErrorIs(t, err, appErrors.ErrQuadletNotFound)
	})

	t.Run("running without force is refused before remove", func(t *testing.T) {
		engine := &fakeQuadletEngine{
			listFn: func(entities.QuadletListOptions) ([]*entities.ListQuadlet, error) {
				return []*entities.ListQuadlet{{Name: "web.container", Status: "running (5s ago)"}}, nil
			},
			// removeFn stays unarmed.
		}
		service := NewQuadletService(fakeEngineProvider{engine: engine}, nil)

		err := service.Delete(context.Background(), "user", "machine", "web.container", false, true)

		require.ErrorIs(t, err, appErrors.ErrQuadletResourceRunning)
	})

	t.Run("running with force proceeds to remove", func(t *testing.T) {
		var removedForce bool
		engine := &fakeQuadletEngine{
			listFn: func(entities.QuadletListOptions) ([]*entities.ListQuadlet, error) {
				return []*entities.ListQuadlet{{Name: "web.container", Status: "running (5s ago)"}}, nil
			},
			removeFn: func(name string, force bool) (*entities.QuadletRemoveReport, error) {
				removedForce = force
				return &entities.QuadletRemoveReport{}, nil
			},
		}
		service := NewQuadletService(fakeEngineProvider{engine: engine}, nil)

		err := service.Delete(context.Background(), "user", "machine", "web.container", true, true)

		require.NoError(t, err)
		assert.True(t, removedForce)
	})
}

func TestQuadletService_Delete_MapsRemoveRefusals(t *testing.T) {
	t.Run("podman's running refusal maps to the 400 sentinel", func(t *testing.T) {
		engine := &fakeQuadletEngine{
			listFn: func(entities.QuadletListOptions) ([]*entities.ListQuadlet, error) {
				return []*entities.ListQuadlet{{Name: "web.container", Status: "stopped"}}, nil
			},
			removeFn: func(string, bool) (*entities.QuadletRemoveReport, error) {
				return nil, errors.New("quadlet web.container is running and force is not set, refusing to remove")
			},
		}
		service := NewQuadletService(fakeEngineProvider{engine: engine}, nil)

		err := service.Delete(context.Background(), "user", "machine", "web.container", false, true)

		require.ErrorIs(t, err, appErrors.ErrQuadletResourceRunning)
	})

	t.Run("per-name report refusal maps too", func(t *testing.T) {
		engine := &fakeQuadletEngine{
			listFn: func(entities.QuadletListOptions) ([]*entities.ListQuadlet, error) {
				return []*entities.ListQuadlet{{Name: "web.container", Status: "stopped"}}, nil
			},
			removeFn: func(string, bool) (*entities.QuadletRemoveReport, error) {
				return &entities.QuadletRemoveReport{
					Errors: map[string]error{
						"web.container": &errorhandling.ErrorModel{ResponseCode: http.StatusNotFound, Message: "no such quadlet"},
					},
				}, nil
			},
		}
		service := NewQuadletService(fakeEngineProvider{engine: engine}, nil)

		err := service.Delete(context.Background(), "user", "machine", "web.container", false, true)

		require.ErrorIs(t, err, appErrors.ErrQuadletNotFound)
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
	service := NewQuadletService(fakeEngineProvider{engine: engine}, nil)

	got, err := service.Generate(context.Background(), "user", "machine", QuadletGenerateRequest{
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
	service := NewQuadletService(fakeEngineProvider{engine: engine}, nil)

	got, err := service.Generate(context.Background(), "user", "machine", QuadletGenerateRequest{
		ResourceID: "stack", ResourceType: "pod",
	})

	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "stack", got.ID)
	assert.Contains(t, got.Content, "PodName=stack")
}

func TestQuadletService_Generate_InvalidResourceType(t *testing.T) {
	// The engine stays unarmed: an invalid resource type must be refused
	// without touching the machine.
	service := NewQuadletService(fakeEngineProvider{engine: &fakeQuadletEngine{}}, nil)

	got, err := service.Generate(context.Background(), "user", "machine", QuadletGenerateRequest{
		ResourceID: "x", ResourceType: "invalid",
	})

	require.ErrorIs(t, err, appErrors.ErrInvalidRequest)
	assert.Nil(t, got)
}

// TestQuadletService_AcquireFailureIsThe502Sentinel pins the service-side
// half of the connection-failure contract: the provider's sentinel crosses
// untouched — the service never wraps, maps, or swallows it (the 502
// mapping itself is pinned in the connections package).
func TestQuadletService_AcquireFailureIsThe502Sentinel(t *testing.T) {
	service := NewQuadletService(fakeEngineProvider{err: appErrors.ErrConnectionFailed}, nil)

	got, err := service.List(context.Background(), "user", "machine", ListFilters{})

	require.ErrorIs(t, err, appErrors.ErrConnectionFailed)
	assert.Nil(t, got)
}

// --- Internal seams: pure domain logic ---
//
// These functions are the part of the implementation that carries real domain
// rules (resource derivation from file suffix, systemd-status normalization,
// and filtering). They are tested directly through their own surface rather than
// through the service, so coverage of the meaningful logic is preserved.

func TestApplyFilters(t *testing.T) {
	quadlets := []QuadletListResponse{
		{ID: "a.container", Name: "alpha.container", Resource: "container", State: "running"},
		{ID: "b.pod", Name: "bravo.pod", Resource: "pod", State: "stopped"},
		{ID: "c.container", Name: "charlie.container", Resource: "container", State: "error"},
	}

	t.Run("no filters returns all", func(t *testing.T) {
		got := applyFilters(quadlets, ListFilters{})
		assert.Len(t, got, 3)
	})

	t.Run("filter by resource", func(t *testing.T) {
		got := applyFilters(quadlets, ListFilters{Resource: "container"})
		assert.Len(t, got, 2)
		for _, q := range got {
			assert.Equal(t, "container", q.Resource)
		}
	})

	t.Run("filter by name is case-insensitive substring", func(t *testing.T) {
		got := applyFilters(quadlets, ListFilters{Name: "POD"})
		assert.Len(t, got, 1)
		assert.Equal(t, "bravo.pod", got[0].Name)
	})

	t.Run("resource and name combined", func(t *testing.T) {
		got := applyFilters(quadlets, ListFilters{Resource: "container", Name: "char"})
		assert.Len(t, got, 1)
		assert.Equal(t, "charlie.container", got[0].Name)
	})

	t.Run("no match returns empty non-nil", func(t *testing.T) {
		got := applyFilters(quadlets, ListFilters{Name: "zzz"})
		assert.Len(t, got, 0)
	})

	t.Run("empty input returns empty", func(t *testing.T) {
		got := applyFilters([]QuadletListResponse{}, ListFilters{Resource: "container"})
		assert.Len(t, got, 0)
	})
}

func TestFromListQuadlet(t *testing.T) {
	t.Run("nil returns zero value", func(t *testing.T) {
		got := FromListQuadlet(nil)
		assert.Equal(t, QuadletListResponse{}, got)
	})

	t.Run("maps fields and derives resource/state", func(t *testing.T) {
		got := FromListQuadlet(&entities.ListQuadlet{
			Name:     "my-container.container",
			UnitName: "my-container.service",
			Path:     "/etc/containers/systemd/my-container.container",
			Status:   "running (12s ago)",
		})
		assert.Equal(t, QuadletListResponse{
			ID:       "my-container.container",
			Name:     "my-container.container",
			Path:     "/etc/containers/systemd/my-container.container",
			Unit:     "my-container.service",
			Resource: "container",
			State:    "running",
			Raw:      "",
		}, got)
	})
}

func TestToQuadletListResponse(t *testing.T) {
	t.Run("empty slice", func(t *testing.T) {
		got := ToQuadletListResponse([]*entities.ListQuadlet{})
		assert.Len(t, got, 0)
	})

	t.Run("maps each element preserving order", func(t *testing.T) {
		in := []*entities.ListQuadlet{
			{Name: "a.container", UnitName: "a.service", Path: "/x/a.container", Status: "running"},
			{Name: "b.pod", UnitName: "b.service", Path: "/x/b.pod", Status: "stopped"},
		}
		got := ToQuadletListResponse(in)
		assert.Len(t, got, 2)
		assert.Equal(t, "a.container", got[0].Name)
		assert.Equal(t, "container", got[0].Resource)
		assert.Equal(t, "running", got[0].State)
		assert.Equal(t, "b.pod", got[1].Name)
		assert.Equal(t, "pod", got[1].Resource)
		assert.Equal(t, "stopped", got[1].State)
	})
}

func TestDeriveResourceFromPath(t *testing.T) {
	tests := []struct {
		path string
		want string
	}{
		{"/etc/containers/systemd/web.container", "container"},
		{"/etc/containers/systemd/grp.pod", "pod"},
		{"/etc/containers/systemd/img.image", "image"},
		{"/etc/containers/systemd/dat.volume", "volume"},
		{"/etc/containers/systemd/net.network", "network"},
		{"/etc/containers/systemd/k8s.kube", "container"},        // kube -> container
		{"/etc/containers/systemd/bld.build", "image"},           // build -> image
		{"/etc/containers/systemd/UPPER.CONTAINER", "container"}, // case-insensitive
		{"/etc/containers/systemd/weird.txt", "unknown"},
		{"", "unknown"},
	}
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			assert.Equal(t, tt.want, deriveResourceFromPath(tt.path))
		})
	}
}

func TestParseStateFromStatus(t *testing.T) {
	tests := []struct {
		status string
		want   string
	}{
		{"running (12s ago)", "running"},
		{"STOPPED", "stopped"},
		{"active (running) since...", "running"},
		{"failed", "error"},
		{"inactive (error)", "error"},
		{"activating", "unknown"},
		{"", "unknown"},
	}
	for _, tt := range tests {
		t.Run(tt.status, func(t *testing.T) {
			assert.Equal(t, tt.want, parseStateFromStatus(tt.status))
		})
	}
}

// --- Pure helpers used by Create ---

func TestResourceToExtension(t *testing.T) {
	tests := []struct {
		resource string
		want     string
	}{
		{"container", ".container"},
		{"pod", ".pod"},
		{"image", ".image"},
		{"volume", ".volume"},
		{"network", ".network"},
		{"CONTAINER", ".container"}, // case-insensitive
		{"Pod", ".pod"},
		{"", ".container"},      // empty defaults to container
		{"kube", ".container"},  // not in contract enum → default
		{"build", ".container"}, // not in contract enum → default
		{"bogus", ".container"}, // unknown → default
	}
	for _, tt := range tests {
		t.Run(tt.resource, func(t *testing.T) {
			assert.Equal(t, tt.want, resourceToExtension(tt.resource))
		})
	}
}

func TestCreateTarBody(t *testing.T) {
	t.Run("produces a valid single-file tar with the requested name and content", func(t *testing.T) {
		fileName := "my-container.container"
		content := []byte("[Container]\nImage=nginx:latest\n")

		reader, err := createTarBody(fileName, content)
		assert.NoError(t, err, "createTarBody should not error for valid input")

		got, err := io.ReadAll(reader)
		assert.NoError(t, err)

		tr := tar.NewReader(bytes.NewReader(got))

		hdr, err := tr.Next()
		assert.NoError(t, err, "Should yield at least one header")
		assert.Equal(t, fileName, hdr.Name, "Tar entry name should match the requested filename")
		assert.Equal(t, int64(len(content)), hdr.Size, "Header size should match content length")

		body, err := io.ReadAll(tr)
		assert.NoError(t, err)
		assert.Equal(t, content, body, "Tar entry body should equal the input content")

		// No further entries beyond EOF.
		_, err = tr.Next()
		assert.ErrorIs(t, err, io.EOF, "Tar should contain exactly one entry")
	})

	t.Run("empty content still produces a valid (zero-length) tar entry", func(t *testing.T) {
		reader, err := createTarBody("empty.container", []byte{})
		assert.NoError(t, err)

		got, err := io.ReadAll(reader)
		assert.NoError(t, err)

		tr := tar.NewReader(bytes.NewReader(got))
		hdr, err := tr.Next()
		assert.NoError(t, err)
		assert.Equal(t, "empty.container", hdr.Name)
		assert.Equal(t, int64(0), hdr.Size)
		_, err = tr.Next()
		assert.ErrorIs(t, err, io.EOF)
	})
}

// --- Pure helpers used by Create + Update ---

func TestFirstInstalledPath(t *testing.T) {
	t.Run("nil map returns empty", func(t *testing.T) {
		assert.Equal(t, "", firstInstalledPath(nil))
	})

	t.Run("empty map returns empty", func(t *testing.T) {
		assert.Equal(t, "", firstInstalledPath(map[string]string{}))
	})

	t.Run("populated map returns one of the destinations", func(t *testing.T) {
		// Map ordering is non-deterministic; the contract is "a populated path".
		installed := map[string]string{
			"/tmp/x.container": "/etc/containers/systemd/x.container",
		}
		assert.Equal(t, "/etc/containers/systemd/x.container", firstInstalledPath(installed))
	})

	t.Run("multiple entries returns exactly one (non-empty)", func(t *testing.T) {
		installed := map[string]string{
			"/tmp/a.container": "/etc/containers/systemd/a.container",
			"/tmp/b.container": "/etc/containers/systemd/b.container",
		}
		got := firstInstalledPath(installed)
		assert.NotEmpty(t, got)
		assert.Contains(t, installed, "/tmp/a.container")
	})
}

// --- Pure helpers used by Update ---

func TestBuildUpdateResponse(t *testing.T) {
	t.Run("maps all fields and forces state to unknown", func(t *testing.T) {
		got := buildUpdateResponse(
			"my-container.container",
			"my-container.service",
			"container",
			"/etc/containers/systemd/my-container.container",
			"[Container]\nImage=nginx:1.25\n",
		)

		assert.Equal(t, &QuadletGetResponse{
			ID:       "my-container.container",
			Name:     "my-container.container",
			Path:     "/etc/containers/systemd/my-container.container",
			Unit:     "my-container.service",
			Resource: "container",
			State:    "unknown",
			Content:  "[Container]\nImage=nginx:1.25\n",
		}, got)
	})

	t.Run("state is unknown even though a freshly updated unit may be running", func(t *testing.T) {
		// The decision under test: Update reports "unknown" right after reinstall
		// because systemd has not reloaded to report a fresh status. This pins
		// that decision regardless of the inputs.
		got := buildUpdateResponse("x.pod", "x.service", "pod", "/p/x.pod", "[Pod]\n")
		assert.Equal(t, "unknown", got.State)
	})

	t.Run("empty installed path is echoed as empty", func(t *testing.T) {
		got := buildUpdateResponse("x.container", "x.service", "container", "", "c")
		assert.Equal(t, "", got.Path)
	})
}

// Compile-time check that *QuadletCreateResponse has the fields the issue requires.
func TestQuadletCreateResponse_Shape(t *testing.T) {
	r := QuadletCreateResponse{
		ID:       "my-container",
		Name:     "my-container",
		Path:     "/etc/containers/systemd/my-container.container",
		Unit:     "",
		Resource: "container",
		State:    "unknown",
		Content:  "[Container]\nImage=nginx:latest\n",
	}
	assert.Equal(t, "my-container", r.ID)
	assert.Equal(t, "unknown", r.State)
}

// Compile-time sanity that ErrQuadletAlreadyExists is exposed for handler mapping.
func TestQuadletAlreadyExists_ErrorIsExposed(t *testing.T) {
	assert.Equal(t, "QUADLET_ALREADY_EXISTS", appErrors.ErrQuadletAlreadyExists.Code)
}

// --- Pure helpers used by Delete ---
//
// The delete path's domain logic (not-found detection from a list, the
// running-resource guard, and post-remove error mapping) is extracted into pure
// functions so it can be unit-tested directly — the engine boundary itself
// cannot be exercised under the mock connection (see the seam note at the top
// of the List tests).

func TestFindQuadletByName(t *testing.T) {
	list := []*entities.ListQuadlet{
		{Name: "alpha.container", Status: "running"},
		{Name: "bravo.pod", Status: "stopped"},
	}

	t.Run("found", func(t *testing.T) {
		got := findQuadletByName(list, "bravo.pod")
		assert.NotNil(t, got)
		assert.Equal(t, "bravo.pod", got.Name)
	})

	t.Run("not found returns nil", func(t *testing.T) {
		assert.Nil(t, findQuadletByName(list, "missing.container"))
	})

	t.Run("empty list returns nil", func(t *testing.T) {
		assert.Nil(t, findQuadletByName(nil, "alpha.container"))
	})

	t.Run("nil entries handled", func(t *testing.T) {
		list := []*entities.ListQuadlet{nil, {Name: "x.container"}}
		got := findQuadletByName(list, "x.container")
		assert.NotNil(t, got)
		assert.Equal(t, "x.container", got.Name)
	})
}

func TestCheckDeletePreconditions(t *testing.T) {
	t.Run("nil target is not found", func(t *testing.T) {
		err := checkDeletePreconditions(nil, false)
		assert.Equal(t, appErrors.ErrQuadletNotFound, err)
	})

	t.Run("stopped quadlet may be deleted without force", func(t *testing.T) {
		err := checkDeletePreconditions(&entities.ListQuadlet{Name: "x.container", Status: "stopped"}, false)
		assert.NoError(t, err)
	})

	t.Run("unknown state may be deleted without force", func(t *testing.T) {
		err := checkDeletePreconditions(&entities.ListQuadlet{Name: "x.container", Status: ""}, false)
		assert.NoError(t, err)
	})

	t.Run("running quadlet without force is rejected", func(t *testing.T) {
		err := checkDeletePreconditions(&entities.ListQuadlet{Name: "x.container", Status: "running (1s ago)"}, false)
		assert.Equal(t, appErrors.ErrQuadletResourceRunning, err)
	})

	t.Run("running quadlet with force is allowed", func(t *testing.T) {
		err := checkDeletePreconditions(&entities.ListQuadlet{Name: "x.container", Status: "running (1s ago)"}, true)
		assert.NoError(t, err)
	})

	t.Run("failed/error state may be deleted without force", func(t *testing.T) {
		err := checkDeletePreconditions(&entities.ListQuadlet{Name: "x.container", Status: "failed"}, false)
		assert.NoError(t, err)
	})
}

func TestIsResourceRunningError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"plain error", errors.New("something broke"), false},
		{"in use delegates to the adapter's conflict predicate", errors.New("resource is in use"), true},
		{"is running", errors.New("container is running"), true},
		{"still running", errors.New("unit still running"), true},
		{"active", errors.New("resource is active"), true},
		{"podman's real running refusal", errors.New("quadlet x.container is running and force is not set, refusing to remove: quadlet is running"), true},
		{"case-insensitive running", errors.New("RESOURCE IS RUNNING"), true},
		{"not found is not running", errors.New("no such quadlet"), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, isResourceRunningError(tt.err))
		})
	}
}

// --- Pure helpers used by Generate ---

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
