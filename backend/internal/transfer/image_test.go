package transfer

import (
	"bytes"
	"context"
	"errors"
	"io"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeImageSource streams the given archive bytes for any reference;
// exportErr simulates an export failure.
type fakeImageSource struct {
	archive   []byte
	exportErr error
}

func (f *fakeImageSource) Export(ref string, w io.Writer) error {
	if f.exportErr != nil {
		return f.exportErr
	}
	_, err := w.Write(f.archive)
	return err
}

// fakeImageTarget consumes the archive, recording the raw byte count and
// answering the load's names; loadErr simulates a load failure after
// reading everything.
type fakeImageTarget struct {
	rawLen  int64
	names   []string
	loadErr error
}

func (f *fakeImageTarget) Load(r io.Reader) ([]string, error) {
	raw, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}
	f.rawLen = int64(len(raw))
	if f.loadErr != nil {
		return nil, f.loadErr
	}
	return f.names, nil
}

func TestCopyImageData_CarriesArchiveCountsBytesAndReportsNames(t *testing.T) {
	archive := bytes.Repeat([]byte("x"), 3<<20)
	src := &fakeImageSource{archive: archive}
	tgt := &fakeImageTarget{names: []string{"registry.example/app:latest"}}

	var reports []int64
	streamed, names, err := CopyImageData(context.Background(), src, tgt, "sha256:abc", 0, func(count int64) {
		reports = append(reports, count)
	})

	require.NoError(t, err)
	assert.Equal(t, int64(len(archive)), streamed, "the count is the archive bytes actually streamed")
	assert.Equal(t, int64(len(archive)), tgt.rawLen, "the whole archive crossed the splice")
	assert.Equal(t, []string{"registry.example/app:latest"}, names, "the load's names ride back")
	require.NotEmpty(t, reports, "a 3 MiB stream reports progress")
	for i := 1; i < len(reports); i++ {
		assert.Greater(t, reports[i], reports[i-1], "reports are strictly increasing")
	}
}

func TestCopyImageData_BoundTrips(t *testing.T) {
	src := &fakeImageSource{archive: bytes.Repeat([]byte("x"), 3<<20)}
	tgt := &fakeImageTarget{}

	streamed, _, err := CopyImageData(context.Background(), src, tgt, "sha256:abc", 1<<20, nil)

	assert.ErrorIs(t, err, ErrBoundExceeded)
	assert.Greater(t, streamed, int64(1<<20), "the count reports how far the stream reached")
}

func TestCopyImageData_ZeroLimitNeverTrips(t *testing.T) {
	src := &fakeImageSource{archive: []byte("x")}
	tgt := &fakeImageTarget{}

	_, _, err := CopyImageData(context.Background(), src, tgt, "sha256:abc", 0, nil)

	require.NoError(t, err)
}

func TestCopyImageData_LoadFailureComesBackUnwrapped(t *testing.T) {
	cause := errors.New("load refused")
	src := &fakeImageSource{archive: []byte("x")}
	tgt := &fakeImageTarget{loadErr: cause}

	_, _, err := CopyImageData(context.Background(), src, tgt, "sha256:abc", 0, nil)

	assert.ErrorIs(t, err, cause)
}

func TestCopyImageData_ExportFailureComesBackUnwrapped(t *testing.T) {
	cause := errors.New("export refused")
	src := &fakeImageSource{exportErr: cause}
	tgt := &fakeImageTarget{}

	_, _, err := CopyImageData(context.Background(), src, tgt, "sha256:abc", 0, nil)

	assert.ErrorIs(t, err, cause)
}

// parkedImageSource streams one MiB, then parks until its context is
// done: the next write must fail on the pipe the AfterFunc closed — a
// mid-stream disconnect.
type parkedImageSource struct{ ctx context.Context }

func (p *parkedImageSource) Export(ref string, w io.Writer) error {
	chunk := bytes.Repeat([]byte("x"), 1<<20)
	if _, err := w.Write(chunk); err != nil {
		return err
	}
	<-p.ctx.Done()
	_, err := w.Write(chunk)
	return err
}

func TestCopyImageData_CancelledContextWins(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	src := &parkedImageSource{ctx: ctx}
	tgt := &fakeImageTarget{}

	firstReport := make(chan int64, 1)
	done := make(chan error, 1)
	go func() {
		_, _, err := CopyImageData(ctx, src, tgt, "sha256:abc", 0, func(count int64) {
			select {
			case firstReport <- count:
			default:
			}
		})
		done <- err
	}()

	<-firstReport // the first MiB crossed the splice; the source is parked
	cancel()

	select {
	case err := <-done:
		assert.ErrorIs(t, err, context.Canceled)
	case <-time.After(5 * time.Second):
		t.Fatal("the cancelled stream did not finish promptly")
	}
}

func TestImageBoundOrDefault(t *testing.T) {
	tests := []struct {
		name  string
		wired int64
		want  int64
	}{
		{"zero falls back to the kit default", 0, DefaultMaxImageTransferBytes},
		{"wired bound passes through", 1 << 20, 1 << 20},
		{"negative wired value passes through untouched", -1, -1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, ImageBoundOrDefault(tt.wired))
		})
	}
}
