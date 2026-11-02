package streaming

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Homerus25/podland/backend/internal/contextutils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The handler-side dispatch suite: HandleLogs owns the whole log-dispatch
// ritual (window parse, query-error fallback, param guard, follow fork),
// so the fork is pinned here once over fake closures — the per-domain
// handler suites pin only their composition.

// fakeLogsDTO is the buffered closure's response stand-in: the dispatch
// is generic over the buffered DTO and no domain vocabulary crosses into
// the kit, so the kit suite pins the fork over its own fake.
type fakeLogsDTO struct {
	Logs string `json:"logs"`
}

// dispatchCapture records what the two fake closures receive.
type dispatchCapture struct {
	followCalled   bool
	followTarget   string
	followSince    time.Time
	followTail     int64
	bufferedCalled bool
	bufferedTarget string
	bufferedSince  time.Time
	bufferedTail   int64
}

// op builds the dispatch input over the capture's fake closures: the
// domain's URL-param name and guard code plus the two service calls,
// exactly the vocabulary a log handler passes.
func (c *dispatchCapture) op() LogsDispatchOp[fakeLogsDTO] {
	return LogsDispatchOp[fakeLogsDTO]{
		ParamName: "thingId",
		GuardCode: "MISSING_THING_ID",
		Follow: func(ctx context.Context, userID, machineID, target string, since time.Time, tail int64, onEvent func(LogEvent)) error {
			c.followCalled = true
			c.followTarget = target
			c.followSince = since
			c.followTail = tail
			onEvent(LogEvent{Timestamp: "2026-09-01T10:00:00Z", Level: "INFO", Message: "hello"})
			return nil
		},
		Buffered: func(ctx context.Context, userID, machineID, target string, since time.Time, tail int64) (fakeLogsDTO, error) {
			c.bufferedCalled = true
			c.bufferedTarget = target
			c.bufferedSince = since
			c.bufferedTail = tail
			return fakeLogsDTO{Logs: "buffered lines"}, nil
		},
	}
}

// performLogDispatch drives HandleLogs the way the router wires it: URL
// params (the machine id and the domain's target param) plus the
// caller identity the subtree middleware already validated.
func performLogDispatch(t *testing.T, capture *dispatchCapture, urlParams map[string]string, query string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/api/machines/test-machine/things/thing-123/logs"+query, nil)
	for key, value := range urlParams {
		req.SetPathValue(key, value)
	}
	ctx := contextutils.SetUserID(context.Background(), "test-user")
	req = req.WithContext(ctx)
	w := httptest.NewRecorder()
	HandleLogs(w, req, capture.op())
	return w
}

// dispatchParams is the fully-populated URL-param set: the machine id
// plus the domain's resolved target.
var dispatchParams = map[string]string{"id": "test-machine", "thingId": "thing-123"}

// TestHandleLogs_QueryErrorRidesJSONEnvelopeEvenWithFollow pins the
// fallback: a query error answers the endpoint envelope even when
// follow=true was requested, and neither closure runs.
func TestHandleLogs_QueryErrorRidesJSONEnvelopeEvenWithFollow(t *testing.T) {
	capture := &dispatchCapture{}

	w := performLogDispatch(t, capture, dispatchParams, "?follow=true&since=not-a-date")

	require.Equal(t, http.StatusBadRequest, w.Code, "a query error must answer 400 through the endpoint envelope")
	assert.Contains(t, w.Body.String(), "INVALID_SINCE_PARAM", "the query error's code must ride the endpoint envelope")
	assert.NotContains(t, w.Body.String(), "STREAM_ENDED", "no stream may begin for a query error")
	assert.False(t, capture.followCalled, "the follow closure must not run for a query error")
	assert.False(t, capture.bufferedCalled, "the buffered closure must not run for a query error")
}

// TestHandleLogs_MissingParamAnswersGuardCodeThroughBufferedEnvelope pins
// the guard: a missing target param answers the passed guard code through
// the buffered envelope — even when follow=true was requested — and
// neither closure runs.
func TestHandleLogs_MissingParamAnswersGuardCodeThroughBufferedEnvelope(t *testing.T) {
	capture := &dispatchCapture{}

	w := performLogDispatch(t, capture, map[string]string{"id": "test-machine"}, "?follow=true")

	require.Equal(t, http.StatusBadRequest, w.Code, "a missing param must answer 400 through the buffered envelope")
	assert.Contains(t, w.Body.String(), "MISSING_THING_ID", "the passed guard code must answer, not a kit-minted one")
	assert.False(t, capture.followCalled, "the follow closure must not run for a missing param")
	assert.False(t, capture.bufferedCalled, "the buffered closure must not run for a missing param")
}

// TestHandleLogs_CleanFollowCrossesStreamingEnvelopeWithParsedWindow pins
// the streaming half of the fork: a clean follow=true request crosses
// the streaming envelope with the parsed window, and the follow closure
// receives the resolved target and that window.
func TestHandleLogs_CleanFollowCrossesStreamingEnvelopeWithParsedWindow(t *testing.T) {
	capture := &dispatchCapture{}

	w := performLogDispatch(t, capture, dispatchParams, "?follow=true&since=2026-09-01T10:00:00Z&tail=7")

	require.Equal(t, http.StatusOK, w.Code, "a clean follow request must cross the streaming envelope")
	lines := decodeLines(t, w.Body.String())
	require.Len(t, lines, 2, "one event plus exactly one terminal event")
	assert.Equal(t, `{"timestamp":"2026-09-01T10:00:00Z","level":"INFO","message":"hello"}`, lines[0],
		"the follow closure's events must cross byte-identical")
	assert.True(t, strings.HasSuffix(lines[1], `,"level":"INFO","message":"STREAM_ENDED"}`),
		"a clean stream end must emit the STREAM_ENDED terminal, got %s", lines[1])
	require.True(t, capture.followCalled, "the follow closure must run for a clean follow request")
	assert.Equal(t, "thing-123", capture.followTarget, "the follow closure must receive the resolved target")
	assert.Equal(t, time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC), capture.followSince,
		"the follow closure must receive the parsed since")
	assert.Equal(t, int64(7), capture.followTail, "the follow closure must receive the parsed tail")
	assert.False(t, capture.bufferedCalled, "the buffered closure must not run for a clean follow request")
}

// TestHandleLogs_NonFollowAnswersBufferedEnvelopeWithDTO pins the
// buffered half of the fork: without a clean follow request the buffered
// envelope answers with the buffered closure's DTO, and the closure
// receives the resolved target and the parsed window.
func TestHandleLogs_NonFollowAnswersBufferedEnvelopeWithDTO(t *testing.T) {
	tests := []struct {
		name  string
		query string
	}{
		{name: "no follow param", query: "?since=2026-09-01T10:00:00Z&tail=5"},
		{name: "explicit follow=false", query: "?follow=false&since=2026-09-01T10:00:00Z&tail=5"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			capture := &dispatchCapture{}

			w := performLogDispatch(t, capture, dispatchParams, tc.query)

			require.Equal(t, http.StatusOK, w.Code, "the buffered envelope must answer 200")
			assert.Contains(t, w.Body.String(), `"logs":"buffered lines"`, "the buffered closure's DTO must ride the envelope")
			require.True(t, capture.bufferedCalled, "the buffered closure must run")
			assert.Equal(t, "thing-123", capture.bufferedTarget, "the buffered closure must receive the resolved target")
			assert.Equal(t, time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC), capture.bufferedSince,
				"the buffered closure must receive the parsed since")
			assert.Equal(t, int64(5), capture.bufferedTail, "the buffered closure must receive the parsed tail")
			assert.False(t, capture.followCalled, "the follow closure must not run without a clean follow request")
		})
	}
}
