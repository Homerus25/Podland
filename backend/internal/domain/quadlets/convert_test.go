package quadlets

import (
	"context"
	"errors"
	"os/exec"
	"testing"

	appErrors "github.com/Homerus25/podland/backend/internal/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeComposeConverter is the function-field fake for the ComposeConverter
// seam (spec #336): each test arms only what its scenario needs, and an
// unarmed call panics so an unexpected conversion fails the test loudly.
type fakeComposeConverter struct {
	convertFn func(ctx context.Context, composeContent string) ([]GeneratedQuadletFile, error)
}

func (f *fakeComposeConverter) Convert(ctx context.Context, composeContent string) ([]GeneratedQuadletFile, error) {
	if f.convertFn == nil {
		panic("unexpected ComposeConverter.Convert call")
	}
	return f.convertFn(ctx, composeContent)
}

// newConvertTestService builds the lean service over the fake engine with
// the given converter wired — the shape the ConvertCompose service tests
// drive. The conversion touches no machine, so the engine stays unarmed.
func newConvertTestService(converter ComposeConverter) *QuadletService {
	return NewQuadletService(fakeEngineProvider{engine: &fakeQuadletEngine{}}, nil, nil).WithComposeConverter(converter)
}

// TestServiceConvertCompose_MapsAndSortsEntries pins the file→entry
// mapping: name is the file name without extension, resource is derived
// from the extension through the domain's existing vocabulary, content is
// the file text verbatim, and the entries come back sorted by name. The
// fake hands the files over out of order so the sort is pinned, not
// inherited from the converter.
func TestServiceConvertCompose_MapsAndSortsEntries(t *testing.T) {
	t.Parallel()

	var gotCompose string
	converter := &fakeComposeConverter{
		convertFn: func(_ context.Context, composeContent string) ([]GeneratedQuadletFile, error) {
			gotCompose = composeContent
			return []GeneratedQuadletFile{
				{FileName: "web.container", Content: "[Container]\nImage=nginx\n"},
				{FileName: "dbdata.volume", Content: "[Volume]\nDriver=local\n"},
				{FileName: "frontend.network", Content: "[Network]\nDriver=bridge\n"},
			}, nil
		},
	}
	service := newConvertTestService(converter)

	response, err := service.ConvertCompose(context.Background(), QuadletConvertComposeRequest{Content: "services: {}"})

	require.NoError(t, err)
	require.NotNil(t, response)
	assert.Equal(t, "services: {}", gotCompose, "the compose content must reach the converter verbatim")
	require.Len(t, response.Quadlets, 3)
	assert.Equal(t, QuadletImportEntry{
		Name:     "dbdata",
		Resource: "volume",
		Content:  "[Volume]\nDriver=local\n",
	}, response.Quadlets[0])
	assert.Equal(t, QuadletImportEntry{
		Name:     "frontend",
		Resource: "network",
		Content:  "[Network]\nDriver=bridge\n",
	}, response.Quadlets[1])
	assert.Equal(t, QuadletImportEntry{
		Name:     "web",
		Resource: "container",
		Content:  "[Container]\nImage=nginx\n",
	}, response.Quadlets[2])
}

// TestServiceConvertCompose_ConverterFailure pins the error mapping: a
// converter rejection (*ConversionError) answers 400 VALIDATION_FAILED
// carrying the converter's stderr — ANSI color escapes stripped, the text
// otherwise verbatim.
func TestServiceConvertCompose_ConverterFailure(t *testing.T) {
	t.Parallel()

	converter := &fakeComposeConverter{
		convertFn: func(context.Context, string) ([]GeneratedQuadletFile, error) {
			return nil, &ConversionError{Stderr: "\x1b[91merror converting service `app`: `image` and `build` cannot both be set\x1b[0m\n"}
		},
	}
	service := newConvertTestService(converter)

	response, err := service.ConvertCompose(context.Background(), QuadletConvertComposeRequest{Content: "services: {}"})

	require.Nil(t, response)
	require.Error(t, err)
	appErr, ok := errors.AsType[*appErrors.AppError](err)
	require.True(t, ok, "a converter rejection must map to an AppError, got %T", err)
	assert.Equal(t, "VALIDATION_FAILED", appErr.Code)
	assert.Equal(t, 400, appErr.Status)
	assert.Equal(t, "error converting service `app`: `image` and `build` cannot both be set\n", appErr.Message,
		"the message must be the converter's stderr verbatim apart from ANSI color escapes")
}

// TestServiceConvertCompose_InternalErrorPropagates pins that a converter
// failure that is NOT a rejection (a transport or environment failure)
// propagates untouched — the envelope's 500 fallback owns it, it must not
// surface as a client-facing VALIDATION_FAILED.
func TestServiceConvertCompose_InternalErrorPropagates(t *testing.T) {
	t.Parallel()

	converter := &fakeComposeConverter{
		convertFn: func(context.Context, string) ([]GeneratedQuadletFile, error) {
			return nil, errors.New("failed to create temp dir")
		},
	}
	service := newConvertTestService(converter)

	response, err := service.ConvertCompose(context.Background(), QuadletConvertComposeRequest{Content: "services: {}"})

	require.Nil(t, response)
	require.Error(t, err)
	assert.NotErrorIs(t, err, appErrors.ErrValidationFailed, "an internal converter failure must not map to VALIDATION_FAILED")
	assert.Equal(t, "failed to create temp dir", err.Error())
}

// TestServiceConvertCompose_NilConverterFailsLoudly pins the wiring-bug
// contract: ConvertCompose on a service whose converter was never wired
// answers 500 INTERNAL_ERROR instead of panicking or silently succeeding.
func TestServiceConvertCompose_NilConverterFailsLoudly(t *testing.T) {
	t.Parallel()

	service := NewQuadletService(fakeEngineProvider{engine: &fakeQuadletEngine{}}, nil, nil)

	response, err := service.ConvertCompose(context.Background(), QuadletConvertComposeRequest{Content: "services: {}"})

	require.Nil(t, response)
	assert.ErrorIs(t, err, appErrors.ErrInternalError)
}

// TestStripANSIColors pins the wire-message decision (spec #336): ANSI
// SGR color escape sequences are terminal formatting, not message text,
// so they are stripped from the converter's stderr; everything else —
// including whitespace and newlines — survives verbatim.
func TestStripANSIColors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "color escapes stripped, text verbatim",
			input: "\x1b[91merror converting compose file\x1b[0m\n   2: \x1b[91merror converting service `app`\x1b[0m\n",
			want:  "error converting compose file\n   2: error converting service `app`\n",
		},
		{
			name:  "plain text untouched",
			input: "error converting compose file\n",
			want:  "error converting compose file\n",
		},
		{
			name:  "empty input",
			input: "",
			want:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, stripANSIColors(tt.input))
		})
	}
}

// TestClassifyRunError pins the run-error classification (pentest
// 2026-11-01, finding 7): only a process that started and exited non-zero
// (*exec.ExitError) is a conversion rejection — podlet's stderr reaches
// the wire as 400 VALIDATION_FAILED. Every other Run error — a missing
// binary (exec.ErrNotFound), a canceled context, any start failure — is a
// server-side problem wrapped as a non-ConversionError, so the envelope
// answers the generic 500 and the raw exec text stays in the server log.
// The errors are real, not synthesized: a failing sh -c for the ExitError,
// a nonexistent binary name for ErrNotFound, a pre-canceled context for
// the cancellation path.
func TestClassifyRunError(t *testing.T) {
	t.Parallel()

	t.Run("non-zero exit is a conversion rejection carrying stderr", func(t *testing.T) {
		t.Parallel()

		runErr := exec.Command("sh", "-c", "exit 3").Run()
		require.Error(t, runErr)

		got := classifyRunError(runErr, "error converting service `app`\n")

		convErr, ok := got.(*ConversionError)
		require.True(t, ok, "a non-zero exit must classify as *ConversionError, got %T", got)
		assert.Equal(t, "error converting service `app`\n", convErr.Stderr,
			"the converter's stderr must pass through verbatim")
	})

	t.Run("non-zero exit with empty stderr falls back to the exit error", func(t *testing.T) {
		t.Parallel()

		runErr := exec.Command("sh", "-c", "exit 3").Run()
		require.Error(t, runErr)

		got := classifyRunError(runErr, "   \n")

		convErr, ok := got.(*ConversionError)
		require.True(t, ok, "a non-zero exit must classify as *ConversionError, got %T", got)
		assert.Equal(t, runErr.Error(), convErr.Stderr,
			"an empty stderr must fall back to the exit error so the message is never blank")
	})

	t.Run("missing binary is a start failure, not a rejection", func(t *testing.T) {
		t.Parallel()

		runErr := exec.Command("podland-no-such-binary").Run()
		require.ErrorIs(t, runErr, exec.ErrNotFound)

		got := classifyRunError(runErr, "")

		assert.NotErrorIs(t, got, appErrors.ErrValidationFailed, "a missing binary must not map to VALIDATION_FAILED")
		_, isConv := got.(*ConversionError)
		assert.False(t, isConv, "a start failure must not classify as *ConversionError")
		assert.ErrorIs(t, got, runErr, "the classification must wrap the original error")
		assert.ErrorIs(t, got, exec.ErrNotFound, "the wrap must preserve the ErrNotFound chain")
		assert.Contains(t, got.Error(), "failed to start podlet",
			"the wrap must name the failure without echoing the raw exec text as the message")
	})

	t.Run("canceled context is a start failure, not a rejection", func(t *testing.T) {
		t.Parallel()

		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		runErr := exec.CommandContext(ctx, "sh", "-c", "true").Run()
		require.ErrorIs(t, runErr, context.Canceled)

		got := classifyRunError(runErr, "")

		_, isConv := got.(*ConversionError)
		assert.False(t, isConv, "a canceled context must not classify as *ConversionError")
		assert.ErrorIs(t, got, context.Canceled, "the wrap must preserve the cancellation chain")
	})
}
