package transfer

import (
	"context"
	"io"
)

// ImageDataSource is the source host's export width: podman's image
// export streaming the image's archive into w by image reference. The
// method names match the images domain's engine, so its engine satisfies
// this structurally — no adapter.
type ImageDataSource interface {
	Export(ref string, w io.Writer) error
}

// ImageDataTarget is the target host's load width: podman's image
// load consuming the archive from r and answering the names the load
// applied. The load is idempotent by construction — images are
// content-addressed, so loading onto a target that already holds the
// image skips present layers and re-applies tags.
type ImageDataTarget interface {
	Load(r io.Reader) ([]string, error)
}

// DefaultMaxImageTransferBytes is the image transfer bound a caller
// falls back to when it wires no explicit override: 10 GiB — images are
// routinely larger than the volume transfer's 2 GiB default, which
// would block common images on first use. The kit owns the bound ritual
// (§19), so it owns this default too: the config default for
// IMAGE_TRANSFER_MAX_BYTES is the same constant, so the 10 GiB policy
// lives in exactly one place. Callers resolve the fallback before
// CopyImageData: the counting reader treats a non-positive limit as
// unbounded.
const DefaultMaxImageTransferBytes int64 = 10 << 30

// ImageBoundOrDefault answers the wired image transfer bound, falling
// back to DefaultMaxImageTransferBytes when the wired value is zero
// (unset). The fallback is zero-only: a negative wired value passes
// through untouched, exactly as BoundOrDefault behaves for the volume
// bound. It is the one fallback the images domain's transfer limit
// resolves through (§19).
func ImageBoundOrDefault(wired int64) int64 {
	if wired == 0 {
		return DefaultMaxImageTransferBytes
	}
	return wired
}

// CopyImageData streams one image's archive from the source host
// into the target (§19, the kit's fourth composition): podman's export
// on the source spliced through the in-process pipe into podman's load
// on the target — the same pipe, counting reader, byte bound, and
// cancellation ritual the volume splice owns, shared through the
// splice core. The counting reader between them owns the transfer bound
// and reports the bytes actually streamed through onBytes at most once
// per BytesReportIncrement. A cancelled context (client disconnect)
// closes the pipe so the export goroutine ends promptly.
//
// It returns the bytes streamed and the names the target's load reports
// (the tags the load applied — the archive preserves them). Failures
// come back unwrapped for the caller to map, except: ErrBoundExceeded
// (wrapped or bare) when the bound tripped, and ctx.Err() when the
// stream was cancelled — a cancelled context wins over the load's own
// failure, so a disconnect reports cancellation, not a spurious
// engine error.
func CopyImageData(ctx context.Context, src ImageDataSource, tgt ImageDataTarget, ref string, limit int64, onBytes func(count int64)) (int64, []string, error) {
	var names []string
	count, err := splice(ctx,
		func(w io.Writer) error { return src.Export(ref, w) },
		func(r io.Reader) error {
			loaded, err := tgt.Load(r)
			if err == nil {
				names = loaded
			}
			return err
		},
		limit, onBytes)
	if err != nil {
		return count, nil, err
	}
	return count, names, nil
}
