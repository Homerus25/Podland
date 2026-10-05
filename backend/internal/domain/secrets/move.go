package secrets

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"path"
	"strings"

	appErrors "github.com/Homerus25/podland/backend/internal/errors"
	"github.com/Homerus25/podland/backend/internal/models"
	"github.com/Homerus25/podland/backend/internal/permission"
	"github.com/Homerus25/podland/backend/internal/podmanerr"
	"github.com/Homerus25/podland/backend/internal/transfer"
)

// Cross-machine secret copy/move (issue 239, architecture-decisions §19):
// the shared transfer kit owns the reveal→create ritual; this file owns
// the secrets domain's composition around it — the preview's facts
// (collision, referencing quadlets), the transfer's install-first-then-
// remove order, and the mapping onto the wire vocabulary. The name is
// preserved on both machines; there is no rename and no overwrite. The
// value travels only through the reveal path and never reaches a log
// statement or a response body (the redaction discipline).

// MovePreview answers everything the copy/move dialog must show before
// confirming (issue 239): whether the target machine already holds a
// same-named secret, and which source quadlets reference the secret — a
// warning in move mode, never a block. See
// doku/description_secrets_api.md.
//
// Access: the route's subtree middleware already proved the caller holds
// an access row on the source machine — read level suffices, this
// endpoint mutates nothing and reveals nothing. The target machine is a
// service-side gate (route middleware only sees the route's machine): the
// caller's stored level must be at least read, checked against the
// access store BEFORE any podman call, so a caller without target access
// gets the standard 403 ACCESS_DENIED and never a false preview.
func (s *SecretService) MovePreview(ctx context.Context, userID, machineID, secretID, targetMachineID string) (*SecretMovePreviewResponse, error) {
	// The target gate fires before the connection lease (the quadlet
	// move-preview's shape): a lookup failure is 500, a level below read
	// — an absent row included — is 403, per permission.RequireLevel's
	// single-owner contract.
	if err := permission.RequireLevel(s.accessRepo, targetMachineID, userID, models.PermissionRead); err != nil {
		return nil, err
	}

	// Source lease: the secret must exist (404) before any scan. The
	// transfer preserves the NAME, so the preview resolves it once here
	// and every fact below reads the name.
	name, err := s.resolveSecretName(ctx, userID, machineID, secretID)
	if err != nil {
		return nil, err
	}

	// Usage lease: the referencing-quadlet scan — the source machine's
	// quadlet files whose `Secret=` lines resolve to the secret, read
	// through the usage seam.
	var referencing []SecretMoveQuadletRef
	err = s.usageEngines.WithEngine(ctx, userID, machineID, "secrets.MovePreviewUsage", func(e SecretUsageEngine) error {
		referencing, err = scanReferencingQuadlets(e, name)
		return err
	})
	if err != nil {
		return nil, err
	}

	// Target lease: the collision check reads the target's secret list
	// by name. The transfer re-checks at execution time; this flag only
	// drives the dialog's confirm gate.
	var collision bool
	err = s.engines.WithEngine(ctx, userID, targetMachineID, "secrets.MovePreviewTarget", func(e SecretEngine) error {
		onTarget, err := secretNames(e)
		if err != nil {
			return err
		}
		collision = onTarget[name]
		return nil
	})
	if err != nil {
		return nil, err
	}

	return &SecretMovePreviewResponse{
		Collision:           collision,
		ReferencingQuadlets: referencing,
	}, nil
}

// Move transfers a secret to another machine (issue 239, §19).
//
// Copy mode reveals the value on the source and creates the secret on
// the target, never touching the source. Move mode additionally removes
// the source secret, but ONLY after the target install succeeded
// (install-first-then-remove): a failed source removal answers the
// transfer envelope with status "partial" plus the failure's detail in
// Error — the secret then exists on both machines and the source side
// must be cleaned up manually. A source secret that vanished between
// the transfer and the removal needs no removal — the answer is "moved".
//
// The collision is re-checked at execution time inside the transfer: a
// target secret that appeared since the preview answers 409
// SECRET_ALREADY_EXISTS, never an overwrite (no WithReplace).
//
// Access: the route's write-level middleware proved the caller may
// mutate the SOURCE machine, but the value LEAVES it — a reveal — so the
// service gates the source at ADMIN (the reveal gate's own level, fired
// before any connection lease: a below-admin caller gets 403
// ACCESS_DENIED, not a redacted or partial transfer, and podman is never
// asked for the value). The TARGET machine is a service-side gate: the
// caller's stored level must be at least write, checked before any
// podman call. The operation is therefore effectively admin-on-source.
//
// The answer is the plain JSON envelope — secret values are small, the
// NDJSON transport stays a volume concern.
func (s *SecretService) Move(ctx context.Context, userID, machineID, secretID string, req SecretMoveRequest) (*SecretMoveResponse, error) {
	// The source admin gate (the reveal) fires before any connection
	// lease, mirroring Get's reveal gate: a below-admin caller is
	// refused outright and podman is never asked for the value.
	if err := permission.RequireLevel(s.accessRepo, machineID, userID, models.PermissionAdmin); err != nil {
		return nil, err
	}
	// The target write gate (the quadlet move's shape): a lookup failure
	// is 500, a level below write — an absent row included — is 403,
	// per permission.RequireLevel's single-owner contract.
	if err := permission.RequireLevel(s.accessRepo, req.TargetMachineID, userID, models.PermissionWrite); err != nil {
		return nil, err
	}

	// Source lease: resolve the secret's name (the transfer preserves
	// it); a missing secret is the clean 404.
	name, err := s.resolveSecretName(ctx, userID, machineID, secretID)
	if err != nil {
		return nil, err
	}

	// Transfer phase (§19): two connection leases stand open at once —
	// the reveal reads the source machine's store while the create
	// writes the target's — so the nested WithEngine is deliberate, the
	// quadlet secret phase's arrangement. The kit owns the reveal→create
	// loop and its same-name skip; the skip IS this endpoint's
	// execution-time collision re-check (the kit checks the target
	// before it reveals anything), so a same-name target answers 409
	// here, never an overwrite.
	err = s.engines.WithEngine(ctx, userID, machineID, "secrets.MoveTransferSource", func(src SecretEngine) error {
		return s.engines.WithEngine(ctx, userID, req.TargetMachineID, "secrets.MoveTransferTarget", func(tgt SecretEngine) error {
			outcomes, err := transfer.TransferSecrets(
				secretSourceAdapter{e: src}, secretTargetAdapter{e: tgt},
				[]transfer.SecretRef{{Name: name}})
			if err != nil {
				var secretErr *transfer.SecretError
				if errors.As(err, &secretErr) {
					return appErrors.ErrSecretTransferFailed.WithMessage(
						fmt.Sprintf("Secret %s: %v", secretErr.Name, secretErr.Err))
				}
				return err
			}
			if len(outcomes) != 1 {
				// One reference in, one outcome out — anything else is a
				// kit contract breach; the generic 500 is the honest
				// answer.
				return fmt.Errorf("secret transfer reported %d outcomes for one reference", len(outcomes))
			}
			outcome := outcomes[0]
			if outcome.Outcome != transfer.SecretOutcomeTransferred {
				if outcome.Reason == transfer.SecretSkipOnTarget {
					return appErrors.ErrSecretAlreadyExists
				}
				// The only other skip is the dangling one: the source
				// secret vanished between the resolve and the reveal.
				return appErrors.ErrSecretNotFound
			}
			// Audit per the redaction discipline: who moved what where,
			// never the value itself (the reveal log's shape).
			slog.InfoContext(ctx, "secret transferred",
				slog.String("userID", userID),
				slog.String("sourceMachineID", machineID),
				slog.String("targetMachineID", req.TargetMachineID),
				slog.String("secretName", name))
			return nil
		})
	})
	if err != nil {
		return nil, err
	}

	response := &SecretMoveResponse{
		Name:            name,
		Status:          SecretMoveStatusCopied,
		SourceMachineID: machineID,
		TargetMachineID: req.TargetMachineID,
	}
	if req.Mode == SecretMoveModeMove {
		response.Status = SecretMoveStatusMoved
	}

	// Move mode's source half, only now that the target holds the
	// secret. A removal failure does not fail the request: the secret
	// exists on both machines, and the envelope says so with status
	// "partial" plus the failure's detail.
	if req.Mode == SecretMoveModeMove {
		if err := s.removeSourceAfterTransfer(ctx, userID, machineID, name); err != nil {
			response.Status = SecretMoveStatusPartial
			response.Error = err.Error()
		}
	}
	return response, nil
}

// resolveSecretName resolves the path segment (a secret's name or ID) to
// the secret's NAME — the identity the transfer preserves — through the
// engine seam, mapping a missing secret to the clean 404 sentinel.
func (s *SecretService) resolveSecretName(ctx context.Context, userID, machineID, secretID string) (string, error) {
	var name string
	err := s.engines.WithEngine(ctx, userID, machineID, "secrets.ResolveSecretName", func(e SecretEngine) error {
		report, err := e.Inspect(secretID, false)
		if err != nil {
			if podmanerr.IsNotFound(err) {
				return appErrors.ErrSecretNotFound
			}
			return fmt.Errorf("failed to inspect secret: %w", err)
		}
		if report == nil {
			return appErrors.ErrSecretNotFound
		}
		name = report.Spec.Name
		return nil
	})
	return name, err
}

// removeSourceAfterTransfer performs the move mode's source-side half
// after the target holds the secret: the delete flow's removal mapping.
// A vanished source secret answers nil — the transfer's goal is met and
// a "partial" would falsely claim the secret exists on both machines.
// Its error is the "partial" detail, never a request failure — the
// target install it follows cannot be rolled back.
func (s *SecretService) removeSourceAfterTransfer(ctx context.Context, userID, machineID, secretName string) error {
	return s.engines.WithEngine(ctx, userID, machineID, "secrets.MoveSourceRemove", func(e SecretEngine) error {
		if err := e.Remove(secretName); err != nil {
			if podmanerr.IsNotFound(err) {
				// The source vanished between the transfer and the
				// removal — the transfer's goal is met.
				return nil
			}
			if podmanerr.IsConflict(err) {
				// Podman's own refusal (which container holds the
				// secret) is the honest partial detail.
				return appErrors.ErrSecretInUse.WithMessage(podmanerr.Message(err))
			}
			return fmt.Errorf("failed to delete secret: %w", err)
		}
		return nil
	})
}

// secretNames lists the machine's secret names as a set — the preview's
// collision read.
func secretNames(e SecretEngine) (map[string]bool, error) {
	reports, err := e.List()
	if err != nil {
		return nil, fmt.Errorf("failed to list secrets: %w", err)
	}
	names := make(map[string]bool, len(reports))
	for _, report := range reports {
		if report != nil {
			names[report.Spec.Name] = true
		}
	}
	return names, nil
}

// scanReferencingQuadlets lists the machine's quadlet files whose
// `Secret=` lines resolve to the secret (issue 239) — the preview's
// warning, never a block. Every quadlet is printed once; a listed but
// unprintable file (it vanished mid-lease) cannot be scanned and is
// skipped rather than failing the preview. The `Secret=` parse is the
// quadlets domain's ritual, mirrored here — domains never import each
// other, and the extraction happens at the third copy.
func scanReferencingQuadlets(e SecretUsageEngine, secretName string) ([]SecretMoveQuadletRef, error) {
	names, err := e.ListQuadletNames()
	if err != nil {
		return nil, fmt.Errorf("failed to list quadlets: %w", err)
	}
	contents := make(map[string]string, len(names))
	for _, name := range names {
		content, err := e.PrintQuadlet(name)
		if err != nil {
			continue
		}
		contents[name] = content
	}

	referencing := []SecretMoveQuadletRef{}
	for _, name := range names {
		content, ok := contents[name]
		if !ok {
			continue
		}
		for _, reference := range quadletSecretReferences(content) {
			if reference == secretName {
				referencing = append(referencing, SecretMoveQuadletRef{Name: name})
				break
			}
		}
	}
	return referencing, nil
}

// quadletSecretReferences extracts every `Secret=` line's resolved secret
// name from quadlet file content: the token before the first comma,
// unless it carries `=` — the all-options form, where the `source=`
// option governs (a token without `source=` names nothing). An
// absolute-path source is a hostPath reference — podman reads it off the
// host filesystem at run time, so it never names a store secret and never
// matches. Comment lines are skipped and key matching is
// case-insensitive, matching systemd's unit-file parser — the quadlets
// domain's ritual, mirrored (the second copy; extraction at the third).
func quadletSecretReferences(content string) []string {
	references := []string{}
	for line := range strings.Lines(content) {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}
		key, value, found := strings.Cut(line, "=")
		if !found || !strings.EqualFold(strings.TrimSpace(key), "Secret") {
			continue
		}
		if name := secretReferenceName(strings.TrimSpace(value)); name != "" {
			references = append(references, name)
		}
	}
	return references
}

// secretReferenceName applies the issue-215 naming rule to one `Secret=`
// value: the token before the first comma is the name, unless it carries
// `=` — the all-options form, where the `source=` option governs. A
// resolved name starting with `/` is a hostPath reference and names
// nothing here.
func secretReferenceName(value string) string {
	token := value
	if idx := strings.Index(value, ","); idx >= 0 {
		token = value[:idx]
	}
	if optKey, optValue, found := strings.Cut(token, "="); found {
		if !strings.EqualFold(strings.TrimSpace(optKey), "source") {
			return ""
		}
		token = strings.TrimSpace(optValue)
	}
	if token == "" || path.IsAbs(token) {
		return ""
	}
	return token
}

// secretSourceAdapter widens the secrets engine's Inspect to the shared
// transfer kit's source width (§19): the reveal is the admin-gated
// Inspect with the value included.
type secretSourceAdapter struct{ e SecretEngine }

func (a secretSourceAdapter) Reveal(name string) (string, error) {
	report, err := a.e.Inspect(name, true)
	if err != nil {
		return "", err
	}
	if report == nil {
		return "", appErrors.ErrSecretNotFound
	}
	return report.SecretData, nil
}

// secretTargetAdapter widens the secrets engine's Inspect and Create to
// the shared transfer kit's target width (§19): the same-name check is
// the unrevealed Inspect — a missing secret answers a clean false, never
// an error.
type secretTargetAdapter struct{ e SecretEngine }

func (a secretTargetAdapter) Has(name string) (bool, error) {
	report, err := a.e.Inspect(name, false)
	if err != nil {
		if podmanerr.IsNotFound(err) {
			return false, nil
		}
		return false, err
	}
	return report != nil, nil
}

func (a secretTargetAdapter) Create(name, value string) error {
	_, err := a.e.Create(name, value)
	return err
}
