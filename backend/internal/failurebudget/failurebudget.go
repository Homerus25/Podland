// Package failurebudget owns the credential-failure budget's
// check/spend/advertise contract (issue 236): the policy concept the
// auth and users credential surfaces share, extracted from two
// verbatim copies whose wiring had already diverged. The middleware
// fixed-window limiter (issue 185) stays the engine underneath — it
// satisfies the Brake seam unchanged — while this module owns
// everything above it: the endpoint-namespaced key, the nil-unwired
// semantics, and the 429 classification behind the X-RateLimit
// advertisement.
package failurebudget

import (
	"errors"
	"net/http"
	"time"

	apperrors "github.com/Homerus25/podland/backend/internal/errors"
	"github.com/Homerus25/podland/backend/internal/web"
)

// Brake is the engine seam: the keyed fixed-window budget the Surface
// reads, spends, and advertises. *middleware.RateLimiter satisfies it
// unchanged (HasBudget is the pre-verification read, Record the
// spend-on-failure, Limit and GetResetIn the advertisement's numbers),
// and function-field fakes stay possible for the domains' tests. The
// module never imports the middleware package — internal/app constructs
// the limiter and hands it in.
type Brake interface {
	HasBudget(key string) bool
	Record(key string)
	Limit() int
	GetResetIn(key string) time.Duration
}

// Surface is one credential endpoint's failure budget: the deep module
// behind a credential-verifying endpoint's brake. Constructed per
// endpoint via NewSurface, it owns the key — the surface's name
// prefixed onto the caller-supplied id (name + ":" + id) — so neither
// the service nor the handler ever touches a raw key and a username and
// a user ID cannot collide even if one is chosen to look like the other
// (issue 196 review). The surface-name string lives at the wiring site
// (internal/app, or the domain tests' own construction): the module
// owns the namespacing rule, not the endpoint vocabulary, so a typo'd
// name can only create a distinct key space, never corrupt another
// surface's.
//
// A nil *Surface is the unwired state: Check reports budget, Spend and
// Advertise are no-ops. The domains' former nil-guard pair collapsed
// into these methods, so "the brake is unwired" needs no per-domain
// handling.
type Surface struct {
	name  string
	brake Brake
}

// NewSurface builds the named endpoint's budget over the given brake.
// One Surface per endpoint, one brake instance per surface: separate
// surfaces never share key spaces, so their spending cannot interact.
func NewSurface(name string, brake Brake) *Surface {
	return &Surface{name: name, brake: brake}
}

// key derives the brake key for id: the surface's name as a prefix.
func (s *Surface) key(id string) string {
	return s.name + ":" + id
}

// Check reports whether id may still attempt a credential
// verification. A nil Surface (brake unwired) always reports budget.
func (s *Surface) Check(id string) bool {
	if s == nil {
		return true
	}
	return s.brake.HasBudget(s.key(id))
}

// Spend spends one unit of id's failure budget after a failed
// credential verification. A nil Surface (brake unwired) records
// nothing.
func (s *Surface) Spend(id string) {
	if s == nil {
		return
	}
	s.brake.Record(s.key(id))
}

// Advertise writes the X-RateLimit-* trio — the configured limit, zero
// remaining, and the seconds until id's window resets — when err is
// the budget's own 429: an *apperrors.AppError with Status 429, the
// verdict Check's failure produces upstream (the domain services still
// return their own ErrRateLimitExceeded sentinel; this classification
// is the budget's, not domain error mapping). Any other failure — a
// non-AppError, a non-429 AppError, a nil error — and an unwired
// Surface write nothing. Headers only: the status line and body stay
// with the envelope.
func (s *Surface) Advertise(w http.ResponseWriter, id string, err error) {
	if s == nil {
		return
	}
	appErr, ok := errors.AsType[*apperrors.AppError](err)
	if !ok || appErr.Status != http.StatusTooManyRequests {
		return
	}
	web.SetRateLimitHeaders(w, s.brake.Limit(), 0, s.brake.GetResetIn(s.key(id)))
}
