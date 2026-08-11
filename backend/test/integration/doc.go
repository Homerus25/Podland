//go:build integration

// Package integration contains Podland's end-to-end integration test
// suite. Every file in this package carries the `//go:build integration`
// build constraint, so the suite is excluded from `go test ./...` and
// `make test` by default. Run it explicitly via:
//
//	make test-integration
//
// which expands to `go test -tags=integration ./test/integration/...`.
//
// The suite brings the backend up in-process (httptest.NewServer wrapping
// internal/app.App.Handler) and exercises the full pipeline — HTTP → JWT
// → machine registration → SSH tunnel → podman socket → HTTP — against a
// real podman machine. TestMain owns the shared fixture (Suite, exposed
// via the package-level `suite` variable) and downstream slices
// (080-02 images, 080-03 containers, …) consume its helpers.
package integration
