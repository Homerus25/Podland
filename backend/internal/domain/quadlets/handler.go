package quadlets

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/Homerus25/podland/backend/internal/contextutils"
	"github.com/Homerus25/podland/backend/internal/errors"
	"github.com/Homerus25/podland/backend/internal/sshexec"
	"github.com/Homerus25/podland/backend/internal/streaming"
	"github.com/Homerus25/podland/backend/internal/web"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

// QuadletHandler handles quadlet requests
//
// JSON endpoints cross the endpoint envelope (internal/web). AutoUpdate
// keeps its own shape — it streams NDJSON rather than encoding one DTO.
type QuadletHandler struct {
	service *QuadletService
}

// NewQuadletHandler creates a new QuadletHandler
func NewQuadletHandler(service *QuadletService) *QuadletHandler {
	return &QuadletHandler{
		service: service,
	}
}

// List handles listing quadlets
func (h *QuadletHandler) List(w http.ResponseWriter, r *http.Request) {
	web.Handle(w, r, http.StatusOK, func(ctx context.Context, userID, machineID string) ([]QuadletListResponse, error) {
		filters := ListFilters{
			Resource: r.URL.Query().Get("resource"),
			Name:     r.URL.Query().Get("name"),
		}
		return h.service.List(ctx, userID, machineID, filters)
	})
}

// Create handles creating a quadlet
func (h *QuadletHandler) Create(w http.ResponseWriter, r *http.Request) {
	web.Handle(w, r, http.StatusCreated, func(ctx context.Context, userID, machineID string) (*QuadletCreateResponse, error) {
		var request QuadletCreateRequest
		if appErr := web.Decode(r, &request); appErr != nil {
			return nil, appErr
		}

		return h.service.Create(ctx, userID, machineID, request)
	})
}

// Get handles getting a quadlet by ID
func (h *QuadletHandler) Get(w http.ResponseWriter, r *http.Request) {
	web.Handle(w, r, http.StatusOK, func(ctx context.Context, userID, machineID string) (*QuadletGetResponse, error) {
		quadletID, guard := web.URLParam(r, "quadletId", "MISSING_QUADLET_ID")
		if guard != nil {
			return nil, guard
		}
		return h.service.Get(ctx, userID, machineID, quadletID)
	})
}

// Update handles updating a quadlet
func (h *QuadletHandler) Update(w http.ResponseWriter, r *http.Request) {
	web.Handle(w, r, http.StatusOK, func(ctx context.Context, userID, machineID string) (*QuadletGetResponse, error) {
		quadletID, guard := web.URLParam(r, "quadletId", "MISSING_QUADLET_ID")
		if guard != nil {
			return nil, guard
		}

		var request QuadletUpdateRequest
		if appErr := web.Decode(r, &request); appErr != nil {
			return nil, appErr
		}

		return h.service.Update(ctx, userID, machineID, quadletID, request)
	})
}

// Delete handles deleting a quadlet. force defaults to false and stop
// defaults to true (doku/description_quadlets_api.md); unparseable
// values leave the defaults in place. A running resource is stopped
// through the lifecycle seam before remove unless force is set; with
// stop=false a running resource is refused (400).
func (h *QuadletHandler) Delete(w http.ResponseWriter, r *http.Request) {
	web.Handle(w, r, http.StatusNoContent, func(ctx context.Context, userID, machineID string) (struct{}, error) {
		quadletID, guard := web.URLParam(r, "quadletId", "MISSING_QUADLET_ID")
		if guard != nil {
			return struct{}{}, guard
		}

		force := web.QueryBool(r, "force", false)
		stop := web.QueryBool(r, "stop", true)

		return struct{}{}, h.service.Delete(ctx, userID, machineID, quadletID, force, stop)
	})
}

// Start handles starting a quadlet-managed resource.
// Runs `systemctl start <unit>` over SSH via the service.
// Returns 200 with {id, state, unit}, or QUADLET_ALREADY_RUNNING (400) /
// QUADLET_NOT_FOUND (404) / CONNECTION_FAILED (502) on error.
func (h *QuadletHandler) Start(w http.ResponseWriter, r *http.Request) {
	web.Handle(w, r, http.StatusOK, func(ctx context.Context, userID, machineID string) (*QuadletLifecycleResponse, error) {
		quadletID, guard := web.URLParam(r, "quadletId", "MISSING_QUADLET_ID")
		if guard != nil {
			return nil, guard
		}
		return h.service.Start(ctx, userID, machineID, quadletID)
	})
}

// Stop handles stopping a running quadlet-managed resource.
// Runs `systemctl stop <unit>` over SSH, honouring the `timeout`
// query param (doku/description_quadlets_api.md). No force param —
// see LifecycleOptions (issue 205).
func (h *QuadletHandler) Stop(w http.ResponseWriter, r *http.Request) {
	web.Handle(w, r, http.StatusOK, func(ctx context.Context, userID, machineID string) (*QuadletLifecycleResponse, error) {
		quadletID, guard := web.URLParam(r, "quadletId", "MISSING_QUADLET_ID")
		if guard != nil {
			return nil, guard
		}
		return h.service.Stop(ctx, userID, machineID, quadletID, parseLifecycleOptions(r))
	})
}

// Restart handles restarting a quadlet-managed resource.
// Runs `systemctl restart <unit>` over SSH, honouring the `timeout`
// query param (doku/description_quadlets_api.md). No force param —
// see LifecycleOptions (issue 205).
func (h *QuadletHandler) Restart(w http.ResponseWriter, r *http.Request) {
	web.Handle(w, r, http.StatusOK, func(ctx context.Context, userID, machineID string) (*QuadletLifecycleResponse, error) {
		quadletID, guard := web.URLParam(r, "quadletId", "MISSING_QUADLET_ID")
		if guard != nil {
			return nil, guard
		}
		return h.service.Restart(ctx, userID, machineID, quadletID, parseLifecycleOptions(r))
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

// GetLogs handles getting quadlet logs. The query is parsed once: only
// a clean follow request crosses the streaming envelope, while query
// errors ride the JSON envelope even when follow=true was requested, so
// no code path here writes an error response by hand.
func (h *QuadletHandler) GetLogs(w http.ResponseWriter, r *http.Request) {
	since, tail, queryErr := streaming.ParseLogWindow(r)
	if queryErr != nil {
		web.Handle(w, r, http.StatusOK, func(ctx context.Context, userID, machineID string) (struct{}, error) {
			return struct{}{}, queryErr
		})
		return
	}

	quadletID, guard := web.URLParam(r, "quadletId", "MISSING_QUADLET_ID")

	if guard == nil && streaming.ParseFollow(r) {
		slog.InfoContext(r.Context(), "streamLogs", slog.String("quadletID", quadletID), slog.Bool("follow", true))
		streaming.RunLogStream(w, r, streaming.LogStreamOp{
			Since: since,
			Tail:  tail,
			Stream: func(ctx context.Context, userID, machineID string, onEvent func(streaming.LogEvent)) error {
				return h.service.GetLogsWithStream(ctx, userID, machineID, quadletID, since, tail, onEvent)
			},
		})
		return
	}

	web.Handle(w, r, http.StatusOK, func(ctx context.Context, userID, machineID string) (*QuadletLogsResponse, error) {
		if guard != nil {
			return nil, guard
		}
		return h.service.GetLogs(ctx, userID, machineID, quadletID, since, tail)
	})
}

// MovePreview handles the move/copy preview for a quadlet: the referenced
// volumes, the target collision flag, and the source's running state
// (doku/description_quadlets_api.md). Read-level on the source suffices
// (the route sits at the subtree's read gate); the target machine's access
// is enforced inside the service against the access store.
func (h *QuadletHandler) MovePreview(w http.ResponseWriter, r *http.Request) {
	web.Handle(w, r, http.StatusOK, func(ctx context.Context, userID, machineID string) (*QuadletMovePreviewResponse, error) {
		quadletID, guard := web.URLParam(r, "quadletId", "MISSING_QUADLET_ID")
		if guard != nil {
			return nil, guard
		}

		targetMachineID, guard := web.QueryString(r, "targetMachineId", "MISSING_TARGET_MACHINE_ID")
		if guard != nil {
			return nil, guard
		}

		return h.service.MovePreview(ctx, userID, machineID, quadletID, targetMachineID)
	})
}

// Move handles the move/copy transfer of a quadlet to another machine
// (doku/description_quadlets_api.md). The route sits at the subtree's
// write gate for the SOURCE machine; the target machine's write-level
// access is enforced inside the service against the access store. Copy
// mode (issue 210) answers the transfer envelope with status "copied";
// move mode (issue 211) removes the source after the install and answers
// "moved", or "partial" with an error detail when the removal failed.
//
// Transport (issue 214): a plain file-only transfer answers one JSON
// envelope exactly as before. A transfer with migrateVolumeData set
// streams NDJSON instead — the data phase can run minutes on real
// volumes, so the caller gets per-volume progress events and one terminal
// event (complete envelope or mapped error) rather than a held-open
// silence; registry pulls set the same precedent. Errors answered BEFORE
// the stream begins (bad param, bad body) keep the plain envelope.
func (h *QuadletHandler) Move(w http.ResponseWriter, r *http.Request) {
	quadletID, guard := web.URLParam(r, "quadletId", "MISSING_QUADLET_ID")
	if guard != nil {
		errors.RespondWithError(w, guard, contextutils.GetRequestID(r.Context()))
		return
	}

	// Decode answers its 400s before the stream begins; the streaming
	// envelope takes over from there (the images Pull arrangement).
	var request QuadletMoveRequest
	if appErr := web.Decode(r, &request); appErr != nil {
		errors.RespondWithError(w, appErr, contextutils.GetRequestID(r.Context()))
		return
	}

	if !request.MigrateVolumeData {
		web.Handle(w, r, http.StatusOK, func(ctx context.Context, userID, machineID string) (*QuadletMoveResponse, error) {
			return h.service.Move(ctx, userID, machineID, quadletID, request)
		})
		return
	}

	operationID := uuid.New().String()
	slog.InfoContext(r.Context(), "Move quadlet with volume data migration",
		slog.String("requestID", contextutils.GetRequestID(r.Context())),
		slog.String("machineID", chi.URLParam(r, "id")),
		slog.String("quadletID", quadletID),
		slog.String("operationID", operationID))

	// Registry pulls set the deadline precedent: the data phase routinely
	// exceeds the global 30-second write timeout, so this response carries
	// its own 10-minute one.
	if err := http.NewResponseController(w).SetWriteDeadline(time.Now().Add(10 * time.Minute)); err != nil {
		slog.WarnContext(r.Context(), "Failed to extend write deadline", slog.Any("error", err))
	}

	stream := streaming.Begin(w, r)
	if stream == nil {
		return
	}

	streaming.Run(r.Context(), stream.Writer, streaming.StreamConfig[QuadletMoveEvent, *QuadletMoveResponse]{
		Operation: func(ctx context.Context, onProgress func(QuadletMoveEvent)) (*QuadletMoveResponse, error) {
			return h.service.MoveWithProgress(ctx, stream.UserID, stream.MachineID, quadletID, request, func(event QuadletMoveEvent) {
				event.OperationID = operationID
				event.MachineID = stream.MachineID
				onProgress(event)
			})
		},
		OnEvent: func(event QuadletMoveEvent) error {
			return stream.Writer.Write(event)
		},
		CompleteEvent: func(response *QuadletMoveResponse) any {
			return NewQuadletMoveCompleteEvent(operationID, stream.MachineID, *response)
		},
		ErrorDefault: errors.ErrConnectionFailed,
		ErrorEvent: func(code, message string, _ bool) any {
			return NewQuadletMoveErrorEvent(operationID, stream.MachineID, code, message)
		},
	})
}

// Generate handles generating a quadlet from an existing container or pod
func (h *QuadletHandler) Generate(w http.ResponseWriter, r *http.Request) {
	web.Handle(w, r, http.StatusOK, func(ctx context.Context, userID, machineID string) (*QuadletGenerateResponse, error) {
		var request QuadletGenerateRequest
		if appErr := web.Decode(r, &request); appErr != nil {
			return nil, appErr
		}

		return h.service.Generate(ctx, userID, machineID, request)
	})
}

// Import handles importing quadlets from uploaded content
func (h *QuadletHandler) Import(w http.ResponseWriter, r *http.Request) {
	web.Handle(w, r, http.StatusCreated, func(ctx context.Context, userID, machineID string) (*QuadletImportResponse, error) {
		var request QuadletImportRequest
		if appErr := web.Decode(r, &request); appErr != nil {
			return nil, appErr
		}

		return h.service.Import(ctx, userID, machineID, request)
	})
}

// AutoUpdate handles running `podman auto-update` on the machine with live
// NDJSON streaming of remote output, terminating in a parsed report event.
// Registry pulls make runs routinely exceed the global 30-second write
// timeout, so this handler extends its own per-response write deadline to
// 10 minutes without touching the server-wide setting. An operation-duration
// fact like that stays handler-side; the streaming envelope (Begin + Run)
// owns everything else.
func (h *QuadletHandler) AutoUpdate(w http.ResponseWriter, r *http.Request) {
	dryRun := web.QueryBool(r, "dryRun", false)

	operationID := uuid.New().String()
	slog.InfoContext(r.Context(), "AutoUpdate quadlets",
		slog.String("requestID", contextutils.GetRequestID(r.Context())),
		slog.String("machineID", chi.URLParam(r, "id")),
		slog.String("operationID", operationID),
		slog.Bool("dryRun", dryRun))

	if err := http.NewResponseController(w).SetWriteDeadline(time.Now().Add(10 * time.Minute)); err != nil {
		slog.WarnContext(r.Context(), "Failed to extend write deadline", slog.Any("error", err))
	}

	stream := streaming.Begin(w, r)
	if stream == nil {
		return
	}

	streaming.Run(r.Context(), stream.Writer, streaming.StreamConfig[AutoUpdateEvent, AutoUpdateReport]{
		Operation: func(ctx context.Context, onProgress func(AutoUpdateEvent)) (AutoUpdateReport, error) {
			return h.service.AutoUpdate(ctx, stream.UserID, stream.MachineID, dryRun, func(line sshexec.OutputLine) {
				onProgress(NewAutoUpdateLogEvent(operationID, stream.MachineID, line))
			})
		},
		OnEvent: func(event AutoUpdateEvent) error {
			return stream.Writer.Write(event)
		},
		CompleteEvent: func(report AutoUpdateReport) any {
			return NewAutoUpdateCompleteEvent(operationID, stream.MachineID, report)
		},
		ErrorDefault: errors.ErrConnectionFailed,
		ErrorEvent: func(code, message string, _ bool) any {
			return NewAutoUpdateErrorEvent(operationID, stream.MachineID, code, message)
		},
	})
}
