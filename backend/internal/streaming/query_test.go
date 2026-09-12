package streaming

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestParseLogWindow pins the strict window contract: valid values
// round-trip, missing values fall back to the defaults (zero time,
// tail -1), and malformed values are 400 AppErrors whose wording says
// "use RFC3339".
func TestParseLogWindow(t *testing.T) {
	tests := []struct {
		name      string
		query     string
		wantSince time.Time
		wantTail  int64
		wantCode  string
	}{
		{
			name:      "defaults are the zero time and tail -1",
			query:     "",
			wantSince: time.Time{},
			wantTail:  -1,
		},
		{
			name:      "valid since and tail round-trip",
			query:     "since=2024-01-01T00:00:00Z&tail=100",
			wantSince: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
			wantTail:  100,
		},
		{
			name:     "malformed since is a 400 INVALID_SINCE_PARAM",
			query:    "since=invalid-date",
			wantCode: "INVALID_SINCE_PARAM",
		},
		{
			name:     "malformed tail is a 400 INVALID_TAIL_PARAM",
			query:    "tail=not-a-number",
			wantCode: "INVALID_TAIL_PARAM",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/logs?"+tt.query, nil)

			since, tail, appErr := ParseLogWindow(req)

			if tt.wantCode != "" {
				require.NotNil(t, appErr, "a malformed window parameter must yield an AppError")
				assert.Equal(t, tt.wantCode, appErr.Code)
				assert.Equal(t, http.StatusBadRequest, appErr.Status)
				assert.Contains(t, appErr.Message, "use RFC3339")
				return
			}
			require.Nil(t, appErr)
			assert.Equal(t, tt.wantSince, since)
			assert.Equal(t, tt.wantTail, tail)
		})
	}
}

// TestParseFollow pins the lenient follow contract: only a present,
// bool-parseable "true" selects streaming; anything else (missing,
// "false", or malformed) is plain false.
func TestParseFollow(t *testing.T) {
	tests := []struct {
		name  string
		query string
		want  bool
	}{
		{name: "absent means false", query: "", want: false},
		{name: "follow=true means true", query: "follow=true", want: true},
		{name: "follow=false means false", query: "follow=false", want: false},
		{name: "malformed value is lenient false", query: "follow=banana", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/logs?"+tt.query, nil)
			assert.Equal(t, tt.want, ParseFollow(req))
		})
	}
}
