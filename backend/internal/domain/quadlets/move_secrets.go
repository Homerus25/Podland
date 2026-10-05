package quadlets

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	appErrors "github.com/Homerus25/podland/backend/internal/errors"
	"github.com/Homerus25/podland/backend/internal/transfer"
)

// Secret transfer for the move/copy transfer (issue 215). The phase sits
// in MoveWithProgress between a migrating move's stop and the volume data
// phase; this file owns the pieces the phase is built from: the failure
// mapping onto the quadlet move's wire vocabulary and the call into the
// shared transfer kit (§19), which owns the reveal→create loop and its
// skip rules, mirroring move_volume_data.go's placement.

// secretTransferFailed wraps a secret-phase engine failure as 500
// SECRET_TRANSFER_FAILED, the message naming the secret and the cause
// (the VOLUME_MIGRATION_FAILED shape, issue 215).
func secretTransferFailed(name string, err error) error {
	return appErrors.ErrSecretTransferFailed.WithMessage(
		fmt.Sprintf("Secret %s: %v", name, err))
}

// transferSecrets runs the secrets phase (issue 215, §19): one outcome per
// `Secret=` reference in the content, in file order, deduplicated by
// resolved name (first reference wins). Two connection leases stand open
// at once — the reveal reads the source machine's store while the create
// writes the target's — so the nested WithEngine is deliberate, the same
// arrangement the volume data phase uses.
//
// The skip rules and the reveal→create loop are the shared transfer kit's
// (§19): a hostPath source is skipped and reported (podman reads it off
// the host filesystem, the store can never resolve it); a same-name secret
// already on the target is skipped BEFORE the reveal — the skip keeps
// re-runs safe and touches no existing target state, and the value is
// never read when nothing needs it; a reference absent from the source
// store (a dangling one) is skipped and reported, not an error; everything
// else is revealed on the source, created on the target, and audited
// value-free — one "secret transferred" event per transferred secret (the
// redaction discipline, the reveal log's shape). Secrets are copy-only:
// nothing here ever removes from the source.
//
// Any other failure fails the whole phase: nothing has been installed
// yet, the caller returns the error and the transfer stays a no-op on
// both machines' quadlet sets. Secrets delivered earlier in the phase
// stay on the target — inert, skipped on re-run by the same-name rule.
func (s *QuadletService) transferSecrets(ctx context.Context, userID, sourceMachineID, targetMachineID, content string) ([]QuadletMoveSecretOutcome, error) {
	outcomes := []QuadletMoveSecretOutcome{}
	err := s.secretEngines.WithEngine(ctx, userID, sourceMachineID, "quadlets.MoveSecretsSource", func(src moveSecretEngine) error {
		return s.secretEngines.WithEngine(ctx, userID, targetMachineID, "quadlets.MoveSecretsTarget", func(tgt moveSecretEngine) error {
			refs := make([]transfer.SecretRef, 0)
			for _, entry := range parseQuadletSecrets(content) {
				refs = append(refs, transfer.SecretRef{
					Name:     entry.Name,
					HostPath: entry.Kind == MovePreviewSecretKindHostPath,
				})
			}

			transferred, err := transfer.TransferSecrets(src, tgt, refs)
			// The audit rides the outcomes the kit reports, BEFORE the
			// error mapping: a mid-phase failure must not lose the trail
			// of secrets already delivered to the target.
			for _, outcome := range transferred {
				outcomes = append(outcomes, QuadletMoveSecretOutcome{
					Name:    outcome.Name,
					Outcome: outcome.Outcome,
					Reason:  outcome.Reason,
				})
				if outcome.Outcome != transfer.SecretOutcomeTransferred {
					continue
				}
				// Audit per the redaction discipline: who moved what
				// where, never the value itself (the reveal log's shape).
				slog.InfoContext(ctx, "secret transferred",
					slog.String("userID", userID),
					slog.String("sourceMachineID", sourceMachineID),
					slog.String("targetMachineID", targetMachineID),
					slog.String("secretName", outcome.Name))
			}
			if err != nil {
				var secretErr *transfer.SecretError
				if errors.As(err, &secretErr) {
					return secretTransferFailed(secretErr.Name, secretErr.Err)
				}
				return err
			}
			return nil
		})
	})
	if err != nil {
		return nil, err
	}
	return outcomes, nil
}
