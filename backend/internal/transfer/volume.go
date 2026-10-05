package transfer

import (
	"archive/tar"
	"context"
	"errors"
	"fmt"
	"io"
)

// VolumeDataSource is the source machine's export width: podman's volume
// export streaming the volume's content as a tar archive into w. The
// method names match the quadlets domain's engine, so its engine
// satisfies this structurally; other domains adapt.
type VolumeDataSource interface {
	VolumeExport(name string, w io.Writer) error
}

// VolumeDataTarget is the target machine's import width: podman's volume
// import consuming a tar archive from r (merge semantics — podman-native,
// there is no replace mode).
type VolumeDataTarget interface {
	VolumeImport(name string, r io.Reader) error
}

// ErrBoundExceeded trips when the streamed volume data crosses the
// transfer bound; the caller maps it to its own 413 vocabulary. The
// returned byte count is the count at the trip.
var ErrBoundExceeded = errors.New("volume data exceeds the transfer bound")

// BytesReportIncrement is the counting reader's progress cadence: an
// onBytes report every 1 MiB actually streamed.
const BytesReportIncrement int64 = 1 << 20

// DefaultMaxVolumeUploadBytes is the transfer bound a caller falls
// back to when it wires no explicit override: 2 GiB, the same default
// the config value VOLUME_UPLOAD_MAX_BYTES carries. The kit owns the
// bound ritual (§19), so it owns the default too — the quadlets
// volume-data migration and the volumes copy/move resolve their
// zero-value fallbacks to this constant, and the config default is
// the same constant, so the 2 GiB policy lives in exactly one place.
// Callers resolve the fallback before CopyVolumeData: the counting
// reader treats a non-positive limit as unbounded.
const DefaultMaxVolumeUploadBytes int64 = 2 << 30

// errProbeNotEmpty terminates the emptiness probe's export once a content
// entry shows up: closing the pipe with it unblocks the export goroutine
// mid-stream — the probe is bounded and never reads a big volume to its end.
var errProbeNotEmpty = errors.New("volume probe: content entry found")

// CopyVolumeData streams one volume's content from the source machine
// into the target (§19): podman's export on the source spliced through
// an in-process pipe into podman's import on the target. The counting
// reader between them owns the transfer bound — the always-on backstop
// for engines that report no inspect sizes — and reports the bytes
// actually streamed through onBytes at most once per
// BytesReportIncrement. A cancelled context (client disconnect) closes
// the pipe so the export goroutine ends promptly.
//
// It returns the bytes streamed. Failures come back unwrapped for the
// caller to map, except: ErrBoundExceeded (wrapped or bare) when the
// bound tripped, and ctx.Err() when the stream was cancelled — a
// cancelled context wins over the import's own failure, so a disconnect
// reports cancellation, not a spurious engine error.
func CopyVolumeData(ctx context.Context, src VolumeDataSource, tgt VolumeDataTarget, name string, limit int64, onBytes func(count int64)) (int64, error) {
	pr, pw := io.Pipe()
	counted := &countingReader{
		r:          pr,
		limit:      limit,
		nextReport: BytesReportIncrement,
		onBytes:    onBytes,
	}
	// A cancelled stream (client disconnect) must end the export promptly:
	// closing the pipe makes the goroutine's next write fail. stop()
	// releases the registration when this function returns.
	stop := context.AfterFunc(ctx, func() { pw.CloseWithError(context.Canceled) })
	defer stop()

	go func() {
		// CloseWithError(nil) is Close: the import sees a clean EOF
		// exactly when the export completed.
		pw.CloseWithError(src.VolumeExport(name, pw))
	}()

	importErr := tgt.VolumeImport(name, counted)
	if importErr == nil {
		return counted.count, nil
	}

	// The import stopped early — the export goroutine must not block on a
	// pipe nobody reads any more. Closing pw unblocks it (a no-op once the
	// goroutine already closed it on the success path).
	pw.Close()

	if counted.exceeded || errors.Is(importErr, ErrBoundExceeded) {
		return counted.count, ErrBoundExceeded
	}
	if ctx.Err() != nil {
		return counted.count, ctx.Err()
	}
	return counted.count, importErr
}

// ProbeVolumeEmpty answers whether the podman volume holds no content,
// decided by exporting it and reading the tar: directory entries (the
// always-framed volume root) and tar meta headers (x/g/L/K) are framing,
// the first real entry proves content and stops the read mid-stream —
// closing the pipe unblocks the export goroutine. A clean EOF after only
// framing means empty: the export returned nil, so the tar was complete.
func ProbeVolumeEmpty(e VolumeDataSource, name string) (bool, error) {
	pr, pw := io.Pipe()
	defer pr.Close()

	type exportOutcome struct{ err error }
	done := make(chan exportOutcome, 1)
	go func() {
		err := e.VolumeExport(name, pw)
		// CloseWithError(nil) is Close: the reader sees a clean EOF
		// exactly when the export completed.
		pw.CloseWithError(err)
		done <- exportOutcome{err: err}
	}()

	tr := tar.NewReader(pr)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			outcome := <-done
			if outcome.err != nil {
				return false, fmt.Errorf("failed to export volume %s: %w", name, outcome.err)
			}
			return true, nil
		}
		if err != nil {
			// The pipe reader surfaces the export's own failure.
			<-done
			return false, fmt.Errorf("failed to export volume %s: %w", name, err)
		}
		switch hdr.Typeflag {
		case tar.TypeDir, tar.TypeXGlobalHeader, tar.TypeXHeader, tar.TypeGNULongName, tar.TypeGNULongLink:
			continue
		}
		pw.CloseWithError(errProbeNotEmpty)
		<-done
		return false, nil
	}
}

// countingReader counts the bytes actually crossing the machine-to-machine
// splice (§19). It owns the transfer bound: crossing limit fails the read
// (and so the import) with ErrBoundExceeded — the backstop that holds even
// when the engine reports no inspect size for a pre-check. onBytes fires at
// most once per BytesReportIncrement.
type countingReader struct {
	r          io.Reader
	limit      int64
	count      int64
	nextReport int64
	exceeded   bool
	onBytes    func(count int64)
}

func (c *countingReader) Read(p []byte) (int, error) {
	n, err := c.r.Read(p)
	if n > 0 {
		c.count += int64(n)
		if c.limit > 0 && c.count > c.limit {
			c.exceeded = true
			return n, ErrBoundExceeded
		}
		if c.count >= c.nextReport {
			c.nextReport = c.count + BytesReportIncrement
			if c.onBytes != nil {
				c.onBytes(c.count)
			}
		}
	}
	return n, err
}
