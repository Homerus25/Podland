package quadlets

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	appErrors "github.com/Homerus25/podland/backend/internal/errors"
)

// GeneratedQuadletFile is one file the compose converter produced: the
// file name (with its resource extension, e.g. "web.container") and the
// file's text.
type GeneratedQuadletFile struct {
	FileName string
	Content  string
}

// ComposeConverter is the consumer-declared seam behind the
// compose-to-quadlet conversion (spec #336): the service depends on this
// interface and never on os/exec. The production adapter
// (podletComposeConverter below) shells out to podlet; tests drive the
// service over a function-field fake.
type ComposeConverter interface {
	// Convert runs the conversion over the compose file content and
	// returns the generated quadlet files. A conversion rejection — the
	// external converter refusing the compose file — is returned as a
	// *ConversionError carrying the converter's stderr.
	Convert(ctx context.Context, composeContent string) ([]GeneratedQuadletFile, error)
}

// ConversionError is the converter's rejection of the compose content: the
// external converter exited non-zero and Stderr carries its error output
// verbatim (ANSI color escapes included — the service strips them when it
// builds the wire message).
type ConversionError struct {
	Stderr string
}

func (e *ConversionError) Error() string {
	return e.Stderr
}

// ConvertCompose converts compose file content into quadlet entries (spec
// #336). The conversion runs behind the ComposeConverter seam — the
// service never touches exec — and the generated files are mapped onto
// the import endpoint's entry shape so the frontend can forward the
// response unchanged. A converter rejection maps to 400 VALIDATION_FAILED
// carrying the converter's stderr (ANSI color escapes stripped, the text
// otherwise verbatim); any other converter failure propagates to the
// envelope's 500 fallback. A nil converter is a wiring bug and fails
// loudly with 500.
func (s *QuadletService) ConvertCompose(ctx context.Context, req QuadletConvertComposeRequest) (*QuadletConvertComposeResponse, error) {
	if s.composeConverter == nil {
		slog.ErrorContext(ctx, "quadlets.ConvertCompose: compose converter not wired")
		return nil, appErrors.ErrInternalError
	}

	files, err := s.composeConverter.Convert(ctx, req.Content)
	if err != nil {
		var convErr *ConversionError
		if errors.As(err, &convErr) {
			return nil, appErrors.ErrValidationFailed.WithMessage(stripANSIColors(convErr.Stderr))
		}
		return nil, err
	}

	return &QuadletConvertComposeResponse{Quadlets: toImportEntries(files)}, nil
}

// toImportEntries maps the converter's generated files onto the import
// endpoint's entry shape: name is the file name without extension,
// resource is derived from the extension through the domain's existing
// vocabulary (deriveResourceFromPath), content is the file text. Entries
// are sorted by name (resource as tie-break) so the response is
// deterministic whatever order the converter produced the files in.
func toImportEntries(files []GeneratedQuadletFile) []QuadletImportEntry {
	entries := make([]QuadletImportEntry, 0, len(files))
	for _, file := range files {
		ext := filepath.Ext(file.FileName)
		entries = append(entries, QuadletImportEntry{
			Name:     strings.TrimSuffix(file.FileName, ext),
			Resource: deriveResourceFromPath(file.FileName),
			Content:  file.Content,
		})
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].Name != entries[j].Name {
			return entries[i].Name < entries[j].Name
		}
		return entries[i].Resource < entries[j].Resource
	})
	return entries
}

// ansiColorPattern matches ANSI SGR color escape sequences — the terminal
// control codes podlet's error output carries. They are terminal
// formatting, not message text, so the conversion error message strips
// them; everything else in the converter's stderr survives verbatim
// (pinned decision, spec #336).
var ansiColorPattern = regexp.MustCompile("\x1b\\[[0-9;]*m")

// stripANSIColors removes ANSI SGR color escape sequences from text.
func stripANSIColors(text string) string {
	return ansiColorPattern.ReplaceAllString(text, "")
}

// podletBinary is the converter executable, resolved through PATH.
const podletBinary = "podlet"

// podletComposeConverter is the production ComposeConverter (spec #336): a
// thin exec adapter over the podlet binary. It runs `podlet --file <dir>
// compose` — --file is a GLOBAL podlet option and goes before the
// subcommand (verified podlet 0.3.2 contract) — with the compose content
// piped via stdin, then reads the generated quadlet files back from the
// directory. Default mode only (no --pod/--kube); relative host paths are
// left untouched.
type podletComposeConverter struct{}

// NewPodletComposeConverter returns the production ComposeConverter backed
// by the podlet binary resolved through PATH.
func NewPodletComposeConverter() ComposeConverter {
	return &podletComposeConverter{}
}

func (c *podletComposeConverter) Convert(ctx context.Context, composeContent string) ([]GeneratedQuadletFile, error) {
	dir, err := os.MkdirTemp("", "podland-convert-compose-")
	if err != nil {
		return nil, fmt.Errorf("failed to create compose conversion directory: %w", err)
	}
	defer os.RemoveAll(dir)

	cmd := exec.CommandContext(ctx, podletBinary, "--file", dir, "compose")
	cmd.Stdin = strings.NewReader(composeContent)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, classifyRunError(err, stderr.String())
	}

	dirEntries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("failed to read converted quadlets: %w", err)
	}
	files := make([]GeneratedQuadletFile, 0, len(dirEntries))
	for _, entry := range dirEntries {
		if entry.IsDir() {
			continue
		}
		content, err := os.ReadFile(filepath.Join(dir, entry.Name()))
		if err != nil {
			return nil, fmt.Errorf("failed to read converted quadlet %s: %w", entry.Name(), err)
		}
		files = append(files, GeneratedQuadletFile{FileName: entry.Name(), Content: string(content)})
	}
	return files, nil
}

// classifyRunError sorts a failed podlet run into its two error families
// (pentest 2026-11-01, finding 7). A process that started and exited
// non-zero (*exec.ExitError) is a genuine conversion rejection: it maps to
// *ConversionError so the service surfaces podlet's stderr as 400
// VALIDATION_FAILED, with an empty stderr falling back to the exit error
// so the message is never blank. Any other Run error — the binary missing
// from PATH (exec.ErrNotFound), permission denied, fork failures, a
// canceled context — never started converting, so it is a server-side
// problem: it comes back wrapped as a non-ConversionError, the service
// propagates it untouched, and the envelope answers the generic 500 with
// the raw exec text only in the server log.
func classifyRunError(runErr error, stderrText string) error {
	var exitErr *exec.ExitError
	if errors.As(runErr, &exitErr) {
		if strings.TrimSpace(stderrText) == "" {
			stderrText = runErr.Error()
		}
		return &ConversionError{Stderr: stderrText}
	}
	return fmt.Errorf("failed to start podlet: %w", runErr)
}
