package quadlets

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/Homerus25/podland/backend/internal/bulkremove"
	"github.com/Homerus25/podland/backend/internal/contextutils"
	"github.com/Homerus25/podland/backend/internal/errors"
	"github.com/Homerus25/podland/backend/internal/streaming"
	"github.com/Homerus25/podland/backend/internal/web"
)

// QuadletHandler handles quadlet requests
//
// JSON endpoints cross the endpoint envelope (internal/web). AutoUpdate
// and the move's migrateVolumeData half stream NDJSON through the
// streaming kit's progress seam (internal/streaming.HandleProgress)
// rather than encoding one DTO.
//
// The handler is constructed over both quadlet modules (architecture
// review 2026-10-19, candidate C2): the lean service owns CRUD, lifecycle,
// logs, generate, import, and auto-update; the transfer service owns the
// cross-host move/copy.
type QuadletHandler struct {
	service  *QuadletService
	transfer *QuadletTransferService
}

// NewQuadletHandler creates a new QuadletHandler over both quadlet
// modules: the lean service and the transfer service.
func NewQuadletHandler(service *QuadletService, transfer *QuadletTransferService) *QuadletHandler {
	return &QuadletHandler{
		service:  service,
		transfer: transfer,
	}
}

// List handles listing quadlets
func (h *QuadletHandler) List(w http.ResponseWriter, r *http.Request) {
	web.Handle(w, r, http.StatusOK, func(ctx context.Context, userID, hostID string) ([]QuadletListResponse, error) {
		filters := ListFilters{
			Resource: r.URL.Query().Get("resource"),
			Name:     r.URL.Query().Get("name"),
		}
		return h.service.List(ctx, userID, hostID, filters)
	})
}

// Create handles creating a quadlet
func (h *QuadletHandler) Create(w http.ResponseWriter, r *http.Request) {
	web.Handle(w, r, http.StatusCreated, func(ctx context.Context, userID, hostID string) (*QuadletCreateResponse, error) {
		var request QuadletCreateRequest
		if appErr := web.Decode(w, r, &request); appErr != nil {
			return nil, appErr
		}

		return h.service.Create(ctx, userID, hostID, request)
	})
}

// Get handles getting a quadlet by ID
func (h *QuadletHandler) Get(w http.ResponseWriter, r *http.Request) {
	web.Handle(w, r, http.StatusOK, func(ctx context.Context, userID, hostID string) (*QuadletGetResponse, error) {
		quadletID, guard := web.URLParam(r, "quadletId", "MISSING_QUADLET_ID")
		if guard != nil {
			return nil, guard
		}
		return h.service.Get(ctx, userID, hostID, quadletID)
	})
}

// Update handles updating a quadlet
func (h *QuadletHandler) Update(w http.ResponseWriter, r *http.Request) {
	web.Handle(w, r, http.StatusOK, func(ctx context.Context, userID, hostID string) (*QuadletGetResponse, error) {
		quadletID, guard := web.URLParam(r, "quadletId", "MISSING_QUADLET_ID")
		if guard != nil {
			return nil, guard
		}

		var request QuadletUpdateRequest
		if appErr := web.Decode(w, r, &request); appErr != nil {
			return nil, appErr
		}

		return h.service.Update(ctx, userID, hostID, quadletID, request)
	})
}

// Delete handles deleting a quadlet. force defaults to false and stop
// defaults to true (doku/description_quadlets_api.md); unparseable
// values leave the defaults in place. A running resource is stopped
// through the lifecycle seam before remove unless force is set; with
// stop=false a running resource is refused (400).
func (h *QuadletHandler) Delete(w http.ResponseWriter, r *http.Request) {
	web.Handle(w, r, http.StatusNoContent, func(ctx context.Context, userID, hostID string) (struct{}, error) {
		quadletID, guard := web.URLParam(r, "quadletId", "MISSING_QUADLET_ID")
		if guard != nil {
			return struct{}{}, guard
		}

		force := web.QueryBool(r, "force", false)
		stop := web.QueryBool(r, "stop", true)

		return struct{}{}, h.service.Delete(ctx, userID, hostID, quadletID, force, stop)
	})
}

// BulkRemove handles removing several quadlets in one request. Route-level
// RequireHostPermission(write) (app.go) gates it at write. The body
// carries the quadlet ids and the force flag (default false); the answer is
// 200 with one report entry per requested id, in request order — a per-id
// failure is an entry, never a status code. The removals cross as ONE
// podman call — the batch remove endpoint accepts the whole array (the
// service's BulkDelete) — behind the same stop-first ritual as the
// single-item Delete: a missing id answers a QUADLET_NOT_FOUND entry, a
// quadlet whose resource is still up is stopped first.
//
// stop stays true, always: a silent server-side default with no request
// option, so containers running from a selected quadlet are stopped
// through the lifecycle seam before the removal (spec 461, user story
// 14). The service bakes the default in — the request has no stop
// field, so a client cannot turn it off.
func (h *QuadletHandler) BulkRemove(w http.ResponseWriter, r *http.Request) {
	web.Handle(w, r, http.StatusOK, func(ctx context.Context, userID, hostID string) ([]bulkremove.Entry, error) {
		var request bulkremove.Request
		if appErr := web.Decode(w, r, &request); appErr != nil {
			return nil, appErr
		}

		return h.service.BulkDelete(ctx, userID, hostID, request.IDs, request.Force), nil
	})
}

// Start handles starting a quadlet-managed resource.
// Runs `systemctl start <unit>` over SSH via the service.
// Returns 200 with {id, state, unit}, or QUADLET_ALREADY_RUNNING (400) /
// QUADLET_NOT_FOUND (404) / CONNECTION_FAILED (502) on error.
func (h *QuadletHandler) Start(w http.ResponseWriter, r *http.Request) {
	web.Handle(w, r, http.StatusOK, func(ctx context.Context, userID, hostID string) (*QuadletLifecycleResponse, error) {
		quadletID, guard := web.URLParam(r, "quadletId", "MISSING_QUADLET_ID")
		if guard != nil {
			return nil, guard
		}
		return h.service.Start(ctx, userID, hostID, quadletID)
	})
}

// Stop handles stopping a running quadlet-managed resource.
// Runs `systemctl stop <unit>` over SSH, honouring the `timeout`
// query param (doku/description_quadlets_api.md). No force param —
// see LifecycleOptions (issue 205).
func (h *QuadletHandler) Stop(w http.ResponseWriter, r *http.Request) {
	web.Handle(w, r, http.StatusOK, func(ctx context.Context, userID, hostID string) (*QuadletLifecycleResponse, error) {
		quadletID, guard := web.URLParam(r, "quadletId", "MISSING_QUADLET_ID")
		if guard != nil {
			return nil, guard
		}
		return h.service.Stop(ctx, userID, hostID, quadletID, parseLifecycleOptions(r))
	})
}

// Restart handles restarting a quadlet-managed resource.
// Runs `systemctl restart <unit>` over SSH, honouring the `timeout`
// query param (doku/description_quadlets_api.md). No force param —
// see LifecycleOptions (issue 205).
func (h *QuadletHandler) Restart(w http.ResponseWriter, r *http.Request) {
	web.Handle(w, r, http.StatusOK, func(ctx context.Context, userID, hostID string) (*QuadletLifecycleResponse, error) {
		quadletID, guard := web.URLParam(r, "quadletId", "MISSING_QUADLET_ID")
		if guard != nil {
			return nil, guard
		}
		return h.service.Restart(ctx, userID, hostID, quadletID, parseLifecycleOptions(r))
	})
}

// parseLifecycleOptions reads the `timeout` (int, seconds) query parameter
// for stop/restart through web's lenient helper. Timeout defaults to 0
// (unset) — positivity is this domain's judgment, so a non-positive value
// stays unset. There is no force param to read (issue 205).
func parseLifecycleOptions(r *http.Request) LifecycleOptions {
	opts := LifecycleOptions{}
	if t := web.QueryInt(r, "timeout", 0); t > 0 {
		opts.Timeout = t
	}
	return opts
}

// GetLogs handles getting quadlet logs: one kit call — the streaming kit
// owns the whole handler-side dispatch (the window parse, the query-error
// fallback, the param guard, the follow fork), and the domain contributes
// its param name, guard code, and two service calls.
func (h *QuadletHandler) GetLogs(w http.ResponseWriter, r *http.Request) {
	streaming.HandleLogs(w, r, streaming.LogsDispatchOp[*QuadletLogsResponse]{
		ParamName: "quadletId",
		GuardCode: "MISSING_QUADLET_ID",
		Follow:    h.service.GetLogsWithStream,
		Buffered:  h.service.GetLogs,
	})
}

// MovePreview handles the move/copy preview for a quadlet: the referenced
// volumes, the target collision flag, and the source's running state
// (doku/description_quadlets_api.md). Read-level on the source suffices
// (the route sits at the subtree's read gate); the target host's access
// is enforced inside the service against the access store.
func (h *QuadletHandler) MovePreview(w http.ResponseWriter, r *http.Request) {
	web.Handle(w, r, http.StatusOK, func(ctx context.Context, userID, hostID string) (*QuadletMovePreviewResponse, error) {
		quadletID, guard := web.URLParam(r, "quadletId", "MISSING_QUADLET_ID")
		if guard != nil {
			return nil, guard
		}

		targetHostID, guard := web.QueryString(r, "targetHostId", "MISSING_TARGET_HOST_ID")
		if guard != nil {
			return nil, guard
		}

		return h.transfer.MovePreview(ctx, userID, hostID, quadletID, targetHostID)
	})
}

// Move handles the move/copy transfer of a quadlet to another host
// (doku/description_quadlets_api.md). The route sits at the subtree's
// write gate for the SOURCE host; the target host's write-level
// access is enforced inside the service against the access store. Copy
// mode (issue 210) answers the transfer envelope with status "copied";
// move mode (issue 211) removes the source after the install and answers
// "moved", or "partial" with an error detail when the removal failed.
//
// Transport (issue 214): a plain file-only transfer answers one JSON
// envelope exactly as before. A transfer with migrateVolumeData set
// streams NDJSON instead through the streaming kit's progress seam
// (HandleProgress) — the data phase can run minutes on real volumes, so
// the caller gets per-volume progress events and one terminal event
// (complete envelope or mapped error) rather than a held-open silence;
// registry pulls set the same precedent. Errors answered BEFORE the
// stream begins (bad param, bad body) keep the plain envelope.
func (h *QuadletHandler) Move(w http.ResponseWriter, r *http.Request) {
	quadletID, guard := web.URLParam(r, "quadletId", "MISSING_QUADLET_ID")
	if guard != nil {
		errors.RespondWithError(w, guard, contextutils.GetRequestID(r.Context()))
		return
	}

	// Decode answers its 400s before the stream begins; the streaming
	// envelope takes over from there (the images Pull arrangement).
	var request QuadletMoveRequest
	if appErr := web.Decode(w, r, &request); appErr != nil {
		errors.RespondWithError(w, appErr, contextutils.GetRequestID(r.Context()))
		return
	}

	if !request.MigrateVolumeData {
		web.Handle(w, r, http.StatusOK, func(ctx context.Context, userID, hostID string) (*QuadletMoveResponse, error) {
			return h.transfer.Move(ctx, userID, hostID, quadletID, request)
		})
		return
	}

	streaming.HandleProgress(w, r, "Move quadlet with volume data migration", []slog.Attr{slog.String("quadletID", quadletID)}, streaming.ProgressConfig[QuadletMoveEvent, *QuadletMoveResponse]{
		Operation: func(ctx context.Context, op streaming.OperationContext, onProgress func(QuadletMoveEvent)) (*QuadletMoveResponse, error) {
			return h.transfer.MoveWithProgress(ctx, op.UserID, op.HostID, quadletID, request, func(event QuadletMoveEvent) {
				event.OperationID = op.OperationID
				event.HostID = op.HostID
				onProgress(event)
			})
		},
		CompleteEvent: func(op streaming.OperationContext, response *QuadletMoveResponse) any {
			return NewQuadletMoveCompleteEvent(op.OperationID, op.HostID, *response)
		},
		ErrorDefault: errors.ErrConnectionFailed,
		ErrorEvent: func(op streaming.OperationContext, code, message string, _ bool) any {
			return NewQuadletMoveErrorEvent(op.OperationID, op.HostID, code, message)
		},
	})
}

// Generate handles generating a quadlet from an existing container or pod
func (h *QuadletHandler) Generate(w http.ResponseWriter, r *http.Request) {
	web.Handle(w, r, http.StatusOK, func(ctx context.Context, userID, hostID string) (*QuadletGenerateResponse, error) {
		var request QuadletGenerateRequest
		if appErr := web.Decode(w, r, &request); appErr != nil {
			return nil, appErr
		}

		return h.service.Generate(ctx, userID, hostID, request)
	})
}

// Import handles importing quadlets from uploaded content
func (h *QuadletHandler) Import(w http.ResponseWriter, r *http.Request) {
	web.Handle(w, r, http.StatusCreated, func(ctx context.Context, userID, hostID string) (*QuadletImportResponse, error) {
		var request QuadletImportRequest
		if appErr := web.Decode(w, r, &request); appErr != nil {
			return nil, appErr
		}

		return h.service.Import(ctx, userID, hostID, request)
	})
}

// ConvertCompose handles converting compose file content into quadlet
// entries (spec #336). The endpoint is the first quadlets route OUTSIDE
// the host subtree: it crosses the user-scoped envelope (web.HandleUser)
// — any authenticated user may call it, with no host access check —
// because conversion touches no host data. The response carries the
// import endpoint's entry shape so the frontend can forward it unchanged.
func (h *QuadletHandler) ConvertCompose(w http.ResponseWriter, r *http.Request) {
	web.HandleUser(w, r, http.StatusOK, func(ctx context.Context, userID string) (*QuadletConvertComposeResponse, error) {
		// The 1 MiB cap is on the raw request body, enforced at the HTTP
		// layer so an oversized body is never fully buffered: a body over
		// the cap trips the reader inside web.Decode's json.Decode and
		// answers 400 INVALID_REQUEST (the volumes Populate precedent).
		r.Body = http.MaxBytesReader(w, r.Body, MaxComposeContentBytes)

		var request QuadletConvertComposeRequest
		if appErr := web.Decode(w, r, &request); appErr != nil {
			return nil, appErr
		}

		return h.service.ConvertCompose(ctx, request)
	})
}

// AutoUpdate handles running `podman auto-update` on the host with live
// NDJSON streaming of remote output, terminating in a parsed report event.
// The stream opens with the service's plan event naming every participating
// unit and reports each matching pull as a pulling event (issue 448); the
// handler stamps the operation and host identities on each service event,
// the move stream's arrangement. The streaming kit's progress seam
// (HandleProgress) owns the handler-side ritual, including the 10-minute
// write-deadline extension: registry pulls make runs routinely exceed the
// global 30-second write timeout.
func (h *QuadletHandler) AutoUpdate(w http.ResponseWriter, r *http.Request) {
	dryRun := web.QueryBool(r, "dryRun", false)
	filter := r.URL.Query().Get("filter")

	streaming.HandleProgress(w, r, "AutoUpdate quadlets", []slog.Attr{slog.Bool("dryRun", dryRun), slog.String("filter", filter)}, streaming.ProgressConfig[AutoUpdateEvent, AutoUpdateReport]{
		Operation: func(ctx context.Context, op streaming.OperationContext, onProgress func(AutoUpdateEvent)) (AutoUpdateReport, error) {
			return h.service.AutoUpdate(ctx, op.UserID, op.HostID, dryRun, filter, func(event AutoUpdateEvent) {
				event.OperationID = op.OperationID
				event.HostID = op.HostID
				onProgress(event)
			})
		},
		CompleteEvent: func(op streaming.OperationContext, report AutoUpdateReport) any {
			return NewAutoUpdateCompleteEvent(op.OperationID, op.HostID, report)
		},
		ErrorDefault: errors.ErrConnectionFailed,
		ErrorEvent: func(op streaming.OperationContext, code, message string, _ bool) any {
			return NewAutoUpdateErrorEvent(op.OperationID, op.HostID, code, message)
		},
	})
}
