package testutils

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"

	"github.com/Homerus25/podland/backend/internal/contextutils"
	"github.com/go-chi/chi/v5"
)

// Request describes one handler invocation for Perform: the identity
// the subtree middleware would have validated, the chi URL params the
// router would have routed, and an optional body.
type Request struct {
	Method string

	// Path is used for the httptest request; URL params come from
	// URLParams, mirroring how chi populates them (the path string
	// itself is not parsed).
	Path string

	// UserID is the authenticated caller the subtree middleware would
	// have validated. No default: set it explicitly ("test-user" in
	// most handler tests); leave empty only when deliberately testing
	// an identity-less context.
	UserID string

	// RequestID defaults to "test-request-id".
	RequestID string

	// URLParams are injected into a fresh chi route context, e.g.
	// {"id": "test-machine", "volumeName": "my-volume"}.
	URLParams map[string]string

	// Body is optional; when set, Content-Type defaults to
	// application/json (override with Header).
	Body   io.Reader
	Header http.Header
}

// Perform invokes a handler the way the router does — request-scoped
// context values plus a chi route context — and records the response.
// It replaces the per-domain recordXRequest scaffolding so handler
// contract tests share one request builder.
func Perform(handler http.HandlerFunc, req Request) *httptest.ResponseRecorder {
	if req.Method == "" {
		req.Method = http.MethodGet
	}
	if req.RequestID == "" {
		req.RequestID = "test-request-id"
	}

	ctx := contextutils.SetRequestID(context.Background(), req.RequestID)
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

	rctx := chi.NewRouteContext()
	for key, value := range req.URLParams {
		rctx.URLParams.Add(key, value)
	}
	httpReq = httpReq.WithContext(context.WithValue(httpReq.Context(), chi.RouteCtxKey, rctx))

	w := httptest.NewRecorder()
	handler(w, httpReq)
	return w
}
