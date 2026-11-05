package testutils

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"

	"github.com/Homerus25/podland/backend/internal/contextutils"
)

// Request describes one handler invocation for Perform: the identity
// the subtree middleware would have validated, the URL params the
// router would have routed, and an optional body.
type Request struct {
	Method string

	// Path is used for the httptest request; URL params come from
	// URLParams, mirroring how the ServeMux populates them (the path
	// string itself is not parsed).
	Path string

	// UserID is the authenticated caller the subtree middleware would
	// have validated. No default: set it explicitly ("test-user" in
	// most handler tests); leave empty only when deliberately testing
	// an identity-less context.
	UserID string

	// Context is the base context the request carries; it defaults to
	// context.Background(). Set it to inject a pre-arranged context
	// (a cancelled one, say) — identity and URL params are layered on
	// top, so a cancelled base stays cancelled.
	Context context.Context

	// RequestID defaults to "test-request-id".
	RequestID string

	// URLParams are injected via SetPathValue, e.g.
	// {"id": "test-host", "volumeName": "my-volume"}.
	URLParams map[string]string

	// Body is optional; when set, Content-Type defaults to
	// application/json (override with Header).
	Body   io.Reader
	Header http.Header
}

// Perform invokes a handler the way the router does — request-scoped
// context values plus URL params — and records the response.
// It replaces the per-domain recordXRequest scaffolding so handler
// contract tests share one request builder.
func Perform(handler http.HandlerFunc, req Request) *httptest.ResponseRecorder {
	if req.Method == "" {
		req.Method = http.MethodGet
	}
	if req.RequestID == "" {
		req.RequestID = "test-request-id"
	}

	base := req.Context
	if base == nil {
		base = context.Background()
	}
	ctx := contextutils.SetRequestID(base, req.RequestID)
	if req.UserID != "" {
		ctx = contextutils.SetUserID(ctx, req.UserID)
	}

	httpReq := httptest.NewRequest(req.Method, req.Path, req.Body).WithContext(ctx)
	if req.Header == nil {
		req.Header = http.Header{}
	}
	if req.Body != nil && req.Header.Get("Content-Type") == "" {
		req.Header.Set("Content-Type", "application/json")
	}
	for key, values := range req.Header {
		for _, value := range values {
			httpReq.Header.Add(key, value)
		}
	}

	for key, value := range req.URLParams {
		httpReq.SetPathValue(key, value)
	}

	w := httptest.NewRecorder()
	handler(w, httpReq)
	return w
}
