package quadlets

import (
	"archive/tar"
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	appErrors "github.com/Homerus25/podland/backend/internal/errors"
	"github.com/Homerus25/podland/backend/internal/podmanerr"
	"go.podman.io/podman/v6/pkg/domain/entities"
)

// Volume data migration for the move/copy transfer (issue 214,
// architecture-decisions §18). The phases live in move.go's MoveWithProgress;
// this file owns the pieces the phases are built from: reference resolution,
// the migration plan, the target-emptiness and size preflight, and the
// machine-to-machine data phase itself (podman export on the source spliced
// into podman import on the target through an in-process pipe).

// errProbeNotEmpty terminates the emptiness probe's export once a content
// entry shows up: closing the pipe with it unblocks the export goroutine
// mid-stream — the probe is bounded and never reads a big volume to its end.
var errProbeNotEmpty = errors.New("volume probe: content entry found")

// errVolumeDataBoundExceeded trips the counting reader when the streamed
// volume crosses the migration bound; the data phase maps it to 413.
var errVolumeDataBoundExceeded = errors.New("volume data exceeds the migration bound")

// volumeBytesReportIncrement is the counting reader's progress cadence: a
// bytes event every 1 MiB actually streamed.
const volumeBytesReportIncrement int64 = 1 << 20

// volumeMigrationFailed wraps a data-phase engine failure as 500
// VOLUME_MIGRATION_FAILED, the message naming the volume and the cause
// (issue 214's error contract).
func volumeMigrationFailed(name string, err error) error {
	return appErrors.ErrVolumeMigrationFailed.WithMessage(
		fmt.Sprintf("Volume %s: %v", name, err))
}

// volumeMigrationPlanEntry is one planned volume data migration: the
// reference as written in the quadlet and the podman volume it resolves to
// on BOTH machines (the transfer carries no rename). QuadletFile names the
// companion .volume quadlet that defined the reference, "" for a bare name.
// SourceExists records the source volume's presence at plan time — the
// data phase re-inspects, but preflight uses it to leave references whose
// data will be skipped anyway out of the target probe.
type volumeMigrationPlanEntry struct {
	Ref          string
	PodmanVolume string
	QuadletFile  string
	SourceExists bool
}

// resolvePodmanVolumeName maps one volume-kind reference to the podman
// volume it names (issue 214, §18): a bare name is itself; a .volume
// reference resolves through its companion file's VolumeName= when the
// companion exists on this machine, else podman's systemd-<name> default.
// A dangling companion still resolves the default name — the preview labels
// the reference and the data phase skips it honestly instead of guessing.
func resolvePodmanVolumeName(source string, list []*entities.ListQuadlet, e QuadletEngine) (name, quadletFile string, err error) {
	quadletFile = volumeQuadletCandidate(source)
	if quadletFile == "" {
		return source, "", nil
	}

	name = "systemd-" + strings.TrimSuffix(quadletFile, ".volume")
	if findQuadletByName(list, quadletFile) == nil {
		return name, quadletFile, nil
	}
	printed, err := e.Print(quadletFile)
	if err != nil {
		// Listed but unprintable — vanished mid-lease; the default stands
		// (the same dangling treatment companionVolumeQuadlets applies).
		if podmanerr.IsNotFound(err) {
			return name, quadletFile, nil
		}
		return "", quadletFile, fmt.Errorf("failed to get volume quadlet %s: %w", quadletFile, err)
	}
	if override := volumeNameFromQuadlet(printed); override != "" {
		name = override
	}
	return name, quadletFile, nil
}

// deriveVolumeMigrationPlan resolves every volume-kind reference in the
// quadlet content to its podman volume (issue 214, §18). Host bind paths
// never enter the plan — there is no podman export path for them. The plan
// is file-ordered and deduplicated by resolved podman volume name (first
// reference wins for the block entry), so two references to one volume
// migrate its data once and report one outcome. It never returns nil.
func deriveVolumeMigrationPlan(content string, list []*entities.ListQuadlet, e QuadletEngine) ([]volumeMigrationPlanEntry, error) {
	plan := []volumeMigrationPlanEntry{}
	seen := map[string]bool{}
	for _, volume := range parseQuadletVolumes(content) {
		if !volume.isPodmanVolume() {
			continue
		}
		name, quadletFile, err := resolvePodmanVolumeName(volume.Source, list, e)
		if err != nil {
			return nil, err
		}
		if seen[name] {
			continue
		}
		seen[name] = true
		plan = append(plan, volumeMigrationPlanEntry{
			Ref:          volume.Source,
			PodmanVolume: name,
			QuadletFile:  quadletFile,
		})
	}
	return plan, nil
}

// preflightVolumeMigration refuses the data phase before anything is
// stopped or installed (issue 214, §18): per planned volume, an absent
// target volume is fine (the import creates it); an existing one must be
// empty unless the request merged in with overwriteVolumes. Entries whose
// SOURCE volume is absent are skipped entirely — the data phase will skip
// them too, and a dangling reference is reported, never an error. Emptiness
// is probed through the same export the data phase uses — a bounded tar
// read that stops at the first content entry — because podman reports no
// volume content size for the target either. Errors are AppErrors: 409
// VOLUME_MIGRATION_TARGET_NOT_EMPTY naming the volume, 500
// VOLUME_MIGRATION_FAILED for engine failures.
func (s *QuadletService) preflightVolumeMigration(ctx context.Context, userID, targetMachineID string, plan []volumeMigrationPlanEntry, overwrite bool) error {
	return s.engines.WithEngine(ctx, userID, targetMachineID, "quadlets.MoveVolumePreflight", func(e QuadletEngine) error {
		for _, entry := range plan {
			if !entry.SourceExists {
				continue
			}
			inspected, err := e.VolumeInspect(entry.PodmanVolume)
			if err != nil {
				return volumeMigrationFailed(entry.PodmanVolume, err)
			}
			if inspected == nil || overwrite {
				continue
			}
			empty, err := probeVolumeEmpty(e, entry.PodmanVolume)
			if err != nil {
				return volumeMigrationFailed(entry.PodmanVolume, err)
			}
			if !empty {
				return appErrors.ErrVolumeMigrationTargetNotEmpty.WithMessage(fmt.Sprintf(
					"Target volume %s already holds content; pass overwriteVolumes to merge", entry.PodmanVolume))
			}
		}
		return nil
	})
}

// probeVolumeEmpty answers whether the podman volume holds no content,
// decided by exporting it and reading the tar: directory entries (the
// always-framed volume root) and tar meta headers (x/g/L/K) are framing,
// the first real entry proves content and stops the read mid-stream —
// closing the pipe unblocks the export goroutine. A clean EOF after only
// framing means empty: the export returned nil, so the tar was complete.
func probeVolumeEmpty(e QuadletEngine, name string) (bool, error) {
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

// migrateVolumeData runs the data phase (issue 214, §18): one outcome per
// planned volume, in plan order. Two connection leases stand open at once —
// the export reads the source machine while the import writes the target —
// so the nested WithEngine is deliberate. Any volume's failure fails the
// whole phase: nothing has been installed yet, the caller returns the error
// and the transfer stays a no-op on both machines' quadlet sets.
func (s *QuadletService) migrateVolumeData(ctx context.Context, userID, sourceMachineID, targetMachineID string, plan []volumeMigrationPlanEntry, onProgress func(QuadletMoveEvent)) ([]QuadletMoveVolumeOutcome, error) {
	outcomes := []QuadletMoveVolumeOutcome{}
	err := s.engines.WithEngine(ctx, userID, sourceMachineID, "quadlets.MoveVolumeDataSource", func(src QuadletEngine) error {
		return s.engines.WithEngine(ctx, userID, targetMachineID, "quadlets.MoveVolumeDataTarget", func(tgt QuadletEngine) error {
			for _, entry := range plan {
				outcome, err := s.migrateOneVolume(ctx, src, tgt, sourceMachineID, entry, onProgress)
				if err != nil {
					return err
				}
				outcomes = append(outcomes, outcome)
			}
			return nil
		})
	})
	if err != nil {
		return nil, err
	}
	return outcomes, nil
}

// migrateOneVolume carries one volume's data from the source machine into
// the target (issue 214, §18): the target volume is created if absent, then
// podman's export streams through an in-process pipe into podman's import.
// The counting reader between them owns the migration bound — the always-on
// backstop for engines that report no inspect sizes — and reports bytes
// progress every volumeBytesReportIncrement. The source volume may have
// vanished since the plan was derived; a dangling reference then skips with
// a visible outcome instead of failing the transfer.
func (s *QuadletService) migrateOneVolume(ctx context.Context, src, tgt QuadletEngine, machineID string, entry volumeMigrationPlanEntry, onProgress func(QuadletMoveEvent)) (QuadletMoveVolumeOutcome, error) {
	emit := func(phase string, bytes int64, reason string) {
		if onProgress != nil {
			// OperationID stays empty: the streaming handler stamps its
			// own (the images Pull arrangement).
			onProgress(NewQuadletMoveVolumeEvent("", machineID, phase, entry.Ref, entry.PodmanVolume, bytes, reason))
		}
	}

	skipped := QuadletMoveVolumeOutcome{
		Ref:          entry.Ref,
		PodmanVolume: entry.PodmanVolume,
		Outcome:      MoveVolumeOutcomeSkipped,
	}

	sourceInspected, err := src.VolumeInspect(entry.PodmanVolume)
	if err != nil {
		return QuadletMoveVolumeOutcome{}, volumeMigrationFailed(entry.PodmanVolume, err)
	}
	if sourceInspected == nil {
		emit(MoveVolumePhaseSkip, 0, MovePreviewReasonDangling)
		skipped.Reason = MovePreviewReasonDangling
		return skipped, nil
	}

	emit(MoveVolumePhaseStart, 0, "")

	// Create-if-absent: an existing (probed empty or merged) volume is a
	// no-op, an absent one comes into being for the import.
	if err := tgt.VolumeCreate(entry.PodmanVolume); err != nil {
		return QuadletMoveVolumeOutcome{}, volumeMigrationFailed(entry.PodmanVolume, err)
	}

	pr, pw := io.Pipe()
	counted := &countingVolumeReader{
		r:          pr,
		limit:      s.volumeDataLimit(),
		nextReport: volumeBytesReportIncrement,
		onBytes: func(count int64) {
			emit(MoveVolumePhaseBytes, count, "")
		},
	}
	// A cancelled stream (client disconnect) must end the export promptly:
	// closing the pipe makes the goroutine's next write fail. stop()
	// releases the registration when this function returns.
	stop := context.AfterFunc(ctx, func() { pw.CloseWithError(context.Canceled) })
	defer stop()

	go func() {
		// CloseWithError(nil) is Close: the import sees a clean EOF
		// exactly when the export completed.
		pw.CloseWithError(src.VolumeExport(entry.PodmanVolume, pw))
	}()

	importErr := tgt.VolumeImport(entry.PodmanVolume, counted)
	if importErr == nil {
		emit(MoveVolumePhaseDone, counted.count, "")
		return QuadletMoveVolumeOutcome{
			Ref:          entry.Ref,
			PodmanVolume: entry.PodmanVolume,
			Outcome:      MoveVolumeOutcomeMigrated,
		}, nil
	}

	// The import stopped early — the export goroutine must not block on a
	// pipe nobody reads any more. Closing pw unblocks it (a no-op once the
	// goroutine already closed it on the success path).
	pw.Close()

	if counted.exceeded || errors.Is(importErr, errVolumeDataBoundExceeded) {
		return QuadletMoveVolumeOutcome{}, appErrors.ErrVolumeUploadTooLarge.WithMessage(fmt.Sprintf(
			"Volume %s exceeded the migration bound at %d bytes", entry.PodmanVolume, counted.count))
	}
	if ctx.Err() != nil {
		return QuadletMoveVolumeOutcome{}, ctx.Err()
	}
	return QuadletMoveVolumeOutcome{}, volumeMigrationFailed(entry.PodmanVolume, importErr)
}

// countingVolumeReader counts the bytes actually crossing the
// machine-to-machine splice (issue 214, §18). It owns the migration bound:
// crossing limit fails the read (and so the import) with
// errVolumeDataBoundExceeded — the backstop that holds even when the engine
// reports no inspect size for a pre-check. onBytes fires at most once per
// volumeBytesReportIncrement.
type countingVolumeReader struct {
	r          io.Reader
	limit      int64
	count      int64
	nextReport int64
	exceeded   bool
	onBytes    func(count int64)
}

func (c *countingVolumeReader) Read(p []byte) (int, error) {
	n, err := c.r.Read(p)
	if n > 0 {
		c.count += int64(n)
		if c.limit > 0 && c.count > c.limit {
			c.exceeded = true
			return n, errVolumeDataBoundExceeded
		}
		if c.count >= c.nextReport {
			c.nextReport = c.count + volumeBytesReportIncrement
			if c.onBytes != nil {
				c.onBytes(c.count)
			}
		}
	}
	return n, err
}
