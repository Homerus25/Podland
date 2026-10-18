// Package inspect owns the single-report inspect resolution ritual
// shared by the slice-report domains' inspect endpoints (containers,
// networks, images). The podman tunnel answers an inspect call as a
// report slice plus a per-item error slice plus a transport error; each
// domain resolves that triple into one report, classifying podman's
// not-found shapes into the domain's own sentinel. The module imports
// no domain vocabulary — the sentinel and the domain label are passed
// in — so the classification matrix is pinned once here and the domains
// keep their composition pins.
package inspect

import (
	"fmt"

	"github.com/Homerus25/podland/backend/internal/podmanerr"
)

// Resolve resolves the (reports, per-item errors, transport error)
// triple into one report. A transport-level or per-item not-found
// classifies into notFound; any other failure wraps with the domain's
// label; zero reports with per-item errors surfaces the first cause,
// and zero reports without errors is an anomaly naming the item.
func Resolve[T any](reports []T, inspectErrs []error, err error, notFound error, domain, id string) (T, error) {
	var zero T
	if err != nil {
		if podmanerr.IsNotFound(err) {
			return zero, notFound
		}
		return zero, fmt.Errorf("failed to inspect %s: %w", domain, err)
	}

	// The tunnel's not-found answer for inspect arrives as a
	// per-item error with zero reports.
	for _, inspectErr := range inspectErrs {
		if podmanerr.IsNotFound(inspectErr) {
			return zero, notFound
		}
	}

	if len(reports) == 0 {
		// Zero reports with per-item errors means the inspect failed
		// for a real cause the not-found loop above did not classify;
		// surface it instead of dropping it behind the anomaly message.
		if len(inspectErrs) > 0 {
			return zero, fmt.Errorf("failed to inspect %s: %w", domain, inspectErrs[0])
		}
		return zero, fmt.Errorf("no inspect report returned for %s: %s", domain, id)
	}

	return reports[0], nil
}
