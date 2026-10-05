// Package transfer is the one transfer kit behind every cross-machine
// copy/move (architecture-decisions §19): the machine-to-machine volume
// data splice (podman export on the source piped into podman import on
// the target), the secret value transfer (reveal on the source, create on
// the target), and the quadlet reference resolution (the `Volume=` and
// `Secret=` line parse and the source→name rules). The quadlet move/copy
// (issues 214 and 215), the volume copy/move (issue 238), and the secret
// copy/move (issue 239) all ride it; handlers, DTOs, and preview
// projections stay per-domain. The reference seam (issue 242) exists so
// the two move previews can never parse a quadlet differently from the
// quadlet move itself — a preview that lied about references would lie
// about what a move breaks.
//
// The kit is engine-agnostic by design: callers hand it the two
// machines' engines through the narrow interfaces below (Go structural
// typing means a domain engine whose methods already match satisfies
// them with no adapter), and the kit owns the rituals every transfer
// shares — the in-process pipe with its cancellation wiring, the
// counting reader that enforces the byte bound and reports progress,
// the bounded export-tar probe, the secret skip rules that keep
// re-runs safe, and the quadlet reference parse. Error mapping stays
// with the caller: the kit returns engine errors unwrapped (plus its
// own sentinels, ErrBoundExceeded and *SecretError), and each domain
// maps them onto its own wire codes.
package transfer
