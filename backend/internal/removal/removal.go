// Package removal owns the remove-and-classify ritual behind every
// domain's Delete and move source half (issue 264): call the remove,
// classify the outcome, answer the caller's terminal. The module is
// transport-agnostic — whether the classified error is a request
// failure or a transfer's "partial" detail stays at the call sites —
// and imports no domain vocabulary: the terminals are the caller's,
// passed in (GLOSSARY: "Removal terminals").
package removal

import (
	"fmt"

	"github.com/Homerus25/podland/backend/internal/podmanerr"
)

// Terminals carries the caller's removal terminals: the domain-owned
// vocabulary of what a removal's two classified outcomes MEAN.
type Terminals struct {
	// Missing answers a vanished source (podman's not-found). nil is
	// legal: every move source half answers nil for a vanished source
	// because the transfer's goal is met.
	Missing error
	// Refusal builds the refusal terminal, threading podman's refusal
	// text where the domain's wire shows it; a caller whose wire does
	// not show the text passes a dropping closure.
	Refusal func(cause string) error
	// IsRefusal reports whether err is the domain's refusal. The
	// domain's refusal phrases stay domain-local, passed in, never
	// named by the module.
	IsRefusal func(err error) bool
}

// Run calls the remove closure and classifies its outcome: a clean
// removal answers nil; podman's not-found answers terminals.Missing
// (nil-legal); a refusal (terminals.IsRefusal) answers
// terminals.Refusal with podman's refusal text threaded; anything else
// is wrapped with wrapText.
func Run(remove func() error, terminals Terminals, wrapText string) error {
	err := remove()
	if err == nil {
		return nil
	}
	if podmanerr.IsNotFound(err) {
		return terminals.Missing
	}
	if terminals.IsRefusal(err) {
		return terminals.Refusal(podmanerr.Message(err))
	}
	return fmt.Errorf("%s: %w", wrapText, err)
}
