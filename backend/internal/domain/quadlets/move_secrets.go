package quadlets

import (
	"context"
	"fmt"
	"log/slog"

	appErrors "github.com/Homerus25/podland/backend/internal/errors"
	"github.com/Homerus25/podland/backend/internal/podmanerr"
)

// Secret transfer for the move/copy transfer (issue 215). The phase sits
// in MoveWithProgress between a migrating move's stop and the volume data
// phase; this file owns the pieces the phase is built from: the failure
// mapping and the machine-to-machine transfer itself (reveal on the
// source, create on the target), mirroring move_volume_data.go's
// placement.

// secretTransferFailed wraps a secret-phase engine failure as 500
// SECRET_TRANSFER_FAILED, the message naming the secret and the cause
// (the VOLUME_MIGRATION_FAILED shape, issue 215).
func secretTransferFailed(name string, err error) error {
	return appErrors.ErrSecretTransferFailed.WithMessage(
		fmt.Sprintf("Secret %s: %v", name, err))
}

// transferSecrets runs the secrets phase (issue 215): one outcome per
// `Secret=` reference in the content, in file order, deduplicated by
// resolved name (first reference wins). Two connection leases stand open
// at once — the reveal reads the source machine's store while the create
// writes the target's — so the nested WithEngine is deliberate, the same
// arrangement the volume data phase uses.
//
// Per reference: a hostPath source is skipped and reported (podman reads
// it off the host filesystem, the store can never resolve it); a
// same-name secret already on the target is skipped BEFORE the reveal —
// the skip keeps re-runs safe and touches no existing target state, and
// the value is never read when nothing needs it; a reference absent from
// the source store (a dangling one) is skipped and reported, not an
// error; everything else is revealed on the source, created on the
// target, and audited value-free — one "secret transferred" event per
// transferred secret (the redaction discipline, the reveal log's shape).
// Secrets are copy-only: nothing here ever removes from the source.
//
// Any other failure fails the whole phase: nothing has been installed
// yet, the caller returns the error and the transfer stays a no-op on
// both machines' quadlet sets. Secrets delivered earlier in the phase
// stay on the target — inert, skipped on re-run by the same-name rule.
func (s *QuadletService) transferSecrets(ctx context.Context, userID, sourceMachineID, targetMachineID, content string) ([]QuadletMoveSecretOutcome, error) {
	outcomes := []QuadletMoveSecretOutcome{}
	err := s.secretEngines.WithEngine(ctx, userID, sourceMachineID, "quadlets.MoveSecretsSource", func(src moveSecretEngine) error {
		return s.secretEngines.WithEngine(ctx, userID, targetMachineID, "quadlets.MoveSecretsTarget", func(tgt moveSecretEngine) error {
			targetNames, err := tgt.Names()
			if err != nil {
				return fmt.Errorf("failed to list target secrets: %w", err)
			}
			onTarget := make(map[string]bool, len(targetNames))
			for _, name := range targetNames {
				onTarget[name] = true
			}

			seen := map[string]bool{}
			for _, entry := range parseQuadletSecrets(content) {
				if seen[entry.Name] {
					continue
				}
				seen[entry.Name] = true

				if entry.Kind == MovePreviewSecretKindHostPath {
					outcomes = append(outcomes, QuadletMoveSecretOutcome{
						Name:    entry.Name,
						Outcome: MoveSecretOutcomeSkipped,
						Reason:  MoveSecretSkipHostPath,
					})
					continue
				}

				if onTarget[entry.Name] {
					outcomes = append(outcomes, QuadletMoveSecretOutcome{
						Name:    entry.Name,
						Outcome: MoveSecretOutcomeSkipped,
						Reason:  MoveSecretSkipOnTarget,
					})
					continue
				}

				value, err := src.Reveal(entry.Name)
				if err != nil {
					if podmanerr.IsNotFound(err) {
						outcomes = append(outcomes, QuadletMoveSecretOutcome{
							Name:    entry.Name,
							Outcome: MoveSecretOutcomeSkipped,
							Reason:  MoveSecretSkipDangling,
						})
						continue
					}
					return secretTransferFailed(entry.Name, err)
				}

				if err := tgt.Create(entry.Name, value); err != nil {
					return secretTransferFailed(entry.Name, err)
				}

				// Audit per the redaction discipline: who moved what
				// where, never the value itself (the reveal log's shape).
				slog.InfoContext(ctx, "secret transferred",
					slog.String("userID", userID),
					slog.String("sourceMachineID", sourceMachineID),
					slog.String("targetMachineID", targetMachineID),
					slog.String("secretName", entry.Name))

				outcomes = append(outcomes, QuadletMoveSecretOutcome{
					Name:    entry.Name,
					Outcome: MoveSecretOutcomeTransferred,
				})
			}
			return nil
		})
	})
	if err != nil {
		return nil, err
	}
	return outcomes, nil
}
