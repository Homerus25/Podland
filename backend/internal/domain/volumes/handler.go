package volumes

import (
	"archive/tar"
	"context"
	stderrors "errors"
	"fmt"
	"io"
	"log/slog"
	"mime"
	"mime/multipart"
	"net/http"
	"os"
	"path"
	"strings"

	"github.com/Homerus25/podland/backend/internal/contextutils"
	"github.com/Homerus25/podland/backend/internal/errors"
	"github.com/Homerus25/podland/backend/internal/streaming"
	"github.com/Homerus25/podland/backend/internal/transfer"
	"github.com/Homerus25/podland/backend/internal/web"
	"go.podman.io/podman/v6/pkg/domain/entities"
)

// VolumeHandler handles volume requests
type VolumeHandler struct {
	service *VolumeService
	// maxUploadBytes bounds populate uploads; enforced with the
	// max-bytes reader while streaming parts to disk.
	maxUploadBytes int64
}

// NewVolumeHandler creates a new VolumeHandler. The populate upload
// cap's unwired default is the transfer kit's DefaultMaxVolumeUploadBytes
// (2 GiB, matching VOLUME_UPLOAD_MAX_BYTES) — the kit owns the bound's
// default (§19).
func NewVolumeHandler(service *VolumeService) *VolumeHandler {
	return &VolumeHandler{
		service:        service,
		maxUploadBytes: transfer.DefaultMaxVolumeUploadBytes,
	}
}

// WithMaxUploadBytes sets the populate upload cap and returns the
// handler for chaining.
func (h *VolumeHandler) WithMaxUploadBytes(max int64) *VolumeHandler {
	if max > 0 {
		h.maxUploadBytes = max
	}
	return h
}

// List handles listing volumes on a machine. Route-level
// RequireMachineAccess (app.go) gates the subtree; the endpoint
// envelope owns extraction, error mapping, and encoding.
func (h *VolumeHandler) List(w http.ResponseWriter, r *http.Request) {
	web.Handle(w, r, http.StatusOK, func(ctx context.Context, userID, machineID string) ([]VolumeResponse, error) {
		volumes, err := h.service.List(ctx, userID, machineID, entities.VolumeListOptions{})
		if err != nil {
			return nil, err
		}
		return ToVolumeListResponse(volumes), nil
	})
}

// Get handles inspecting a single volume on a machine
func (h *VolumeHandler) Get(w http.ResponseWriter, r *http.Request) {
	web.Handle(w, r, http.StatusOK, func(ctx context.Context, userID, machineID string) (VolumeInspectResponse, error) {
		volumeName, guard := web.URLParam(r, "volumeName", "MISSING_VOLUME_NAME")
		if guard != nil {
			return VolumeInspectResponse{}, guard
		}

		report, err := h.service.Get(ctx, userID, machineID, volumeName)
		if err != nil {
			return VolumeInspectResponse{}, err
		}
		return FromPodmanVolumeInspect(report), nil
	})
}

// Create handles creating a volume on a machine. Route-level
// RequireMachinePermission(write) (app.go) gates it at write. The body is
// optional in shape: an absent/empty name creates an anonymous volume.
// Name-format validation is podman's job; its rejections surface as 400
// INVALID_REQUEST from the service.
func (h *VolumeHandler) Create(w http.ResponseWriter, r *http.Request) {
	web.Handle(w, r, http.StatusCreated, func(ctx context.Context, userID, machineID string) (VolumeInspectResponse, error) {
		var request VolumeCreateRequest
		if appErr := web.Decode(w, r, &request); appErr != nil {
			return VolumeInspectResponse{}, appErr
		}

		report, err := h.service.Create(ctx, userID, machineID, request)
		if err != nil {
			return VolumeInspectResponse{}, err
		}
		return FromPodmanVolumeInspect(report), nil
	})
}

// Delete handles removing a volume by exact name. Route-level
// RequireMachinePermission(write) (app.go) gates it at write. The force
// query flag (default false) lets podman stop dependent containers and
// remove the volume anyway; without it an in-use volume surfaces as 409
// VOLUME_IN_USE. Returns 204 with an empty body.
func (h *VolumeHandler) Delete(w http.ResponseWriter, r *http.Request) {
	web.Handle(w, r, http.StatusNoContent, func(ctx context.Context, userID, machineID string) (struct{}, error) {
		volumeName, guard := web.URLParam(r, "volumeName", "MISSING_VOLUME_NAME")
		if guard != nil {
			return struct{}{}, guard
		}

		force := web.QueryBool(r, "force", false)

		return struct{}{}, h.service.Delete(ctx, userID, machineID, volumeName, force)
	})
}

// Prune handles removing all unused volumes on a machine. Route-level
// RequireMachinePermission(write) (app.go) gates it at write; podman
// leaves in-use volumes alone. Returns 200 with one report entry per
// removed volume (name, reclaimed size, any per-volume error).
func (h *VolumeHandler) Prune(w http.ResponseWriter, r *http.Request) {
	web.Handle(w, r, http.StatusOK, func(ctx context.Context, userID, machineID string) ([]VolumePruneResponse, error) {
		prunes, err := h.service.Prune(ctx, userID, machineID)
		if err != nil {
			return nil, err
		}
		return FromPodmanPruneReports(prunes), nil
	})
}

// Export handles streaming a volume's contents as a tar download.
// Read-level permission suffices (subtree RequireMachineAccess): users
// who can read a machine's state can read its data. It crosses the
// endpoint envelope's binary twin (web.HandleStream): identity guards,
// AppError mapping, and the 500 fallback live there; the closure is tar
// headers plus the streaming service call. Headers are set before the
// service streams, so the download starts with the right
// Content-Type/Disposition; a 404/4xx happens before any byte is
// written (the service resolves existence first), so error envelopes
// stay intact — the envelope's error write replaces the tar
// Content-Type. Mid-stream failures can't retroactively change the
// status — the connection just truncates.
func (h *VolumeHandler) Export(w http.ResponseWriter, r *http.Request) {
	web.HandleStream(w, r, func(ctx context.Context, userID, machineID string) error {
		volumeName, guard := web.URLParam(r, "volumeName", "MISSING_VOLUME_NAME")
		if guard != nil {
			return guard
		}

		w.Header().Set("Content-Type", "application/x-tar")
		w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", volumeName+".tar"))

		return h.service.Export(ctx, userID, machineID, volumeName, w)
	})
}

// Populate handles filling a volume from an upload. The body is
// multipart/form-data in exactly one of two shapes:
//
//   - one or more "files" parts: each part's filename is the relative
//     path inside the volume; the backend assembles a tar on disk
//   - exactly one "archive" part: a ready-made tar, imported as-is
//
// Sending both shapes, or neither, is a 400. Semantics are
// podman-native MERGE: uploaded paths overwrite same-named files,
// everything else in the volume is kept — there is NO replace mode.
//
// Parts are streamed to temp files (never buffered in RAM); total
// upload size is capped by VOLUME_UPLOAD_MAX_BYTES (default 2 GiB) via
// the max-bytes reader, and exceeding it yields 413
// VOLUME_UPLOAD_TOO_LARGE. Temp files are removed on every path.
func (h *VolumeHandler) Populate(w http.ResponseWriter, r *http.Request) {
	web.HandleStream(w, r, func(ctx context.Context, userID, machineID string) error {
		volumeName, guard := web.URLParam(r, "volumeName", "MISSING_VOLUME_NAME")
		if guard != nil {
			return guard
		}

		r.Body = http.MaxBytesReader(w, r.Body, h.maxUploadBytes)
		mr, err := r.MultipartReader()
		if err != nil {
			return errors.ErrInvalidRequest.WithMessage("body must be multipart/form-data")
		}

		payload, err := h.collectUpload(mr)
		if payload != nil {
			// Every temp file is removed on every path once the handler is
			// done with it.
			defer payload.cleanup()
		}
		if err != nil {
			return err
		}

		if err := h.service.Import(ctx, userID, machineID, volumeName, payload.reader()); err != nil {
			return err
		}

		w.WriteHeader(http.StatusNoContent)
		return nil
	})
}

// MovePreview answers the copy/move dialog's facts (issue 238,
// doku/description_volumes_api.md): collision, in-use, referencing
// quadlets. Route-level RequireMachineAccess (app.go) gates the subtree
// at read; the target machine's read access is enforced inside the
// service against the access store.
func (h *VolumeHandler) MovePreview(w http.ResponseWriter, r *http.Request) {
	web.Handle(w, r, http.StatusOK, func(ctx context.Context, userID, machineID string) (*VolumeMovePreviewResponse, error) {
		volumeName, guard := web.URLParam(r, "volumeName", "MISSING_VOLUME_NAME")
		if guard != nil {
			return nil, guard
		}

		targetMachineID, guard := web.QueryString(r, "targetMachineId", "MISSING_TARGET_MACHINE_ID")
		if guard != nil {
			return nil, guard
		}

		return h.service.MovePreview(ctx, userID, machineID, volumeName, targetMachineID)
	})
}

// Move handles the cross-machine copy/move transfer of a volume (issue
// 238, doku/description_volumes_api.md). The route sits at the
// subtree's write gate for the SOURCE machine; the target machine's
// write-level access is enforced inside the service against the access
// store.
//
// Transport: ALWAYS NDJSON — no plain/stream fork, one code path with
// progress for free, through the streaming kit's progress seam
// (HandleProgress). The data phase can run minutes on real volumes, so
// the caller gets progress events and one terminal event (complete
// envelope or mapped error); the kit's 10-minute write deadline covers
// the data phase's routine excess over the global 30-second write
// timeout (the registry-pull precedent). Errors answered BEFORE the
// stream begins (bad param, bad body) keep the plain envelope.
func (h *VolumeHandler) Move(w http.ResponseWriter, r *http.Request) {
	volumeName, guard := web.URLParam(r, "volumeName", "MISSING_VOLUME_NAME")
	if guard != nil {
		errors.RespondWithError(w, guard, contextutils.GetRequestID(r.Context()))
		return
	}

	// Decode answers its 400s before the stream begins; the streaming
	// kit's progress seam takes over from there (the quadlet move's
	// arrangement).
	var request VolumeMoveRequest
	if appErr := web.Decode(w, r, &request); appErr != nil {
		errors.RespondWithError(w, appErr, contextutils.GetRequestID(r.Context()))
		return
	}

	streaming.HandleProgress(w, r, "Move volume across machines", []slog.Attr{slog.String("volumeName", volumeName)}, streaming.ProgressConfig[VolumeMoveEvent, *VolumeMoveResponse]{
		Operation: func(ctx context.Context, op streaming.OperationContext, onProgress func(VolumeMoveEvent)) (*VolumeMoveResponse, error) {
			return h.service.Move(ctx, op.UserID, op.MachineID, volumeName, request, func(event VolumeMoveEvent) {
				event.OperationID = op.OperationID
				event.MachineID = op.MachineID
				onProgress(event)
			})
		},
		CompleteEvent: func(op streaming.OperationContext, response *VolumeMoveResponse) any {
			return NewVolumeMoveCompleteEvent(op.OperationID, op.MachineID, *response)
		},
		ErrorDefault: errors.ErrConnectionFailed,
		ErrorEvent: func(op streaming.OperationContext, code, message string, _ bool) any {
			return NewVolumeMoveErrorEvent(op.OperationID, op.MachineID, code, message)
		},
	})
}

// spooledFile is one part streamed to a temp file: its spool and the
// byte count written. The spool is removed by the payload's cleanup.
type spooledFile struct {
	spool *os.File
	size  int64
}

// uploadPayload is the assembled import source in one of the two
// shapes: a list of spooled files to tar up (files shape) or a single
// spooled tar (archive shape).
type uploadPayload struct {
	files     []spooledFile
	archive   *os.File
	spools    []*spooledFile
	assembled *os.File
}

// reader returns the import source.
func (p *uploadPayload) reader() io.Reader {
	if p.assembled != nil {
		p.assembled.Seek(0, io.SeekStart)
		return p.assembled
	}
	p.archive.Seek(0, io.SeekStart)
	return p.archive
}

// cleanup removes every temp file on the path, success or failure.
func (p *uploadPayload) cleanup() {
	for _, f := range p.spools {
		f.spool.Close()
		os.Remove(f.spool.Name())
	}
	if p.assembled != nil {
		p.assembled.Close()
		os.Remove(p.assembled.Name())
	}
}

// collectUpload streams the multipart body to temp files and resolves
// the shape. Both shapes at once, or neither, is a client error; the
// max-bytes reader's MaxBytesError is folded into the 413 sentinel here,
// so collectUpload's client errors all leave as AppErrors — anything
// else rides the envelope's 500 fallback.
func (h *VolumeHandler) collectUpload(mr *multipart.Reader) (*uploadPayload, error) {
	payload := &uploadPayload{}
	var tarWriter *tar.Writer

	for {
		part, err := mr.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			return payload, uploadError(err)
		}

		switch part.FormName() {
		case "files":
			if payload.archive != nil {
				return payload, errors.ErrInvalidRequest.WithMessage("send either files or archive, not both")
			}
			name, err := rawPartFilename(part)
			if err != nil {
				return payload, err
			}
			name, err = safeArchivePath(name)
			if err != nil {
				return payload, err
			}
			spooled, err := spoolPart(part)
			if err != nil {
				return payload, err
			}
			payload.spools = append(payload.spools, spooled)
			payload.files = append(payload.files, *spooled)

			// Lazily open the assembled tar over a temp file.
			if tarWriter == nil {
				assembled, err := os.CreateTemp("", "podland-populate-*.tar")
				if err != nil {
					return payload, err
				}
				payload.assembled = assembled
				tarWriter = tar.NewWriter(assembled)
			}
			if err := appendSpooledToTar(tarWriter, name, spooled); err != nil {
				return payload, err
			}
		case "archive":
			if payload.archive != nil || len(payload.files) > 0 {
				return payload, errors.ErrInvalidRequest.WithMessage("send either files or archive, not both")
			}
			spooled, err := spoolPart(part)
			if err != nil {
				return payload, err
			}
			payload.spools = append(payload.spools, spooled)
			payload.archive = spooled.spool
		}
		// Other part names are ignored: shape is decided by the named parts.
	}

	if payload.archive == nil && len(payload.files) == 0 {
		return payload, errors.ErrInvalidRequest.WithMessage("upload must contain either files parts or exactly one archive part")
	}
	if tarWriter != nil {
		if err := tarWriter.Close(); err != nil {
			return payload, err
		}
	}
	return payload, nil
}

// uploadError folds the max-bytes reader's MaxBytesError into the 413
// sentinel; anything else passes through unchanged.
func uploadError(err error) error {
	var maxBytesErr *http.MaxBytesError
	if stderrors.As(err, &maxBytesErr) {
		return errors.ErrVolumeUploadTooLarge
	}
	return err
}

// spoolPart streams one part to a temp file. Nothing is buffered in
// RAM; the cap is enforced by the max-bytes reader wrapping the body.
func spoolPart(part *multipart.Part) (*spooledFile, error) {
	spool, err := os.CreateTemp("", "podland-part-*")
	if err != nil {
		return nil, err
	}
	size, err := io.Copy(spool, part)
	if err != nil {
		spool.Close()
		os.Remove(spool.Name())
		return nil, uploadError(err)
	}
	return &spooledFile{spool: spool, size: size}, nil
}

// appendSpooledToTar writes one spooled file into the assembled tar
// under the given relative path.
func appendSpooledToTar(tw *tar.Writer, name string, spooled *spooledFile) error {
	if err := tw.WriteHeader(&tar.Header{
		Typeflag: tar.TypeReg,
		Name:     name,
		Size:     spooled.size,
		Mode:     0o644,
	}); err != nil {
		return err
	}
	if _, err := spooled.spool.Seek(0, io.SeekStart); err != nil {
		return err
	}
	_, err := io.Copy(tw, spooled.spool)
	return err
}

// rawPartFilename extracts the filename parameter straight from the
// part's Content-Disposition header. mime/multipart's FileName() runs
// filepath.Base on it, which would destroy the relative paths the
// files shape must preserve — so the raw value is parsed here instead.
func rawPartFilename(part *multipart.Part) (string, error) {
	_, params, err := mime.ParseMediaType(part.Header.Get("Content-Disposition"))
	if err != nil {
		return "", err
	}
	return params["filename"], nil
}

// safeArchivePath validates the relative path a files part carries:
// absolute paths and ".." traversal are rejected so uploads stay inside
// the volume.
func safeArchivePath(name string) (string, error) {
	if name == "" {
		return "", errors.ErrInvalidRequest.WithMessage("files parts must carry a filename")
	}
	for _, segment := range strings.Split(name, "/") {
		if segment == ".." {
			return "", errors.ErrInvalidRequest.WithMessage("file paths must be relative and stay inside the volume")
		}
	}
	cleaned := path.Clean("/" + name)
	cleaned = strings.TrimPrefix(cleaned, "/")
	if cleaned == "" || cleaned == "." {
		return "", errors.ErrInvalidRequest.WithMessage("file paths must be relative and stay inside the volume")
	}
	return cleaned, nil
}
