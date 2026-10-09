package volumes

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	appErrors "github.com/Homerus25/podland/backend/internal/errors"
	"github.com/Homerus25/podland/backend/internal/models"
	"github.com/Homerus25/podland/backend/internal/permission"
	"github.com/Homerus25/podland/backend/internal/podmanerr"
	"github.com/Homerus25/podland/backend/internal/transfer"
	"go.podman.io/podman/v6/pkg/domain/entities"
)

// Cross-machine volume copy/move (issue 238, architecture-decisions §19):
// the shared transfer kit owns the export→import splice; this file owns
// the volumes domain's composition around it — the preview's facts
// (collision, in-use, referencing quadlets), the transfer's create →
// splice → remove order, and the mapping onto the wire vocabulary. The
// name is preserved on both machines; there is no rename and no
// overwrite.

// MovePreview answers everything the copy/move dialog must show before
// confirming (issue 238): whether the target machine already holds a
// same-named volume, whether a container on the source machine uses the
// volume, and which source quadlets reference it. See
// doku/description_volumes_api.md.
//
// Access: the route's subtree middleware already proved the caller holds
// an access row on the source machine — read level suffices, this
// endpoint mutates nothing. The target machine is a service-side gate
// (route middleware only sees the route's machine): the caller's stored
// level must be at least read, checked against the access store BEFORE
// any podman call, so a caller without target access gets the standard
// 403 ACCESS_DENIED and never a false preview.
func (s *VolumeService) MovePreview(ctx context.Context, userID, machineID, volumeName, targetMachineID string) (*VolumeMovePreviewResponse, error) {
	// The target gate fires before the connection lease (the quadlet
	// move-preview's shape): a lookup failure is 500, a level below read
	// — an absent row included — is 403, per permission.RequireLevel's
	// single-owner contract.
	if err := permission.RequireLevel(s.accessRepo, targetMachineID, userID, models.PermissionRead); err != nil {
		return nil, err
	}

	// Source lease: the volume must exist (404) before any scan.
	exists, err := s.volumeExists(ctx, userID, machineID, volumeName)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, appErrors.ErrVolumeNotFound
	}

	// Usage lease: the in-use scan — container mounts and referencing
	// quadlets, both read through the usage seam on the source machine.
	var inUse bool
	var referencing []VolumeMoveQuadletRef
	err = s.usageEngines.WithEngine(ctx, userID, machineID, "volumes.MovePreviewSource", func(e VolumeUsageEngine) error {
		inUse, err = volumeInUse(e, volumeName)
		if err != nil {
			return err
		}
		// The referencing-quadlets warning rides the shared transfer
		// kit's machine-wide quadlet scan (§19, issue 259): the usage
		// engine satisfies the kit's QuadletReader structurally, so it
		// is handed over directly — no adapter. The kit's error rides
		// untouched: the envelope's 500 fallback answers the generic
		// INTERNAL_ERROR, the wrap message is internal.
		names, err := transfer.QuadletsReferencingVolume(e, volumeName)
		if err != nil {
			return err
		}
		referencing = toVolumeMoveQuadletRefs(names)
		return nil
	})
	if err != nil {
		return nil, err
	}

	// Target lease: the collision check reads the target's volume by
	// exact name. The transfer re-checks at execution time; this flag
	// only drives the dialog's confirm gate.
	var collision bool
	err = s.engines.WithEngine(ctx, userID, targetMachineID, "volumes.MovePreviewTarget", func(e VolumeEngine) error {
		inspected, err := e.Inspect(volumeName)
		if err != nil {
			return fmt.Errorf("failed to inspect target volume: %w", err)
		}
		collision = inspected != nil
		return nil
	})
	if err != nil {
		return nil, err
	}

	return &VolumeMovePreviewResponse{
		Collision:           collision,
		InUse:               inUse,
		ReferencingQuadlets: referencing,
	}, nil
}

// Move transfers a volume to another machine (issue 238, §19).
//
// Copy mode creates the target volume and streams the source's data into
// it — always, there is no metadata-only option — and never touches the
// source. Move mode additionally removes the source volume, but ONLY
// after the target holds the data (install-first-then-remove): an in-use
// source refuses the move with ErrVolumeInUse unless force is set —
// the delete flow's checkbox and hint — and a failed source removal
// answers the transfer envelope with status "partial" plus the failure's
// detail in Error: the volume then exists on both machines and the
// source side must be cleaned up manually. A source volume that vanished
// between the data phase and the removal needs no removal — the answer
// is "moved".
//
// The collision is re-checked at execution time in the target lease: a
// target volume that appeared since the preview answers 409
// VOLUME_ALREADY_EXISTS, never an overwrite. Any failure AFTER the
// target volume's creation rolls the half-populated volume back — the
// terminal error event says so — so a partial volume never survives to
// block a retry with a false collision.
//
// Access: the route's write-level middleware proved the caller may
// mutate the SOURCE machine; the TARGET machine is a service-side gate
// (route middleware only sees the route's machine): the caller's stored
// level must be at least write, checked against the access store BEFORE
// any podman call.
//
// Move answers through the NDJSON stream (the handler always streams);
// onProgress is the transport's feed.
func (s *VolumeService) Move(ctx context.Context, userID, machineID, volumeName string, req VolumeMoveRequest, onProgress func(VolumeMoveEvent)) (*VolumeMoveResponse, error) {
	// The target gate fires before any connection lease (the quadlet
	// move's shape, one level up from MovePreview's read gate): a lookup
	// failure is 500, a level below write — an absent row included — is
	// 403, per permission.RequireLevel's single-owner contract.
	if err := permission.RequireLevel(s.accessRepo, req.TargetMachineID, userID, models.PermissionWrite); err != nil {
		return nil, err
	}

	// Source lease: the volume must exist (404), and a move without
	// force refuses an in-use source up front — the delete flow's
	// refusal, before anything is created on the target.
	exists, err := s.volumeExists(ctx, userID, machineID, volumeName)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, appErrors.ErrVolumeNotFound
	}
	if req.Mode == VolumeMoveModeMove && !req.Force {
		err = s.usageEngines.WithEngine(ctx, userID, machineID, "volumes.MoveSourceUsage", func(u VolumeUsageEngine) error {
			inUse, err := volumeInUse(u, volumeName)
			if err != nil {
				return err
			}
			if inUse {
				return appErrors.ErrVolumeInUse
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
	}

	// Data phase (§19): two connection leases stand open at once — the
	// export reads the source machine while the import writes the target
	// — so the nested WithEngine is deliberate, the quadlet data
	// phase's arrangement. The target volume is created if absent
	// (collision re-checked first), then the shared transfer kit's
	// splice streams the data across; a failure after the creation
	// rolls the half-populated volume back.
	bytes, err := s.copyVolumeData(ctx, userID, machineID, req.TargetMachineID, volumeName, onProgress)
	if err != nil {
		return nil, err
	}

	response := &VolumeMoveResponse{
		Name:            volumeName,
		Status:          VolumeMoveStatusCopied,
		SourceMachineID: machineID,
		TargetMachineID: req.TargetMachineID,
		Bytes:           bytes,
	}
	if req.Mode == VolumeMoveModeMove {
		response.Status = VolumeMoveStatusMoved
	}

	// Move mode's source half, only now that the target holds the data.
	// A removal failure does not fail the request: the volume exists on
	// both machines, and the envelope says so with status "partial" plus
	// the failure's detail.
	if req.Mode == VolumeMoveModeMove {
		if err := s.removeSourceAfterCopy(ctx, userID, machineID, volumeName, req.Force); err != nil {
			response.Status = VolumeMoveStatusPartial
			response.Error = err.Error()
		}
	}
	return response, nil
}

// copyVolumeData runs the data phase (issue 238, §19): the target volume
// is created if absent — the collision re-checked at execution time, a
// same-named volume answers 409 VOLUME_ALREADY_EXISTS, never an
// overwrite — then the shared transfer kit streams the source's export
// into the target's import, the kit's counting reader owning the transfer
// bound. Any failure AFTER the creation removes the half-populated
// target volume (the rollback): a partial volume must not survive to
// block a retry with a false collision, and the error says the rollback
// ran — except on a cancelled context, where no narration could reach
// the wire (the streaming kit's cancelled-wins rule; the ctx.Err()
// branch says so). Progress flows through onProgress (nil emits nothing).
func (s *VolumeService) copyVolumeData(ctx context.Context, userID, sourceMachineID, targetMachineID, volumeName string, onProgress func(VolumeMoveEvent)) (int64, error) {
	emit := func(phase string, bytes int64) {
		if onProgress != nil {
			// OperationID stays empty: the streaming handler stamps its
			// own (the quadlet move's arrangement).
			onProgress(NewVolumeMoveProgressEvent("", sourceMachineID, volumeName, phase, bytes))
		}
	}

	var streamed int64
	err := s.engines.WithEngine(ctx, userID, sourceMachineID, "volumes.MoveDataSource", func(src VolumeEngine) error {
		return s.engines.WithEngine(ctx, userID, targetMachineID, "volumes.MoveDataTarget", func(tgt VolumeEngine) error {
			// The collision re-check: the preview's flag is stale the
			// moment it answered. An existing target volume is a 409 —
			// the transfer never overwrites or merges (§18's
			// overwriteVolumes stays a quadlet-phase concern).
			inspected, err := tgt.Inspect(volumeName)
			if err != nil {
				return fmt.Errorf("failed to inspect target volume: %w", err)
			}
			if inspected != nil {
				return appErrors.ErrVolumeAlreadyExists
			}

			if _, err := tgt.Create(entities.VolumeCreateOptions{Name: volumeName}); err != nil {
				if podmanerr.IsConflict(err) {
					return appErrors.ErrVolumeAlreadyExists
				}
				return fmt.Errorf("failed to create target volume: %w", err)
			}

			// Everything after this point owns the rollback: the target
			// volume exists, so any failure must remove it again.
			emit(VolumeMovePhaseStart, 0)
			count, err := transfer.CopyVolumeData(ctx,
				volumeExportAdapter{e: src}, volumeImportAdapter{e: tgt},
				volumeName, s.volumeDataLimit(), func(count int64) {
					emit(VolumeMovePhaseBytes, count)
				})
			if err == nil {
				emit(VolumeMovePhaseDone, count)
				streamed = count
				return nil
			}

			// The rollback: a half-populated volume must not survive —
			// it would block a retry with a false collision. Its own
			// failure cannot mask the transfer's; both ride the message.
			_, rollbackErr := tgt.Remove(volumeName, false)
			if errors.Is(err, transfer.ErrBoundExceeded) {
				if rollbackErr != nil {
					return appErrors.ErrVolumeUploadTooLarge.WithMessage(fmt.Sprintf(
						"Volume %s exceeded the transfer bound at %d bytes; removing the partially transferred target volume failed too: %v",
						volumeName, count, rollbackErr))
				}
				return appErrors.ErrVolumeUploadTooLarge.WithMessage(fmt.Sprintf(
					"Volume %s exceeded the transfer bound at %d bytes; the partially transferred target volume was removed",
					volumeName, count))
			}
			if ctx.Err() != nil {
				// The rollback above already ran: the engine calls carry
				// no ctx, so the still-open target lease performs the
				// removal whatever happened to the request. No
				// narration rides this error, by design — the handler
				// streams through the kit whose cancelled-wins rule
				// (streaming.Run's terminalError) answers a cancelled
				// stream with the generic OPERATION_CANCELLED terminal
				// event whatever the operation returns, so a rollback
				// note here could never reach the wire.
				return ctx.Err()
			}
			if rollbackErr != nil {
				return appErrors.ErrVolumeMigrationFailed.WithMessage(fmt.Sprintf(
					"Volume %s: %v; removing the partially transferred target volume failed too: %v",
					volumeName, err, rollbackErr))
			}
			return appErrors.ErrVolumeMigrationFailed.WithMessage(fmt.Sprintf(
				"Volume %s: %v; the partially transferred target volume was removed",
				volumeName, err))
		})
	})
	return streamed, err
}

// removeSourceAfterCopy performs the move mode's source-side half after
// the target holds the data: the delete flow's remove with force passed
// through. A vanished source volume answers nil — the transfer's goal is
// met and a "partial" would falsely claim the volume exists on both
// machines. Its error is the "partial" detail, never a request failure —
// the data phase it follows cannot be rolled back.
func (s *VolumeService) removeSourceAfterCopy(ctx context.Context, userID, machineID, volumeName string, force bool) error {
	return s.engines.WithEngine(ctx, userID, machineID, "volumes.MoveSourceRemove", func(e VolumeEngine) error {
		removed, err := e.Remove(volumeName, force)
		if err != nil {
			return fmt.Errorf("failed to remove volume: %w", err)
		}
		for _, report := range removed {
			if report == nil || report.Err == nil {
				continue
			}
			if podmanerr.IsNotFound(report.Err) {
				// The source vanished between the data phase and the
				// removal — the transfer's goal is met.
				return nil
			}
			if appErr := mapInUseError(report.Err); appErr != nil {
				return appErr
			}
			return fmt.Errorf("failed to remove volume: %w", report.Err)
		}
		return nil
	})
}

// volumeExists answers whether the volume exists on the machine through
// the volume engine seam.
func (s *VolumeService) volumeExists(ctx context.Context, userID, machineID, volumeName string) (bool, error) {
	exists := false
	err := s.engines.WithEngine(ctx, userID, machineID, "volumes.VolumeExists", func(e VolumeEngine) error {
		inspected, err := e.Inspect(volumeName)
		if err != nil {
			return fmt.Errorf("failed to inspect volume: %w", err)
		}
		exists = inspected != nil
		return nil
	})
	return exists, err
}

// volumeInUse answers whether any container on the machine mounts the
// volume (issue 238). Podman's Service API reports no in-use flag on
// volume inspect and no volume filter on container list, so in-use is
// derived: every container's user-declared mounts, each scanned for the
// volume's exact name — the source as written, the token before the
// first ":" (`Volume=name:/mnt:ro`).
func volumeInUse(e VolumeUsageEngine, volumeName string) (bool, error) {
	containers, err := e.ContainerMounts()
	if err != nil {
		return false, fmt.Errorf("failed to list containers: %w", err)
	}
	for _, container := range containers {
		for _, mount := range container.Mounts {
			if volumeMountSource(mount) == volumeName {
				return true, nil
			}
		}
	}
	return false, nil
}

// volumeMountSource extracts one mount string's volume source: the
// token before the first ":" (`name:/dest:opts`), or the whole string
// when no ":" is present.
func volumeMountSource(mount string) string {
	if idx := strings.Index(mount, ":"); idx >= 0 {
		return mount[:idx]
	}
	return mount
}

// volumeExportAdapter widens the volumes engine's Export to the shared
// transfer kit's source width (§19): the method names differ, the call
// is one line.
type volumeExportAdapter struct{ e VolumeEngine }

func (a volumeExportAdapter) VolumeExport(name string, w io.Writer) error {
	return a.e.Export(name, w)
}

// volumeImportAdapter widens the volumes engine's Import to the shared
// transfer kit's target width (§19).
type volumeImportAdapter struct{ e VolumeEngine }

func (a volumeImportAdapter) VolumeImport(name string, r io.Reader) error {
	return a.e.Import(name, r)
}
