package quadlets

import (
	"context"
	"errors"
	"fmt"

	appErrors "github.com/Homerus25/podland/backend/internal/errors"
	"github.com/Homerus25/podland/backend/internal/models"
	"github.com/Homerus25/podland/backend/internal/permission"
	"github.com/Homerus25/podland/backend/internal/podmanerr"
	"github.com/Homerus25/podland/backend/internal/transfer"
	"go.podman.io/podman/v6/pkg/domain/entities"
)

// lifecycleSeam is the narrow seam the transfer ritual crosses into the
// lean service's lifecycle half (architecture review 2026-10-19, candidate
// C2): the three callbacks MoveWithProgress makes there — stop the source
// unit (the lifecycle's unit action with actionStop), reload the target's
// systemd manager, and the stop-and-remove source half with the move's
// terminals. It is consumer-declared here, in the transfer module, and
// satisfied implicitly by *QuadletService.
type lifecycleSeam interface {
	applyUnitAction(ctx context.Context, userID, machineID string, action lifecycleAction, unit string, opts LifecycleOptions) error
	reloadUserSystemd(ctx context.Context, machineID, userID string) error
	stopAndRemove(ctx context.Context, userID, machineID, name string, force, stop bool, op string, terminals removeTerminals) error
}

// Compile-time pin: the lean service satisfies the transfer's seam.
var _ lifecycleSeam = (*QuadletService)(nil)

// QuadletTransferService owns the cross-machine move/copy transfer (issues
// 209-215): the preview, the move (plain and progress), the volume-data
// phase, and the secret-transfer phase. It is the transfer half of the
// quadlets domain split (architecture review 2026-10-19, candidate C2) —
// the lean QuadletService keeps CRUD, lifecycle, logs, generate, import,
// and auto-update, and this service crosses into that half through the
// lifecycleSeam.
type QuadletTransferService struct {
	// engines is the connection seam both machines' leases cross.
	engines QuadletEngineProvider
	// secretEngines resolves the podman secret stores the move/copy's
	// secret surface crosses (issue 215): the existence reads for the
	// preview, the value reveal on the source and the create on the
	// target for the transfer.
	secretEngines moveSecretEngineProvider
	// accessRepo resolves the caller's permission level for the
	// cross-machine operations: route middleware only sees the route's
	// machine, so the target machine's access is checked here, against
	// the store, before any podman call.
	accessRepo permission.PermissionGetter
	// maxVolumeUploadBytes bounds the volume data migration's stream
	// (issue 214, §18), enforced by the data phase's counting reader —
	// the same VOLUME_UPLOAD_MAX_BYTES config value the volumes domain
	// caps populate uploads with; zero resolves through the transfer
	// kit's BoundOrDefault to its DefaultMaxVolumeUploadBytes (the kit
	// owns the default per §19).
	maxVolumeUploadBytes int64
	// lifecycle is the seam into the lean service's lifecycle half: the
	// migrating move's source stop, the target's manager reload, and the
	// move mode's stop-and-remove source half.
	lifecycle lifecycleSeam
}

// NewQuadletTransferService creates the transfer service with all five
// dependencies wired. The constructor is flat — a missing wiring is a
// compile error instead of a panic at the seam. maxVolumeUploadBytes == 0
// keeps the transfer kit's default fallback meaning for tests.
func NewQuadletTransferService(engines QuadletEngineProvider, secretEngines moveSecretEngineProvider, accessRepo permission.PermissionGetter, maxVolumeUploadBytes int64, lifecycle lifecycleSeam) *QuadletTransferService {
	return &QuadletTransferService{
		engines:              engines,
		secretEngines:        secretEngines,
		accessRepo:           accessRepo,
		maxVolumeUploadBytes: maxVolumeUploadBytes,
		lifecycle:            lifecycle,
	}
}

// volumeDataLimit answers the wired migration bound; the zero-value
// fallback resolves through the transfer kit's BoundOrDefault — the
// kit owns the default (§19), so this fallback and the volumes
// domain's copy/move bound cannot drift apart.
func (s *QuadletTransferService) volumeDataLimit() int64 {
	return transfer.BoundOrDefault(s.maxVolumeUploadBytes)
}

// checkDeletePreconditions evaluates the not-found and running-resource guards
// for a delete operation against the target quadlet's parsed state. It returns
// ErrQuadletNotFound when the target is absent, ErrQuadletResourceRunning when
// the managed resource is running and neither force nor the stop-before-remove
// waiver applies, or nil when the delete may proceed (with a stop first when
// the resource is running and stop is set).
//
// Its surviving caller is the move preflight (MoveWithProgress below); Delete's
// own pre-check is the shared ritual in remove.go now. The running test
// delegates to quadletResourceRunning, the one derivation the ritual's
// body also evaluates, so the preflight and the ritual cannot drift
// apart (issue 231).
func checkDeletePreconditions(target *entities.ListQuadlet, force, stop bool) error {
	if target == nil {
		return appErrors.ErrQuadletNotFound
	}
	if quadletResourceRunning(target) && !force && !stop {
		return appErrors.ErrQuadletResourceRunning
	}
	return nil
}

// MovePreview answers everything the move/copy transfer dialog must show
// before confirming (issue 209): the volumes the quadlet references, whether
// the target machine already has a quadlet of the same name and resource,
// and whether the source quadlet's managed resource is running. Volume
// entries whose source names a companion .volume quadlet on the source
// machine additionally carry that file and its state on the target (issue
// 217). Every volume-kind entry also carries its resolved podman volume and
// the migration facts both machines hold about it — sizes when the engines
// report any, target existence, migratability — regardless of what the
// transfer request later opts into (issue 214). Every `Secret=` reference
// carries its resolved name and kind plus its existence in each machine's
// podman secret store, always computed and flag-independent (issue 215). See
// doku/description_quadlets_api.md.
//
// Access: the route's subtree middleware already proved the caller holds an
// access row on the source machine — read level suffices, this endpoint
// mutates nothing. The target machine is a service-side gate (route
// middleware only sees the route's machine): the caller's stored level must
// be at least read, checked against the access store BEFORE any podman call,
// so a caller without target access gets the standard 403 ACCESS_DENIED and
// never a false preview. The target's quadlet list itself is read through
// the connection seam on the target's machine ID.
func (s *QuadletTransferService) MovePreview(ctx context.Context, userID, machineID, quadletID, targetMachineID string) (*QuadletMovePreviewResponse, error) {
	// The target gate fires before the connection lease (the same shape as
	// the secrets reveal gate): a lookup failure is 500, a level below read
	// — an absent row included — is 403, per permission.RequireLevel's
	// single-owner contract.
	if err := permission.RequireLevel(s.accessRepo, targetMachineID, userID, models.PermissionRead); err != nil {
		return nil, err
	}

	// Source lease: content (for the volume parse) plus the list entry
	// (for resource and live state). A missing quadlet is a 404. The
	// content parses once per lease (issue 230): the companion set and the
	// volume resolution both read the same refs, and each companion file
	// is Printed once — its verbatim content feeds both the companion
	// install set and the resolution's VolumeName= extraction. The
	// resolution itself is the one derivation the transfer's plan also
	// reads (issue 230, §18), so the preview cannot promise what the
	// transfer would do differently (issue 214).
	var content string
	var source *entities.ListQuadlet
	var companions []companionVolumeQuadlet
	var resolution volumeResolution
	err := s.engines.WithEngine(ctx, userID, machineID, "quadlets.MovePreview", func(e QuadletEngine) error {
		c, list, listed, err := fetchQuadletWithList(e, quadletID)
		if err != nil {
			return err
		}
		source = listed

		refs := parseQuadletVolumes(c)
		if companions, err = companionVolumeQuadlets(refs, list, e); err != nil {
			return err
		}
		if resolution, err = deriveVolumeResolution(refs, companions, e); err != nil {
			return err
		}

		content = c
		return nil
	})
	if err != nil {
		return nil, err
	}

	// Target lease: the collision check reads the target's quadlet list,
	// and each companion's file is read through the same seam to label its
	// target state (absent / identical / differs, issue 217). Presence is
	// decided by the LIST, not the print: podman's print answers an empty
	// string (no error) for a file it does not know, so only a listed
	// file's content is worth comparing.
	var collision bool
	targetStates := map[string]string{}
	// Target volume inspects (issue 214): existence and size per resolved
	// podman volume; a nil inspect is an absent volume (ExistsOnTarget
	// false — a fresh target, the honest answer).
	targetVolumes := map[string]*QuadletVolumeInspect{}
	err = s.engines.WithEngine(ctx, userID, targetMachineID, "quadlets.MovePreviewTarget", func(e QuadletEngine) error {
		list, err := e.List(entities.QuadletListOptions{})
		if err != nil {
			return fmt.Errorf("failed to list target quadlets: %w", err)
		}

		collision = quadletNameAndResourceTaken(list, source.Name, deriveResourceFromPath(source.Path))

		for _, companion := range companions {
			if findQuadletByName(list, companion.File) == nil {
				targetStates[companion.File] = MovePreviewCompanionAbsent
				continue
			}
			targetContent, err := e.Print(companion.File)
			if err != nil {
				return fmt.Errorf("failed to read target volume quadlet %s: %w", companion.File, err)
			}
			if targetContent == companion.Content {
				targetStates[companion.File] = MovePreviewCompanionIdentical
			} else {
				targetStates[companion.File] = MovePreviewCompanionDiffers
			}
		}

		for _, volume := range resolution.Volumes {
			inspected, err := e.VolumeInspect(volume.Name)
			if err != nil {
				return fmt.Errorf("failed to inspect target volume %s: %w", volume.Name, err)
			}
			targetVolumes[volume.Name] = inspected
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	// The one projection into the preview's wire rows (issue 230, §18):
	// per-reference entries in file order — two references resolving to
	// one volume stay two entries sharing podmanVolume — with the
	// companion fields where discovery resolved a source file and the
	// migration facts where the reference resolves to a podman volume at
	// all (issues 217 and 214).
	volumes := ToMovePreviewVolumes(resolution, targetStates, targetVolumes)

	// Secret references (issue 215): every `Secret=` line, always
	// computed, flag-independent — the volume facts' discipline. Existence
	// means "in the machine's podman secret store" and is only claimed for
	// kind secret entries; a hostPath source is read off the host
	// filesystem at run time and never consults a store, so it carries no
	// claims. The two name lists ride their own connection leases on the
	// machines the quadlet leases above dialed — the source list read on
	// the source (read level suffices, the preview mutates nothing), the
	// target list through the connection seam like the collision read.
	// The leases open only when a kind secret entry needs them.
	secrets := parseQuadletSecrets(content)
	sourceSecretNames := map[string]bool{}
	targetSecretNames := map[string]bool{}
	needsSecretStores := false
	for _, secret := range secrets {
		if secret.Kind == MovePreviewSecretKindSecret {
			needsSecretStores = true
			break
		}
	}
	if needsSecretStores {
		err = s.secretEngines.WithEngine(ctx, userID, machineID, "quadlets.MovePreviewSecretsSource", func(e moveSecretEngine) error {
			names, err := e.Names()
			if err != nil {
				return fmt.Errorf("failed to list source secrets: %w", err)
			}
			for _, name := range names {
				sourceSecretNames[name] = true
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
		err = s.secretEngines.WithEngine(ctx, userID, targetMachineID, "quadlets.MovePreviewSecretsTarget", func(e moveSecretEngine) error {
			names, err := e.Names()
			if err != nil {
				return fmt.Errorf("failed to list target secrets: %w", err)
			}
			for _, name := range names {
				targetSecretNames[name] = true
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	for i := range secrets {
		if secrets[i].Kind != MovePreviewSecretKindSecret {
			continue
		}
		secrets[i].ExistsOnSource = sourceSecretNames[secrets[i].Name]
		secrets[i].ExistsOnTarget = targetSecretNames[secrets[i].Name]
	}

	return &QuadletMovePreviewResponse{
		Volumes:   volumes,
		Secrets:   secrets,
		Collision: collision,
		Running:   parseStateFromStatus(source.Status) == "running",
	}, nil
}

// fetchQuadletWithList is the source-lease ritual both machine reads share
// (MovePreview and MoveWithProgress's source lease): the quadlet's printed
// content (for the volume/secret parse) plus its list entry (for name,
// resource, and live state), fetched under one engine lease. A missing
// quadlet is a 404 — both the print's not-found and a list that fails to
// name the file. The full list rides along: both callers derive their
// companion set and volume resolution from it while the lease is open.
func fetchQuadletWithList(e QuadletEngine, quadletID string) (content string, list []*entities.ListQuadlet, source *entities.ListQuadlet, err error) {
	content, err = e.Print(quadletID)
	if err != nil {
		if podmanerr.IsNotFound(err) {
			return "", nil, nil, appErrors.ErrQuadletNotFound
		}
		return "", nil, nil, fmt.Errorf("failed to get quadlet content: %w", err)
	}

	list, err = e.List(entities.QuadletListOptions{})
	if err != nil {
		return "", nil, nil, fmt.Errorf("failed to list quadlets: %w", err)
	}

	source = findQuadletByName(list, quadletID)
	if source == nil {
		return "", nil, nil, appErrors.ErrQuadletNotFound
	}
	return content, list, source, nil
}

// Move transfers a quadlet to another machine (issues 210 and 211).
//
// Copy mode installs the source quadlet's file — same name, resource, and
// content — on the target without overwriting and never touches the source:
// no stop, no removal, not even for a running quadlet; the stop/force flags
// are move-mode vocabulary and are ignored. The installed unit is never
// started, mirroring Create.
//
// Move mode removes the source quadlet, but ONLY after the target install
// succeeded (target manager reload included): the source lease first
// evaluates the delete flow's preconditions against the source's live
// state — a running source quadlet refuses the move with
// ErrQuadletResourceRunning unless stop or force is set, before anything is
// installed — then the target lease installs, and only then does a second
// source lease perform the removal (stop-then-remove with the stop flag,
// force passed through to the remove, the delete flow's error mapping). An
// install or reload failure therefore answers a plain error with the source
// untouched. A removal failure after a successful install answers the
// transfer envelope with status "partial" plus the failure's detail in
// Error: the quadlet then exists on BOTH machines and the source side must
// be cleaned up manually. A source quadlet that vanished between install
// and removal needs no removal — the answer is "moved".
//
// Companion .volume quadlets (issue 217): when the quadlet references
// volume sources a same-named .volume quadlet file on the source machine
// defines, the transfer carries those definitions along unless the request
// opts out with transferVolumeQuadlets=false. The companion set is
// re-derived from the source content at execution time — the client never
// names files — and installs FIRST (dependencies before the dependent); a
// companion already on the target byte-identical is skipped, differing
// content refuses the transfer with 409, and the first failure stops the
// set naming the failed file (companions already installed stay; a re-run
// is safe because identical files skip). Companions are copy-only: a move
// removes ONLY the main quadlet — companions are shared definitions other
// source quadlets may reference. The volumes' DATA follows only when the
// request opts into migrateVolumeData (issue 214, §18).
//
// Volume data migration (issue 214, architecture-decisions §18): with
// migrateVolumeData set, every volume-kind `Volume=` reference is resolved
// from the source content at execution time and, in order, preflighted
// against the target and then carried: podman export on the source streams
// through a counting reader into podman import on the target. The phases
// run before anything is installed — preflight (target emptiness, 409
// VOLUME_MIGRATION_TARGET_NOT_EMPTY; size bound, 413), then the secret
// phase when opted into, then a move's stop, then the data itself — so a
// refusal or a data failure answers an error with both machines' quadlet
// sets untouched. A migrating move stops the source's running resource
// BEFORE the data phase (exporting under a live writer would capture torn
// state); the non-migration path keeps the delete flow's stop inside the
// removal, byte-identical to its issue-211 shape. Any failure after that
// stop carries the stop note — the source quadlet was stopped before this
// failure and remains stopped — because the source's runtime state was
// mutated even though the quadlet sets were not (issue 207 review).
// Per-volume outcomes ride the envelope's volumeMigration block; progress
// events flow through onProgress (nil for the plain transport).
//
// Secret transfer (issue 215): with transferSecrets set, every `Secret=`
// reference is re-derived from the source content at execution time — the
// client never names secrets — and, in phase order after the volume
// preflight but BEFORE a migrating move's stop and the data phase (cheap
// failures abort before any state is mutated or heavy bytes move), each
// reference's value is revealed on the source and the secret created on
// the target. The admin
// gate on the source fired in the preflight (a reveal plus a create is a
// value movement; below-admin is refused with 403 before anything moves).
// A same-name secret already on the target is skipped, never overwritten —
// the skip is checked before the reveal, so the value is never read when
// nothing needs it — which keeps re-runs safe; dangling and host-path
// references are skipped and reported, not errors. Secrets are copy-only —
// never removed from the source, any mode, any outcome (other source
// quadlets may reference them). Any secret failure aborts the transfer
// with 500 SECRET_TRANSFER_FAILED before anything is installed, source
// untouched; secrets delivered earlier stay on the target — inert, skipped
// on re-run. Per-reference outcomes ride the envelope's secretTransfer
// block; no progress events and no NDJSON transport ride on secrets alone.
//
// Access: the route's write-level middleware proved the caller may mutate
// the SOURCE machine; the TARGET machine is a service-side gate (route
// middleware only sees the route's machine): the caller's stored level must
// be at least write, checked against the access store BEFORE any podman
// call, so a read-level grantee is refused with the standard 403
// ACCESS_DENIED before anything is installed. The transfer itself then
// crosses the connection seam once per machine: the source lease reads
// content and list entry (and, in move mode, the removal lease), the target
// lease re-checks the collision and installs.
//
// Move answers through the plain JSON envelope; MoveWithProgress is the
// full orchestration and reports data-phase progress. The two share every
// semantic — Move just discards the progress callback.
func (s *QuadletTransferService) Move(ctx context.Context, userID, machineID, quadletID string, req QuadletMoveRequest) (*QuadletMoveResponse, error) {
	return s.MoveWithProgress(ctx, userID, machineID, quadletID, req, nil)
}

// MoveWithProgress performs the transfer and reports volume data-phase
// progress through onProgress (the NDJSON transport's feed; nil progress
// simply emits nothing). See Move for the transfer semantics.
func (s *QuadletTransferService) MoveWithProgress(ctx context.Context, userID, machineID, quadletID string, req QuadletMoveRequest, onProgress func(QuadletMoveEvent)) (*QuadletMoveResponse, error) {
	// The target gate fires before any connection lease (the same shape as
	// the secrets reveal gate, one level up from MovePreview's read gate):
	// a lookup failure is 500, a level below write — an absent row included
	// — is 403, per permission.RequireLevel's single-owner contract.
	if err := permission.RequireLevel(s.accessRepo, req.TargetMachineID, userID, models.PermissionWrite); err != nil {
		return nil, err
	}

	// The secret transfer's admin gate on the SOURCE (issue 215): a
	// transferSecrets request moves VALUES off this machine, so the reveal
	// gate's admin level stays the single owner of value exposure — a
	// write-level caller must never be able to move values off a machine.
	// Fires in the preflight, before any byte moves.
	if req.TransferSecrets {
		if err := permission.RequireLevel(s.accessRepo, machineID, userID, models.PermissionAdmin); err != nil {
			return nil, err
		}
	}

	// Source lease: content plus the list entry (for name, resource, and
	// live state). A missing quadlet is a 404. The lease only reads — the
	// move mode's removal runs in its own lease after the install, so a
	// transfer onto the source's own machine finds its own collision below
	// and installs (and therefore removes) nothing. The companion set is
	// derived here when the request carries companions along (issue 217):
	// re-derived from the source content at execution time, the same
	// discipline as the collision re-check.
	var content string
	var source *entities.ListQuadlet
	var companions []companionVolumeQuadlet
	var plan []volumeMigrationPlanEntry
	err := s.engines.WithEngine(ctx, userID, machineID, "quadlets.MoveSource", func(e QuadletEngine) error {
		c, list, listed, err := fetchQuadletWithList(e, quadletID)
		if err != nil {
			return err
		}
		source = listed

		// Move mode's delete preconditions, evaluated BEFORE anything is
		// installed: a running source quadlet refuses the move unless stop
		// or force is set (copy mode stops nothing and never gets here).
		if req.Mode == MoveModeMove {
			if err := checkDeletePreconditions(source, req.Force, req.Stop); err != nil {
				return err
			}
		}

		// One parse per lease (issue 230): the companion set and the
		// volume resolution both read these refs, and each companion file
		// is Printed once — its verbatim content feeds both the install
		// set and the resolution's VolumeName= extraction.
		refs := parseQuadletVolumes(c)

		// The companion contents are fetched once per lease (issue 230)
		// whenever either consumer needs them: the install set below, or
		// the volume resolution when the request migrates data.
		if req.WantsVolumeQuadletTransfer() || req.MigrateVolumeData {
			if companions, err = companionVolumeQuadlets(refs, list, e); err != nil {
				return err
			}
		}

		// Volume data migration planning (issue 214, §18): the same
		// derivation the preview reads (issue 230) resolves every
		// volume-kind reference against the source and inspects each
		// resolved volume once while the lease is open. The size bound's
		// pre-check fires when an engine reports an inspect size —
		// production never does (podman's service API has no volume
		// usage), so the counting reader during the data phase stays the
		// always-on enforcement.
		if req.MigrateVolumeData {
			resolution, err := deriveVolumeResolution(refs, companions, e)
			if err != nil {
				return err
			}
			for _, volume := range resolution.Volumes {
				if volume.Inspect != nil && volume.Inspect.Size != nil && *volume.Inspect.Size > s.volumeDataLimit() {
					return appErrors.ErrVolumeUploadTooLarge.WithMessage(fmt.Sprintf(
						"Volume %s is %d bytes, above the %d byte migration bound",
						volume.Name, *volume.Inspect.Size, s.volumeDataLimit()))
				}
			}
			plan = deriveVolumeMigrationPlan(resolution)
		}

		content = c
		return nil
	})
	if err != nil {
		return nil, err
	}

	resource := deriveResourceFromPath(source.Path)

	// Volume data migration phases (issue 214, §18) — all BEFORE anything
	// is installed, so a refusal or a data failure leaves both machines'
	// quadlet sets untouched. Phase order: the target preflight first,
	// then the secret phase (issue 215) — its cheap failures must abort
	// before any state is mutated, and secrets never need the source
	// stopped — then a migrating move's stop, then the data itself.
	var volumeOutcomes []QuadletMoveVolumeOutcome
	if req.MigrateVolumeData {
		// Preflight against the target: emptiness (409) and engine
		// failures name their volume. The collision re-check below still
		// runs in the install lease; this earlier target lease exists so a
		// volume refusal lands before a stop or any data moves.
		if err := s.preflightVolumeMigration(ctx, userID, req.TargetMachineID, plan, req.OverwriteVolumes); err != nil {
			return nil, err
		}
	}

	// Secret transfer phase (issue 215), after the preflight but before
	// the migrating move's stop — a cheap secret failure aborts the whole
	// transfer with SECRET_TRANSFER_FAILED while nothing has been mutated
	// or installed yet; secrets delivered earlier stay on the target and
	// are skipped on re-run.
	var secretOutcomes []QuadletMoveSecretOutcome
	if req.TransferSecrets {
		outcomes, err := s.transferSecrets(ctx, userID, machineID, req.TargetMachineID, content)
		if err != nil {
			return nil, err
		}
		secretOutcomes = outcomes
	}

	// The migrating move's stop, immediately before the data phase:
	// exporting a volume under a live writer would capture torn state.
	// The delete flow's stop-before-remove gate (running && stop && !force);
	// move mode only — copy never stops anything. The non-migration path
	// keeps the stop inside the removal. Everything after a successful
	// stop annotates its failure (noteStopped): the quadlet sets stay
	// untouched, but the stop mutated the source's runtime state and the
	// error says so.
	var stopApplied bool
	if req.MigrateVolumeData {
		if req.Mode == MoveModeMove && parseStateFromStatus(source.Status) == "running" && req.Stop && !req.Force {
			if err := s.lifecycle.applyUnitAction(ctx, userID, machineID, actionStop, resolveUnitName(source, quadletID), LifecycleOptions{}); err != nil {
				return nil, fmt.Errorf("failed to stop quadlet before volume migration: %w", err)
			}
			stopApplied = true
		}
		outcomes, err := s.migrateVolumeData(ctx, userID, machineID, req.TargetMachineID, plan, onProgress)
		if err != nil {
			return nil, noteStopped(err, stopApplied)
		}
		volumeOutcomes = outcomes
	}

	// Target lease: re-check the collision at execution time — the install
	// must not overwrite — then install the companions first (dependencies
	// before the dependent, issue 217), then the main file verbatim (the
	// source's name carries its extension, so it names the target file
	// too).
	var response *QuadletMoveResponse
	err = s.engines.WithEngine(ctx, userID, req.TargetMachineID, "quadlets.MoveTarget", func(e QuadletEngine) error {
		list, err := e.List(entities.QuadletListOptions{})
		if err != nil {
			return fmt.Errorf("failed to list target quadlets: %w", err)
		}
		if quadletNameAndResourceTaken(list, source.Name, resource) {
			return appErrors.ErrQuadletAlreadyExists
		}

		for _, companion := range companions {
			if err := installCompanionVolumeQuadlet(e, companion, list); err != nil {
				return err
			}
		}

		report, err := e.Install(source.Name, []byte(content), false)
		if err != nil {
			if podmanerr.IsAlreadyExists(err) {
				return appErrors.ErrQuadletAlreadyExists
			}
			return fmt.Errorf("failed to install quadlet on target: %w", err)
		}
		// A conflict surfaces as an entry in QuadletErrors keyed by source path.
		if report != nil {
			for _, qErr := range report.QuadletErrors {
				if qErr != nil && podmanerr.IsAlreadyExists(qErr) {
					return appErrors.ErrQuadletAlreadyExists
				}
			}
		}

		response = &QuadletMoveResponse{
			ID:              source.Name,
			Name:            source.Name,
			Resource:        resource,
			Status:          MoveStatusCopied,
			SourceMachineID: machineID,
			TargetMachineID: req.TargetMachineID,
		}
		if req.Mode == MoveModeMove {
			response.Status = MoveStatusMoved
		}
		if report != nil {
			response.Path = firstInstalledPath(report.InstalledQuadlets)
		}
		return nil
	})
	if err != nil {
		// A post-stop install failure (collision re-check 409, engine
		// failure) answers annotated: the source may sit stopped.
		return nil, noteStopped(err, stopApplied)
	}

	// The data phase's outcomes ride the envelope (issue 214): present even
	// when every reference was skipped, so skipped danglings stay visible.
	if req.MigrateVolumeData {
		response.VolumeMigration = &QuadletMoveVolumeMigration{Volumes: volumeOutcomes}
	}
	// The secret phase's outcomes ride the envelope beside it (issue 215):
	// present whenever the request opted into transferSecrets, even when
	// every reference was skipped, so skipped danglings and host paths
	// stay visible.
	if req.TransferSecrets {
		response.SecretTransfer = &QuadletMoveSecretTransfer{Secrets: secretOutcomes}
	}

	// The same manager-visibility guarantee Create gives a single machine:
	// podman's internal reload reaches the manager the podman service sees,
	// not necessarily the one the lifecycle's scoped systemctl calls address,
	// so the TARGET's manager is reloaded explicitly. A reload failure fails
	// the transfer with ErrConnectionFailed even though the file is
	// installed — and, in move mode, before the source is touched at all.
	if err := s.lifecycle.reloadUserSystemd(ctx, req.TargetMachineID, userID); err != nil {
		return nil, noteStopped(err, stopApplied)
	}

	// Move mode's source half, only now that the install fully succeeded.
	// A removal failure does not fail the request: the quadlet exists on
	// both machines, and the envelope says so with status "partial" plus
	// the failure's detail.
	if req.Mode == MoveModeMove {
		if err := s.removeSourceAfterInstall(ctx, userID, machineID, quadletID, req); err != nil {
			response.Status = MoveStatusPartial
			response.Error = err.Error()
		}
	}
	return response, nil
}

// removeSourceAfterInstall performs the move mode's source-side half after
// the target install succeeded: the shared stop-and-remove ritual
// (stopAndRemove, remove.go — issue 231) with the transfer's terminals,
// crossed through the lifecycle seam into the lean service. A
// vanished source quadlet answers nil — the transfer's goal is met and a
// "partial" would falsely claim the quadlet exists on both machines — and a
// running-resource refusal answers the wrapped sentinel that becomes the
// partial detail (a dropping closure: the cause is dropped today). Its
// error is the "partial" detail, never a request failure — the install it
// follows cannot be rolled back.
//
// The removal rides its own source lease and re-lists first: the state the
// precondition saw belongs to the earlier lease, and the honest decisions
// (stop or not, nothing-to-remove or not) need the fresh one. A quadlet
// that started since with stop=false now refuses up front in the ritual —
// issue 231's accepted tightening: same partial status, byte-identical
// detail, where the old path asked podman to refuse at the remove instead.
// No existing test pinned the old path.
func (s *QuadletTransferService) removeSourceAfterInstall(ctx context.Context, userID, machineID, quadletID string, req QuadletMoveRequest) error {
	return s.lifecycle.stopAndRemove(ctx, userID, machineID, quadletID, req.Force, req.Stop, "quadlets.MoveSourceRemove", removeTerminals{
		missing: nil,
		// The running refusal's cause (podman's text) is dropped today:
		// the wire answers the wrapped sentinel that becomes the partial
		// detail.
		running: func(string) error {
			return fmt.Errorf("failed to remove quadlet: %w", appErrors.ErrQuadletResourceRunning)
		},
	})
}

// stopNote is what any failure after the migrating move's stop admits
// (issue 207 review): the transfer answers a plain error and leaves both
// machines' quadlet sets untouched, but the source's unit was already
// stopped and stays stopped — the error says so instead of going silent.
const stopNote = "the source quadlet was stopped before this failure and remains stopped"

// noteStopped annotates a failure that followed the migrating move's stop,
// one annotation point for every post-stop failure (the data phase, the
// install lease, the target reload). AppErrors keep their code (409
// refusal, 500 migration failure, 413 bound) and take the note in their
// message; anything else is wrapped. The stop only exists on the migrating
// path, so every annotated failure rides the NDJSON terminal error event,
// where code and message survive.
func noteStopped(err error, stopApplied bool) error {
	if !stopApplied {
		return err
	}
	if appErr, ok := errors.AsType[*appErrors.AppError](err); ok {
		return appErr.WithMessage(fmt.Sprintf("%s; %s", appErr.Message, stopNote))
	}
	return fmt.Errorf("%w; %s", err, stopNote)
}

// quadletNameAndResourceTaken reports whether the list already holds a
// quadlet of the same name and resource — the move/copy collision contract.
// Name is the quadlet's file name (with extension); resource is derived from
// the install path the same way every list response derives it.
func quadletNameAndResourceTaken(list []*entities.ListQuadlet, name, resource string) bool {
	for _, q := range list {
		if q == nil {
			continue
		}
		if q.Name == name && deriveResourceFromPath(q.Path) == resource {
			return true
		}
	}
	return false
}

// companionVolumeQuadlet is one companion .volume quadlet file the
// transferred quadlet references by filename (issue 217): File is the
// file's name (extension included) and Content its verbatim bytes, both
// read on the SOURCE machine.
type companionVolumeQuadlet struct {
	File    string
	Content string
}

// companionVolumeQuadlets derives the companion set from the quadlet's
// parsed volume refs against the source machine's own list (issue 217):
// every parsed volume source that names a candidate (suffix .volume, no
// path separator — the shared transfer kit's companion discovery rule,
// §19 issue 242) AND has a same-named quadlet file on the source machine
// — exact filename match — yields that file with its printed content. A
// candidate with no matching source file is dangling and yields nothing
// (the reference stays an ordinary volume entry); bare names and host
// paths never match at all. The set is file-ordered by first reference and
// deduplicated, so one companion file referenced twice prints once — the
// issue-230 guarantee that each companion is Printed exactly once per
// source lease, its content feeding both the install set and the volume
// resolution. Both the preview and the transfer call this — the transfer
// re-derives at execution time, never trusting what a preview saw.
func companionVolumeQuadlets(refs []volumeRef, list []*entities.ListQuadlet, e QuadletEngine) ([]companionVolumeQuadlet, error) {
	companions := []companionVolumeQuadlet{}
	seen := map[string]bool{}
	for _, volume := range refs {
		file := transfer.QuadletVolumeCandidate(volume.Source)
		if file == "" || seen[file] {
			continue
		}
		if findQuadletByName(list, file) == nil {
			continue
		}
		printed, err := e.Print(file)
		if err != nil {
			// Listed but unprintable — the file vanished mid-lease. It
			// counts as dangling: the reference stays an ordinary entry
			// and the transfer carries nothing half-defined.
			if podmanerr.IsNotFound(err) {
				continue
			}
			return nil, fmt.Errorf("failed to get volume quadlet %s: %w", file, err)
		}
		seen[file] = true
		companions = append(companions, companionVolumeQuadlet{File: file, Content: printed})
	}
	return companions, nil
}

// installCompanionVolumeQuadlet installs one companion .volume quadlet on
// the target (issue 217), against the target list the lease already
// fetched. A file already present is compared byte-for-byte through the
// connection seam: byte-identical is SKIPPED (installs nothing, no error —
// also what makes re-running a failed set safe), differing content refuses
// the transfer with 409 QUADLET_ALREADY_EXISTS before anything further
// installs. An install failure is the plain error naming the failed file —
// companions already installed stay on the target, the source is untouched,
// and there is no rollback.
func installCompanionVolumeQuadlet(e QuadletEngine, companion companionVolumeQuadlet, targetList []*entities.ListQuadlet) error {
	if findQuadletByName(targetList, companion.File) != nil {
		targetContent, err := e.Print(companion.File)
		if err != nil {
			return fmt.Errorf("failed to read target volume quadlet %s: %w", companion.File, err)
		}
		if targetContent != companion.Content {
			return appErrors.ErrQuadletAlreadyExists.WithMessage(
				fmt.Sprintf("Volume quadlet %s already exists on the target with different content", companion.File))
		}
		return nil
	}

	report, err := e.Install(companion.File, []byte(companion.Content), false)
	if err != nil {
		if podmanerr.IsAlreadyExists(err) {
			return appErrors.ErrQuadletAlreadyExists
		}
		return fmt.Errorf("failed to install volume quadlet %s on target: %w", companion.File, err)
	}
	if report != nil {
		for _, qErr := range report.QuadletErrors {
			if qErr != nil && podmanerr.IsAlreadyExists(qErr) {
				return appErrors.ErrQuadletAlreadyExists
			}
		}
	}
	return nil
}
