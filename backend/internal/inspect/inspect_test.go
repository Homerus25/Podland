package inspect

import (
	"errors"
	"fmt"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	errorhandling "go.podman.io/podman/v6/pkg/errorhandling"
)

// The matrix is pinned once here with a stand-in report type and
// sentinel; the domains keep their composition pins (their own sentinel
// and label wired through their service).

var errWidgetNotFound = errors.New("widget not found")

type widgetReport struct{ ID string }

func TestResolve_ReturnsFirstReport(t *testing.T) {
	report := widgetReport{ID: "w-1"}

	got, err := Resolve(
		[]widgetReport{report}, nil, nil,
		errWidgetNotFound, "widget", "w-1",
	)

	require.NoError(t, err)
	assert.Equal(t, report, got)
}

func TestResolve_TransportNotFoundClassifiesIntoSentinel(t *testing.T) {
	notFound := &errorhandling.ErrorModel{ResponseCode: http.StatusNotFound, Message: "no such widget"}

	got, err := Resolve[widgetReport](
		nil, nil, notFound,
		errWidgetNotFound, "widget", "w-1",
	)

	require.Error(t, err)
	assert.Equal(t, widgetReport{}, got)
	assert.ErrorIs(t, err, errWidgetNotFound)
}

func TestResolve_PerItemNotFoundClassifiesIntoSentinel(t *testing.T) {
	got, err := Resolve[widgetReport](
		nil, []error{fmt.Errorf("no such widget %q", "w-1")}, nil,
		errWidgetNotFound, "widget", "w-1",
	)

	require.Error(t, err)
	assert.Equal(t, widgetReport{}, got)
	assert.ErrorIs(t, err, errWidgetNotFound)
}

func TestResolve_EngineFailureWrapsWithDomainLabel(t *testing.T) {
	got, err := Resolve[widgetReport](
		nil, nil, assert.AnError,
		errWidgetNotFound, "widget", "w-1",
	)

	require.Error(t, err)
	assert.Equal(t, widgetReport{}, got)
	assert.Contains(t, err.Error(), "failed to inspect widget")
}

func TestResolve_ZeroReportsWithPerItemErrorWrapsCause(t *testing.T) {
	got, err := Resolve[widgetReport](
		nil, []error{assert.AnError}, nil,
		errWidgetNotFound, "widget", "w-1",
	)

	require.Error(t, err)
	assert.Equal(t, widgetReport{}, got)
	assert.ErrorIs(t, err, assert.AnError)
	assert.NotErrorIs(t, err, errWidgetNotFound)
	assert.Contains(t, err.Error(), "failed to inspect widget")
}

func TestResolve_ZeroReportsWithoutErrorsIsAnomaly(t *testing.T) {
	got, err := Resolve[widgetReport](
		nil, nil, nil,
		errWidgetNotFound, "widget", "w-1",
	)

	require.Error(t, err)
	assert.Equal(t, widgetReport{}, got)
	assert.Contains(t, err.Error(), "no inspect report returned for widget: w-1")
}
