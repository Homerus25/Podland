package transfer

import (
	"fmt"

	"github.com/Homerus25/podland/backend/internal/podmanerr"
)

// SecretSource is the source host's secret store width: the value
// reveal. A missing secret answers a podman not-found error the kit
// classifies through the podmanerr adapter.
type SecretSource interface {
	Reveal(name string) (string, error)
}

// SecretTarget is the target host's secret store width: the same-name
// membership check (the skip) and the create. A membership predicate, not
// a name list — the kit asks per reference, so the adapter answers from
// whatever read its store serves cheapest and no name set crosses the
// seam only to be re-hashed.
type SecretTarget interface {
	Has(name string) (bool, error)
	Create(name, value string) error
}

// SecretRef is one reference the secret transfer carries (§19): the
// resolved name and whether the source is a host path — podman reads a
// host-path reference off the host filesystem at run time, so no store
// can ever resolve it and the transfer skips it.
type SecretRef struct {
	Name     string
	HostPath bool
}

// Outcome labels for SecretOutcome.Outcome (§19): "transferred" — the
// value was revealed on the source and the secret created on the target;
// "skipped" — the reference carried nothing, with Reason saying why.
const (
	SecretOutcomeTransferred = "transferred"
	SecretOutcomeSkipped     = "skipped"
)

// Skip reasons (§19): the honest label a skipped reference's outcome
// carries. A same-name skip keeps re-runs safe while touching no
// existing target state; values are never compared, so a skip never
// means "equal".
const (
	SecretSkipHostPath = "host path references are not transferred"
	SecretSkipDangling = "source secret does not exist"
	SecretSkipOnTarget = "a secret with this name already exists on the target"
)

// SecretError names the secret a transfer failure involves (§19). Its
// message keeps the shape the quadlet move's wire contract already
// speaks ("Secret <name>: <cause>"), so the caller maps it onto its own
// sentinel with the message intact.
type SecretError struct {
	Name string
	Err  error
}

func (e *SecretError) Error() string {
	return fmt.Sprintf("Secret %s: %v", e.Name, e.Err)
}

func (e *SecretError) Unwrap() error {
	return e.Err
}

// TransferSecrets carries each referenced secret's value from the source
// host's store into the target's (§19): one outcome per reference, in
// the order the caller listed them, deduplicated by name (first
// reference wins — the caller deduplicates or not by the list it hands
// over).
//
// Per reference: a host-path source is skipped and reported (podman
// reads it off the host filesystem, the store can never resolve it); a
// same-name secret already on the target is skipped BEFORE the reveal —
// the skip keeps re-runs safe and touches no existing target state, and
// the value is never read when nothing needs it; a reference absent from
// the source store (a dangling one) is skipped and reported, not an
// error; everything else is revealed on the source and created on the
// target. Secrets are copy-only: nothing here ever removes from the
// source.
//
// Any other failure fails the whole phase and comes back as the plain
// cause (a target membership-check failure) or a *SecretError naming the
// secret (a reveal or create failure) — the caller maps both onto its own
// wire codes. The outcomes gathered before the failure ride back alongside
// the error, so a caller that audits per secret keeps its trail when
// the phase dies mid-way. Secrets delivered before the failure stay on
// the target — inert, skipped on re-run by the same-name rule.
func TransferSecrets(src SecretSource, tgt SecretTarget, refs []SecretRef) ([]SecretOutcome, error) {
	outcomes := []SecretOutcome{}
	seen := map[string]bool{}
	for _, ref := range refs {
		if seen[ref.Name] {
			continue
		}
		seen[ref.Name] = true

		if ref.HostPath {
			outcomes = append(outcomes, SecretOutcome{
				Name:    ref.Name,
				Outcome: SecretOutcomeSkipped,
				Reason:  SecretSkipHostPath,
			})
			continue
		}

		onTarget, err := tgt.Has(ref.Name)
		if err != nil {
			return outcomes, fmt.Errorf("failed to check target secret %s: %w", ref.Name, err)
		}
		if onTarget {
			outcomes = append(outcomes, SecretOutcome{
				Name:    ref.Name,
				Outcome: SecretOutcomeSkipped,
				Reason:  SecretSkipOnTarget,
			})
			continue
		}

		value, err := src.Reveal(ref.Name)
		if err != nil {
			if podmanerr.IsNotFound(err) {
				outcomes = append(outcomes, SecretOutcome{
					Name:    ref.Name,
					Outcome: SecretOutcomeSkipped,
					Reason:  SecretSkipDangling,
				})
				continue
			}
			return outcomes, &SecretError{Name: ref.Name, Err: err}
		}

		if err := tgt.Create(ref.Name, value); err != nil {
			return outcomes, &SecretError{Name: ref.Name, Err: err}
		}
		outcomes = append(outcomes, SecretOutcome{
			Name:    ref.Name,
			Outcome: SecretOutcomeTransferred,
		})
	}
	return outcomes, nil
}

// SecretOutcome is one reference's transfer result (§19): the resolved
// name, the outcome label, and — for a skip — the honest reason.
type SecretOutcome struct {
	Name    string
	Outcome string
	Reason  string
}
