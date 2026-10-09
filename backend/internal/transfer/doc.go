// Package transfer is the one transfer kit behind every cross-machine
// copy/move (architecture-decisions §19): the machine-to-machine volume
// data splice (podman export on the source piped into podman import on
// the target), the secret value transfer (reveal on the source, create on
// the target), the quadlet reference resolution (the `Volume=` and
// `Secret=` line parse and the source→name rules), and the machine-wide
// quadlet scan (issue 259: the list → print-each → match ritual that
// finds the quadlets referencing a volume or secret). The quadlet move/copy
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
// re-runs safe, the quadlet reference parse, and the machine-wide
// quadlet scan (issue 259, §19). Error mapping stays
// with the caller: the kit returns engine errors unwrapped (plus its
// own sentinels, ErrBoundExceeded and *SecretError, and the quadlet
// scan's list-failure wrap — issue 259's kit-owned wrap, the one wrap
// the kit mints), and each domain maps them onto its own wire codes.
package transfer
