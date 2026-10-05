package quadlets

import (
	"context"
	"errors"
	"fmt"

	appErrors "github.com/Homerus25/podland/backend/internal/errors"
	"github.com/Homerus25/podland/backend/internal/transfer"
)

// Volume data migration for the move/copy transfer (issue 214,
// architecture-decisions §18). The phases live in move.go's MoveWithProgress;
// this file owns the pieces the phases are built from: the one volume
// resolution derivation both the preview and the plan read (issue 230) —
// built on the shared transfer kit's quadlet reference seam (§19, issue
// 242) — the migration plan projected from that derivation, the
// target-emptiness and size preflight, and the machine-to-machine data
// phase itself. The data splice and the emptiness probe are the shared
// transfer kit's too (§19): this file maps their outcomes onto the quadlet
// move's wire vocabulary, the kit owns the pipe.

// volumeRef is one parsed `Volume=` reference (issue 230): the source as
// written (the part of the `Volume=` value before the first ":") and its
// kind. parseQuadletVolumes fills Source and Kind; deriveVolumeResolution
// enriches each volume-kind ref with the podman volume it resolves to and
// the companion file it consulted. Refs stay in file order without dedup —
// two references resolving to one volume stay two refs, the preview's
// per-reference rows (§18).
type volumeRef struct {
	Source string
	Kind   string
	// PodmanVolume is the podman volume the reference resolves to on the
	// source machine; "" until the derivation fills it, and always "" for
	// host bind paths.
	PodmanVolume string
	// QuadletFile is the companion .volume quadlet file the source names
	// ("" for bare names and host paths) — the candidate, present for
	// dangling references too.
	QuadletFile string
}

// isPodmanVolume reports whether the parsed reference names a podman
// volume rather than a host bind path — the one test the volume/host split
// turns on, shared by the resolution derivation and its projections
// (issue 214).
func (r volumeRef) isPodmanVolume() bool {
	return r.Kind == MovePreviewVolumeKindVolume
}

// parseQuadletVolumes maps the shared transfer kit's quadlet reference
// parse (§19, issue 242) onto the domain's volumeRef: the kit owns the
// `Volume=` line ritual — the same parse the volumes move preview reads —
// and this maps its kind labels onto the preview's wire vocabulary.
// Refs stay in file order without dedup — two references resolving to one
// volume stay two refs, the preview's per-reference rows (§18).
func parseQuadletVolumes(content string) []volumeRef {
	parsed := transfer.ParseQuadletVolumes(content)
	refs := make([]volumeRef, 0, len(parsed))
	for _, p := range parsed {
		kind := MovePreviewVolumeKindVolume
		if p.Kind == transfer.QuadletRefKindHostPath {
			kind = MovePreviewVolumeKindHostPath
		}
		refs = append(refs, volumeRef{Source: p.Source, Kind: kind})
	}
	return refs
}

// resolvedVolume is one resolved podman volume's facts (issue 230, §18):
// the name both machines address it by, the FIRST reference (file order)
// that resolved to it, and the source machine's inspect — nil means the
// volume does not exist there (a dangling reference).
type resolvedVolume struct {
	Name        string
	Ref         string
	QuadletFile string
	Inspect     *QuadletVolumeInspect
}

// volumeResolution is the one derivation of the volume facts both the
// move preview and the volume data migration plan read (issue 230, §18):
// per-reference facts in file order without dedup, plus per-resolved-volume
// facts deduplicated by name (first reference wins).
type volumeResolution struct {
	Refs    []volumeRef
	Volumes []resolvedVolume
}

// deriveVolumeResolution is the single entry point both projections — the
// preview's per-reference entries and the plan's per-volume entries —
// derive from (issue 230, §18). It enriches each volume-kind ref through
// the shared transfer kit's reference-resolution seam (§19, issue 242) —
// the same resolution the volumes move preview reads — consuming the
// companion contents the caller fetched once, never re-Printing, and
// inspects each resolved volume on the source machine once, deduplicated
// by resolved name with the first reference winning the volume fact.
// Host bind paths stay unresolved and out of the volume facts. The dedup
// key is the resolved name for the volume level and nothing at the ref
// level — the preview keeps per-reference rows while the plan dedups —
// which is exactly the divergence (preview deduped by reference string,
// plan by resolved name) that required one derivation. Neither slice is
// ever nil.
func deriveVolumeResolution(refs []volumeRef, companions []companionVolumeQuadlet, e QuadletEngine) (volumeResolution, error) {
	companionContents := make(map[string]string, len(companions))
	for _, companion := range companions {
		companionContents[companion.File] = companion.Content
	}

	resolution := volumeResolution{
		Refs:    make([]volumeRef, len(refs)),
		Volumes: []resolvedVolume{},
	}
	copy(resolution.Refs, refs)

	seen := map[string]bool{}
	for i := range resolution.Refs {
		ref := &resolution.Refs[i]
		if !ref.isPodmanVolume() {
			continue
		}
		name, quadletFile := transfer.ResolveQuadletVolumeName(ref.Source, companionContents)
		ref.PodmanVolume = name
		ref.QuadletFile = quadletFile
		if seen[name] {
			continue
		}
		seen[name] = true
		inspected, err := e.VolumeInspect(name)
		if err != nil {
			return volumeResolution{}, fmt.Errorf("failed to inspect source volume %s: %w", name, err)
		}
		resolution.Volumes = append(resolution.Volumes, resolvedVolume{
			Name:        name,
			Ref:         ref.Source,
			QuadletFile: quadletFile,
			Inspect:     inspected,
		})
	}
	return resolution, nil
}

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

// deriveVolumeMigrationPlan projects the volume resolution into the data
// phase's plan (issue 214, §18): one entry per RESOLVED volume — the
// derivation already deduplicated by name, the first reference winning —
// so two references to one volume migrate its data once and report one
// outcome. Host bind paths never entered the resolution. It never returns
// nil.
func deriveVolumeMigrationPlan(resolution volumeResolution) []volumeMigrationPlanEntry {
	plan := []volumeMigrationPlanEntry{}
	for _, volume := range resolution.Volumes {
		plan = append(plan, volumeMigrationPlanEntry{
			Ref:          volume.Ref,
			PodmanVolume: volume.Name,
			QuadletFile:  volume.QuadletFile,
			SourceExists: volume.Inspect != nil,
		})
	}
	return plan
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
			empty, err := transfer.ProbeVolumeEmpty(e, entry.PodmanVolume)
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
// the shared transfer kit's splice streams the data across (§19) — podman's
// export on the source into podman's import on the target, the kit's
// counting reader owning the migration bound and reporting bytes progress
// every 1 MiB actually streamed. The source volume may have vanished since
// the plan was derived; a dangling reference then skips with a visible
// outcome instead of failing the transfer.
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

	bytes, err := transfer.CopyVolumeData(ctx, src, tgt, entry.PodmanVolume, s.volumeDataLimit(), func(count int64) {
		emit(MoveVolumePhaseBytes, count, "")
	})
	if err == nil {
		emit(MoveVolumePhaseDone, bytes, "")
		return QuadletMoveVolumeOutcome{
			Ref:          entry.Ref,
			PodmanVolume: entry.PodmanVolume,
			Outcome:      MoveVolumeOutcomeMigrated,
		}, nil
	}

	if errors.Is(err, transfer.ErrBoundExceeded) {
		return QuadletMoveVolumeOutcome{}, appErrors.ErrVolumeUploadTooLarge.WithMessage(fmt.Sprintf(
			"Volume %s exceeded the migration bound at %d bytes", entry.PodmanVolume, bytes))
	}
	if ctx.Err() != nil {
		return QuadletMoveVolumeOutcome{}, ctx.Err()
	}
	return QuadletMoveVolumeOutcome{}, volumeMigrationFailed(entry.PodmanVolume, err)
}
