package quadlets

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.podman.io/podman/v6/pkg/domain/entities"

	"github.com/Homerus25/podland/backend/internal/sshexec"
	"github.com/Homerus25/podland/backend/internal/testutils"
)

func newAutoUpdateService(mockSSH *testutils.MockSSHExec) *QuadletService {
	// AutoUpdate's run crosses the SSH-exec seam; the engine provider is
	// touched only by the plan build, which reads an empty list here.
	engine := &fakeQuadletEngine{
		listFn: func(entities.QuadletListOptions) ([]*entities.ListQuadlet, error) {
			return nil, nil
		},
	}
	return NewQuadletService(fakeEngineProvider{engine: engine}, mockSSH, lookupForUser("core"))
}

func TestQuadletService_AutoUpdate_CommandAssembly(t *testing.T) {
	tests := []struct {
		name     string
		dryRun   bool
		filter   string
		wantArgs []string
	}{
		{name: "without dry-run", dryRun: false, wantArgs: []string{"auto-update", "--format", "json"}},
		{name: "with dry-run", dryRun: true, wantArgs: []string{"auto-update", "--format", "json", "--dry-run"}},
		{name: "with filter", filter: "web.service", wantArgs: []string{"auto-update", "--format", "json", "web.service"}},
		{name: "with dry-run and filter", dryRun: true, filter: "web.service", wantArgs: []string{"auto-update", "--format", "json", "--dry-run", "web.service"}},
		{name: "empty filter adds no positional arg", filter: "", wantArgs: []string{"auto-update", "--format", "json"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockSSH := testutils.NewMockSSHExecWithStreamResult(sshexec.ExecResult{Stdout: "[]", ExitCode: 0})
			svc := newAutoUpdateService(mockSSH)

			_, err := svc.AutoUpdate(context.Background(), "user-1", "machine-1", tt.dryRun, tt.filter, func(AutoUpdateEvent) {})

			require.NoError(t, err)
			require.Len(t, mockSSH.StreamCalls(), 1)
			call := mockSSH.StreamCalls()[0]
			assert.Equal(t, "machine-1", call.MachineID)
			assert.Equal(t, "user-1", call.UserID)
			assert.Equal(t, "podman", call.Command)
			assert.Equal(t, tt.wantArgs, call.Opts.Args)
		})
	}
}

func TestQuadletService_AutoUpdate_LinesForwardedInArrivalOrder(t *testing.T) {
	mockSSH := testutils.NewMockSSHExecWithStreamFunc(func(ctx context.Context, machineID, userID, command string, opts sshexec.ExecOptions, onLine func(sshexec.OutputLine)) (sshexec.ExecResult, error) {
		onLine(sshexec.OutputLine{Stream: sshexec.StdoutStream, Text: "line-1"})
		onLine(sshexec.OutputLine{Stream: sshexec.StderrStream, Text: "err-1"})
		onLine(sshexec.OutputLine{Stream: sshexec.StdoutStream, Text: "line-2"})
		return sshexec.ExecResult{Stdout: "[]", ExitCode: 0}, nil
	})
	svc := newAutoUpdateService(mockSSH)

	var got []AutoUpdateEvent
	_, err := svc.AutoUpdate(context.Background(), "user-1", "machine-1", false, "", func(event AutoUpdateEvent) {
		got = append(got, event)
	})

	require.NoError(t, err)
	require.Len(t, got, 4, "the empty plan event plus three log events")
	assert.Equal(t, AutoUpdateEventTypePlan, got[0].Type)
	assert.Empty(t, got[0].Plan, "an empty machine answers an empty plan")
	wantLogs := []struct{ stream, line string }{
		{"stdout", "line-1"},
		{"stderr", "err-1"},
		{"stdout", "line-2"},
	}
	for i, want := range wantLogs {
		assert.Equal(t, AutoUpdateEventTypeLog, got[1+i].Type)
		assert.Equal(t, want.stream, got[1+i].Stream)
		assert.Equal(t, want.line, got[1+i].Line)
	}
}

func TestQuadletService_AutoUpdate_ParsesReportExitZero(t *testing.T) {
	stdout := "[{\"Unit\":\"web.service\",\"ContainerName\":\"web\",\"ImageName\":\"nginx:latest\",\"Updated\":\"true\"}," +
		"{\"Unit\":\"api.service\",\"ContainerName\":\"api\",\"ImageName\":\"api:1.0\",\"Updated\":\"false\"}]"
	mockSSH := testutils.NewMockSSHExecWithStreamResult(sshexec.ExecResult{Stdout: stdout, ExitCode: 0})
	svc := newAutoUpdateService(mockSSH)

	report, err := svc.AutoUpdate(context.Background(), "user-1", "machine-1", false, "", func(AutoUpdateEvent) {})

	require.NoError(t, err)
	assert.Equal(t, AutoUpdateCompleted, report.Overall)
	assert.Equal(t, 0, report.ExitCode)
	require.Len(t, report.Containers, 2)
	assert.Equal(t, AutoUpdateItem{
		ContainerName: "web", ImageName: "nginx:latest", Updated: AutoUpdateItemUpdated, Unit: "web.service",
	}, report.Containers[0])
	assert.Equal(t, AutoUpdateItem{
		ContainerName: "api", ImageName: "api:1.0", Updated: AutoUpdateItemUpToDate, Unit: "api.service",
	}, report.Containers[1])
}

func TestQuadletService_AutoUpdate_NonZeroExitWithFailuresIsData(t *testing.T) {
	stdout := "[{\"Unit\":\"web.service\",\"ContainerName\":\"web\",\"ImageName\":\"nginx:latest\",\"Updated\":\"true\"}," +
		"{\"Unit\":\"db.service\",\"ContainerName\":\"db\",\"ImageName\":\"postgres:16\",\"Updated\":\"false\",\"Error\":\"database is locked\"}]"
	mockSSH := testutils.NewMockSSHExecWithStreamResult(sshexec.ExecResult{Stdout: stdout, ExitCode: 3})
	svc := newAutoUpdateService(mockSSH)

	report, err := svc.AutoUpdate(context.Background(), "user-1", "machine-1", false, "", func(AutoUpdateEvent) {})

	require.NoError(t, err)
	assert.Equal(t, AutoUpdateWithFailures, report.Overall)
	assert.Equal(t, 3, report.ExitCode)
	require.Len(t, report.Containers, 2)
	assert.Equal(t, AutoUpdateItemUpdated, report.Containers[0].Updated)
	assert.Equal(t, AutoUpdateItemFailed, report.Containers[1].Updated)
	assert.Equal(t, "database is locked", report.Containers[1].Error)
}

func TestQuadletService_AutoUpdate_DryRunReportsPendingAndDryRunOverall(t *testing.T) {
	stdout := "[{\"Unit\":\"web.service\",\"ContainerName\":\"web\",\"ImageName\":\"nginx:latest\",\"Updated\":\"pending\"}]"
	mockSSH := testutils.NewMockSSHExecWithStreamResult(sshexec.ExecResult{Stdout: stdout, ExitCode: 0})
	svc := newAutoUpdateService(mockSSH)

	report, err := svc.AutoUpdate(context.Background(), "user-1", "machine-1", true, "", func(AutoUpdateEvent) {})

	require.NoError(t, err)
	assert.Equal(t, AutoUpdateDryRun, report.Overall)
	require.Len(t, report.Containers, 1)
	assert.Equal(t, AutoUpdateItemPending, report.Containers[0].Updated)
}

func TestQuadletService_AutoUpdate_UnparsableOutputCompletesWithoutError(t *testing.T) {
	mockSSH := testutils.NewMockSSHExecWithStreamResult(sshexec.ExecResult{Stdout: "not json at all", ExitCode: 0})
	svc := newAutoUpdateService(mockSSH)

	report, err := svc.AutoUpdate(context.Background(), "user-1", "machine-1", false, "", func(AutoUpdateEvent) {})

	require.NoError(t, err)
	assert.Equal(t, AutoUpdateUnparsable, report.Overall)
	assert.Nil(t, report.Containers)
}

func TestQuadletService_AutoUpdate_SSHErrorPropagates(t *testing.T) {
	mockSSH := testutils.NewMockSSHExecWithStreamError(errors.New("dial failed"))
	svc := newAutoUpdateService(mockSSH)

	report, err := svc.AutoUpdate(context.Background(), "user-1", "machine-1", false, "", func(AutoUpdateEvent) {})

	require.Error(t, err)
	assert.Equal(t, AutoUpdateReport{}, report)
}

// --- The plan and pulling vocabulary (issue 448): pure-function seam ---
//
// The plan event's content and the pulling event's unit resolution are
// pure functions, pinned here alongside the report parser's tests: the
// AutoUpdate=/Image= parse of one .container file, the unit-only
// degradation, the filter narrowing, and the exact/suffix/unmatched
// image matching.

// TestParseAutoUpdateQuadlet pins the AutoUpdate=/Image= parse: a unit
// participates when its .container file carries AutoUpdate=registry, and
// its image is the file's Image= value — empty when the line is missing
// or carries no value (the unit-only degradation).
func TestParseAutoUpdateQuadlet(t *testing.T) {
	tests := []struct {
		name         string
		content      string
		participates bool
		image        string
	}{
		{
			name:         "registry policy with image",
			content:      "[Container]\nImage=docker.io/library/nginx:latest\nAutoUpdate=registry\n",
			participates: true,
			image:        "docker.io/library/nginx:latest",
		},
		{
			name:         "registry policy without image degrades to unit-only",
			content:      "[Container]\nAutoUpdate=registry\n",
			participates: true,
			image:        "",
		},
		{
			name:         "empty image value is unparsable",
			content:      "[Container]\nImage=\nAutoUpdate=registry\n",
			participates: true,
			image:        "",
		},
		{
			name:         "other policy does not participate",
			content:      "[Container]\nImage=nginx:latest\nAutoUpdate=local\n",
			participates: false,
		},
		{
			name:         "no policy does not participate",
			content:      "[Container]\nImage=nginx:latest\n",
			participates: false,
		},
		{
			name:         "whitespace around keys and values is tolerated",
			content:      "[Container]\n  Image = nginx:latest  \nAutoUpdate=registry\n",
			participates: true,
			image:        "nginx:latest",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			participates, image := parseAutoUpdateQuadlet(tt.content)
			assert.Equal(t, tt.participates, participates)
			assert.Equal(t, tt.image, image)
		})
	}
}

// TestBuildAutoUpdatePlan pins the plan build: participating units in file
// order, the unit-only degradation for unreadable files and missing
// images, and the filter narrowing to one unit on a filtered run.
func TestBuildAutoUpdatePlan(t *testing.T) {
	sources := []autoUpdatePlanSource{
		{Unit: "web.service", Readable: true, Content: "[Container]\nImage=nginx:latest\nAutoUpdate=registry\n"},
		{Unit: "db.service", Readable: true, Content: "[Container]\nImage=postgres:16\n"},
		{Unit: "cache.service", Readable: true, Content: "[Container]\nAutoUpdate=registry\n"},
		{Unit: "broken.service", Readable: false},
	}

	t.Run("unfiltered run lists every participating unit in file order", func(t *testing.T) {
		plan := buildAutoUpdatePlan(sources, "")
		assert.Equal(t, []AutoUpdatePlanItem{
			{Unit: "web.service", Image: "nginx:latest"},
			{Unit: "cache.service"},
			{Unit: "broken.service"},
		}, plan)
	})

	t.Run("filtered run narrows to the filtered unit", func(t *testing.T) {
		plan := buildAutoUpdatePlan(sources, "cache.service")
		assert.Equal(t, []AutoUpdatePlanItem{{Unit: "cache.service"}}, plan)
	})

	t.Run("filter matching no planned unit answers an empty plan", func(t *testing.T) {
		plan := buildAutoUpdatePlan(sources, "nope.service")
		assert.Empty(t, plan)
		assert.NotNil(t, plan, "the plan event must carry an empty list, not a missing field")
	})
}

// TestPullImageFromLine pins the "Trying to pull <image>" line parse:
// podman prints the fully-qualified image followed by an ellipsis.
func TestPullImageFromLine(t *testing.T) {
	tests := []struct {
		name     string
		line     string
		wantOK   bool
		wantImag string
	}{
		{name: "podman's pull line with ellipsis", line: "Trying to pull docker.io/library/nginx:latest...", wantOK: true, wantImag: "docker.io/library/nginx:latest"},
		{name: "pull line without ellipsis still parses", line: "Trying to pull quay.io/some/image:1", wantOK: true, wantImag: "quay.io/some/image:1"},
		{name: "blob progress line is not a pull line", line: "Copying blob sha256:abcdef"},
		{name: "empty image is not a pull line", line: "Trying to pull ..."},
		{name: "unrelated line", line: "Updating image"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			image, ok := pullImageFromLine(tt.line)
			assert.Equal(t, tt.wantOK, ok)
			assert.Equal(t, tt.wantImag, image)
		})
	}
}

// TestMatchPullingUnit pins the image matching: exact match first, then
// suffix (podman prints normalized fully-qualified names; quadlet files
// may carry short names), a suffix counting only across a registry/repo
// boundary, an unmatched image matching no unit, and a unit-only plan
// entry never matching mid-run.
func TestMatchPullingUnit(t *testing.T) {
	plan := []AutoUpdatePlanItem{
		{Unit: "web.service", Image: "nginx:latest"},
		{Unit: "api.service", Image: "docker.io/library/api:2"},
		{Unit: "cache.service"},
	}

	t.Run("exact match wins", func(t *testing.T) {
		item, ok := matchPullingUnit(plan, "docker.io/library/api:2")
		assert.True(t, ok)
		assert.Equal(t, "api.service", item.Unit)
	})

	t.Run("suffix match resolves a short planned name", func(t *testing.T) {
		item, ok := matchPullingUnit(plan, "docker.io/library/nginx:latest")
		assert.True(t, ok)
		assert.Equal(t, "web.service", item.Unit)
	})

	t.Run("suffix match counts only across a registry/repo boundary", func(t *testing.T) {
		// "app" is a suffix of "docker.io/library/app:latest" only
		// because the preceding "/" is the registry/repo boundary; the
		// same short name inside another repo name ("myapp") is not a
		// match.
		plan := []AutoUpdatePlanItem{{Unit: "app.service", Image: "app:latest"}}
		item, ok := matchPullingUnit(plan, "docker.io/library/app:latest")
		assert.True(t, ok)
		assert.Equal(t, "app.service", item.Unit)
	})

	t.Run("boundary-blind suffix is rejected", func(t *testing.T) {
		// "app:latest" is a raw suffix of "myapp:latest", but the
		// preceding character is "p", not "/": a different repo, not the
		// planned image.
		plan := []AutoUpdatePlanItem{{Unit: "app.service", Image: "app:latest"}}
		_, ok := matchPullingUnit(plan, "myapp:latest")
		assert.False(t, ok)
	})

	t.Run("boundary rule resolves an ambiguous suffix to the right unit", func(t *testing.T) {
		// Both planned images are suffixes of the pulled one under raw
		// matching; only "myapp:latest" sits behind a "/" boundary, so
		// the pull must flip the myapp row, not the app row.
		plan := []AutoUpdatePlanItem{
			{Unit: "app.service", Image: "app:latest"},
			{Unit: "myapp.service", Image: "myapp:latest"},
		}
		item, ok := matchPullingUnit(plan, "docker.io/library/myapp:latest")
		assert.True(t, ok)
		assert.Equal(t, "myapp.service", item.Unit)
	})

	t.Run("exact match beats a suffix match", func(t *testing.T) {
		// web.service's short name is also a suffix of api.service's
		// fully-qualified image only if the names overlap; pin the
		// priority with a plan where both rules answer different units.
		plan := []AutoUpdatePlanItem{
			{Unit: "short.service", Image: "nginx:latest"},
			{Unit: "full.service", Image: "docker.io/library/nginx:latest"},
		}
		item, ok := matchPullingUnit(plan, "docker.io/library/nginx:latest")
		assert.True(t, ok)
		assert.Equal(t, "full.service", item.Unit)
	})

	t.Run("unmatched image answers no unit", func(t *testing.T) {
		_, ok := matchPullingUnit(plan, "quay.io/unrelated:1")
		assert.False(t, ok)
	})

	t.Run("unit-only entry never matches mid-run", func(t *testing.T) {
		_, ok := matchPullingUnit(plan, "cache.service")
		assert.False(t, ok)
	})
}

// --- The plan and pulling events on the service seam (issue 448) ---

// autoUpdatePlanEngine builds the function-field fake for the plan build:
// a machine with two .container quadlets (one registry-policy unit with a
// short image name, one unreadable file), a policy-less .container, a
// .volume file, and one .container whose systemd name must be derived
// because podman reports no UnitName.
func autoUpdatePlanEngine() *fakeQuadletEngine {
	return &fakeQuadletEngine{
		listFn: func(entities.QuadletListOptions) ([]*entities.ListQuadlet, error) {
			return []*entities.ListQuadlet{
				{Name: "web.container", UnitName: "web.service", Path: "/etc/containers/systemd/web.container"},
				{Name: "broken.container", UnitName: "broken.service", Path: "/etc/containers/systemd/broken.container"},
				{Name: "idle.container", UnitName: "idle.service", Path: "/etc/containers/systemd/idle.container"},
				{Name: "cache.container", Path: "/etc/containers/systemd/cache.container"},
				{Name: "data.volume", UnitName: "data.service", Path: "/etc/containers/systemd/data.volume"},
			}, nil
		},
		printFn: func(name string) (string, error) {
			switch name {
			case "web.container":
				return "[Container]\nImage=nginx:latest\nAutoUpdate=registry\n", nil
			case "broken.container":
				return "", errors.New("unreadable file")
			case "idle.container":
				return "[Container]\nImage=alpine:latest\n", nil
			case "cache.container":
				return "[Container]\nImage=redis:7\nAutoUpdate=registry\n", nil
			}
			panic("unexpected quadlet print: " + name)
		},
	}
}

// TestQuadletService_AutoUpdate_EmitsPlanPullingAndLogEvents pins the
// service seam's event sequence: the plan event opens the run (every
// participating unit, the unreadable file degraded to a unit-only entry,
// the policy-less and non-container files excluded, the derived unit
// name), and each "Trying to pull" line that matches a planned image
// emits a pulling event ahead of its own log event — an unmatched image
// emits nothing but the log event.
func TestQuadletService_AutoUpdate_EmitsPlanPullingAndLogEvents(t *testing.T) {
	mockSSH := testutils.NewMockSSHExecWithStreamFunc(func(_ context.Context, _, _, _ string, _ sshexec.ExecOptions, onLine func(sshexec.OutputLine)) (sshexec.ExecResult, error) {
		onLine(sshexec.OutputLine{Stream: sshexec.StdoutStream, Text: "Trying to pull docker.io/library/nginx:latest..."})
		onLine(sshexec.OutputLine{Stream: sshexec.StdoutStream, Text: "Trying to pull quay.io/unrelated:1..."})
		onLine(sshexec.OutputLine{Stream: sshexec.StdoutStream, Text: "Copying blob sha256:abcdef"})
		return sshexec.ExecResult{ExitCode: 0, Stdout: "[]"}, nil
	})
	svc := NewQuadletService(fakeEngineProvider{engine: autoUpdatePlanEngine()}, mockSSH, lookupForUser("core"))

	var got []AutoUpdateEvent
	_, err := svc.AutoUpdate(context.Background(), "user-1", "machine-1", false, "", func(event AutoUpdateEvent) {
		got = append(got, event)
	})

	require.NoError(t, err)
	require.Len(t, got, 5, "the plan event, the pulling event, and three log events")

	assert.Equal(t, AutoUpdateEventTypePlan, got[0].Type, "the plan event opens the run")
	assert.Equal(t, []AutoUpdatePlanItem{
		{Unit: "web.service", Image: "nginx:latest"},
		{Unit: "broken.service"},
		{Unit: "cache.service", Image: "redis:7"},
	}, got[0].Plan, "participating units in file order; unreadable file unit-only; policy-less and non-container files excluded; derived unit name")

	assert.Equal(t, AutoUpdateEventTypePulling, got[1].Type, "a matching pull line emits a pulling event ahead of its log event")
	assert.Equal(t, "web.service", got[1].Unit, "the short planned name resolves by suffix against podman's fully-qualified image")
	assert.Equal(t, "docker.io/library/nginx:latest", got[1].Image)

	wantLogs := []struct{ stream, line string }{
		{"stdout", "Trying to pull docker.io/library/nginx:latest..."},
		{"stdout", "Trying to pull quay.io/unrelated:1..."},
		{"stdout", "Copying blob sha256:abcdef"},
	}
	for i, want := range wantLogs {
		assert.Equal(t, AutoUpdateEventTypeLog, got[2+i].Type)
		assert.Equal(t, want.stream, got[2+i].Stream)
		assert.Equal(t, want.line, got[2+i].Line,
			"an unmatched image emits no pulling event; every line still arrives as a log event")
	}
}

// TestQuadletService_AutoUpdate_FilteredRunNarrowsPlan pins the filter's
// effect on the plan: a filtered run's plan event lists only the filtered
// unit.
func TestQuadletService_AutoUpdate_FilteredRunNarrowsPlan(t *testing.T) {
	mockSSH := testutils.NewMockSSHExecWithStreamFunc(func(_ context.Context, _, _, _ string, _ sshexec.ExecOptions, _ func(sshexec.OutputLine)) (sshexec.ExecResult, error) {
		return sshexec.ExecResult{ExitCode: 0, Stdout: "[]"}, nil
	})
	svc := NewQuadletService(fakeEngineProvider{engine: autoUpdatePlanEngine()}, mockSSH, lookupForUser("core"))

	var got []AutoUpdateEvent
	_, err := svc.AutoUpdate(context.Background(), "user-1", "machine-1", false, "cache.service", func(event AutoUpdateEvent) {
		got = append(got, event)
	})

	require.NoError(t, err)
	require.Len(t, got, 1, "the narrowed plan event plus no output lines")
	assert.Equal(t, AutoUpdateEventTypePlan, got[0].Type)
	assert.Equal(t, []AutoUpdatePlanItem{{Unit: "cache.service", Image: "redis:7"}}, got[0].Plan)
}

// TestQuadletService_AutoUpdate_PlanBuildFailureDegradesToEmptyPlan pins
// the run-level degradation: when the quadlet list cannot be read at all,
// the run still proceeds with an empty plan rather than failing — the
// report at the end still tells the truth.
func TestQuadletService_AutoUpdate_PlanBuildFailureDegradesToEmptyPlan(t *testing.T) {
	engine := &fakeQuadletEngine{
		listFn: func(entities.QuadletListOptions) ([]*entities.ListQuadlet, error) {
			return nil, errors.New("connection failed")
		},
	}
	mockSSH := testutils.NewMockSSHExecWithStreamFunc(func(_ context.Context, _, _, _ string, _ sshexec.ExecOptions, _ func(sshexec.OutputLine)) (sshexec.ExecResult, error) {
		return sshexec.ExecResult{ExitCode: 0, Stdout: "[]"}, nil
	})
	svc := NewQuadletService(fakeEngineProvider{engine: engine}, mockSSH, lookupForUser("core"))

	var got []AutoUpdateEvent
	report, err := svc.AutoUpdate(context.Background(), "user-1", "machine-1", false, "", func(event AutoUpdateEvent) {
		got = append(got, event)
	})

	require.NoError(t, err, "a plan-build failure must not fail the run")
	assert.Equal(t, AutoUpdateCompleted, report.Overall)
	require.Len(t, got, 1)
	assert.Equal(t, AutoUpdateEventTypePlan, got[0].Type)
	assert.NotNil(t, got[0].Plan, "the plan event still carries a list")
	assert.Empty(t, got[0].Plan)
}
