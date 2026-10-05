package transfer

import (
	"archive/tar"
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeVolumeSource hands the splice a tar of the given path→content
// entries; exportErr simulates an export failure.
type fakeVolumeSource struct {
	entries   map[string][]byte
	exportErr error
}

func (f *fakeVolumeSource) VolumeExport(name string, w io.Writer) error {
	if f.exportErr != nil {
		return f.exportErr
	}
	tw := tar.NewWriter(w)
	_ = tw.WriteHeader(&tar.Header{Typeflag: tar.TypeDir, Name: name})
	for path, content := range f.entries {
		_ = tw.WriteHeader(&tar.Header{Typeflag: tar.TypeReg, Name: path, Size: int64(len(content)), Mode: 0o644})
		_, _ = tw.Write(content)
	}
	return tw.Close()
}

// fakeVolumeTarget consumes the import, recording the raw byte count and
// the entries it received; importErr simulates an import failure after
// reading everything.
type fakeVolumeTarget struct {
	rawLen    int64
	imported  map[string][]byte
	importErr error
}

func (f *fakeVolumeTarget) VolumeImport(name string, r io.Reader) error {
	raw, err := io.ReadAll(r)
	if err != nil {
		return err
	}
	f.rawLen = int64(len(raw))
	f.imported = map[string][]byte{}
	tr := tar.NewReader(bytes.NewReader(raw))
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		if hdr.Typeflag == tar.TypeReg {
			data, _ := io.ReadAll(tr)
			f.imported[hdr.Name] = data
		}
	}
	return f.importErr
}

func TestCopyVolumeData_CarriesEntriesAndCountsBytes(t *testing.T) {
	entries := map[string][]byte{
		"pg/PG_VERSION": []byte("17"),
		"pg/note.txt":   bytes.Repeat([]byte("a"), 3<<20),
	}
	src := &fakeVolumeSource{entries: entries}
	tgt := &fakeVolumeTarget{}

	var reports []int64
	streamed, err := CopyVolumeData(context.Background(), src, tgt, "vol", 0, func(count int64) { reports = append(reports, count) })

	require.NoError(t, err)
	assert.Equal(t, tgt.rawLen, streamed, "the count is the tar bytes actually streamed")
	assert.Equal(t, entries, tgt.imported)
	// The cadence is at most one report per MiB and a single pipe write can
	// jump past a boundary, so the exact report count depends on how the
	// reader chunks the stream — assert the contract, not the chunking.
	require.NotEmpty(t, reports, "a 3 MiB stream reports progress")
	for i := 1; i < len(reports); i++ {
		assert.Greater(t, reports[i], reports[i-1], "reports are strictly increasing")
	}
	assert.GreaterOrEqual(t, reports[0], int64(1<<20), "the first report waits for a full MiB")
}

// rawChunkSource writes exact 1 MiB chunks with no tar framing, so every
// pipe write lands as exactly one 1 MiB read and the counting reader's
// cadence is observable exactly.
type rawChunkSource struct{ chunks int }

func (s *rawChunkSource) VolumeExport(name string, w io.Writer) error {
	chunk := bytes.Repeat([]byte("x"), 1<<20)
	for i := 0; i < s.chunks; i++ {
		if _, err := w.Write(chunk); err != nil {
			return err
		}
	}
	return nil
}

// rawChunkTarget reads with a 1 MiB buffer: one Read per pipe write.
type rawChunkTarget struct{ total int64 }

func (t *rawChunkTarget) VolumeImport(name string, r io.Reader) error {
	buf := make([]byte, 1<<20)
	for {
		n, err := r.Read(buf)
		t.total += int64(n)
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
	}
}

func TestCopyVolumeData_ReportsOncePerMiBStreamed(t *testing.T) {
	src := &rawChunkSource{chunks: 3}
	tgt := &rawChunkTarget{}

	var reports []int64
	streamed, err := CopyVolumeData(context.Background(), src, tgt, "vol", 0, func(count int64) { reports = append(reports, count) })

	require.NoError(t, err)
	assert.Equal(t, int64(3<<20), streamed)
	assert.Equal(t, int64(3<<20), tgt.total)
	assert.Equal(t, []int64{1 << 20, 2 << 20, 3 << 20}, reports, "one report per MiB actually streamed")
}

func TestCopyVolumeData_BoundTrips(t *testing.T) {
	src := &fakeVolumeSource{entries: map[string][]byte{"big.bin": bytes.Repeat([]byte("x"), 1<<20)}}
	tgt := &fakeVolumeTarget{}

	streamed, err := CopyVolumeData(context.Background(), src, tgt, "vol", 1<<20, nil)

	assert.ErrorIs(t, err, ErrBoundExceeded)
	assert.Greater(t, streamed, int64(1<<20), "the count reports how far the stream reached")
}

func TestCopyVolumeData_ZeroLimitNeverTrips(t *testing.T) {
	src := &fakeVolumeSource{entries: map[string][]byte{"a": []byte("x")}}
	tgt := &fakeVolumeTarget{}

	_, err := CopyVolumeData(context.Background(), src, tgt, "vol", 0, nil)

	require.NoError(t, err)
}

func TestCopyVolumeData_ImportFailureComesBackUnwrapped(t *testing.T) {
	cause := errors.New("import refused")
	src := &fakeVolumeSource{entries: map[string][]byte{"a": []byte("x")}}
	tgt := &fakeVolumeTarget{importErr: cause}

	_, err := CopyVolumeData(context.Background(), src, tgt, "vol", 0, nil)

	assert.ErrorIs(t, err, cause)
}

func TestCopyVolumeData_ExportFailureComesBackUnwrapped(t *testing.T) {
	cause := errors.New("export refused")
	src := &fakeVolumeSource{exportErr: cause}
	tgt := &fakeVolumeTarget{}

	_, err := CopyVolumeData(context.Background(), src, tgt, "vol", 0, nil)

	assert.ErrorIs(t, err, cause)
}

// parkedSource streams one MiB, then parks until its context is done: the
// next write must fail on the pipe the AfterFunc closed — a mid-stream
// disconnect, the deterministic stand-in for a pre-cancelled context (which
// races the AfterFunc goroutine against a tiny stream).
type parkedSource struct{ ctx context.Context }

func (p *parkedSource) VolumeExport(name string, w io.Writer) error {
	chunk := bytes.Repeat([]byte("x"), 1<<20)
	if _, err := w.Write(chunk); err != nil {
		return err
	}
	<-p.ctx.Done()
	_, err := w.Write(chunk)
	return err
}

func TestCopyVolumeData_CancelledContextWins(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	src := &parkedSource{ctx: ctx}
	tgt := &rawChunkTarget{}

	firstReport := make(chan int64, 1)
	done := make(chan error, 1)
	go func() {
		_, err := CopyVolumeData(ctx, src, tgt, "vol", 0, func(count int64) {
			select {
			case firstReport <- count:
			default:
			}
		})
		done <- err
	}()

	<-firstReport // the first MiB crossed the splice; the source is parked
	cancel()

	select {
	case err := <-done:
		assert.ErrorIs(t, err, context.Canceled)
	case <-time.After(5 * time.Second):
		t.Fatal("the cancelled stream did not finish promptly")
	}
}

func TestProbeVolumeEmpty_EmptyVolumeIsTrue(t *testing.T) {
	// Directory-only tar: the always-framed volume root, no content.
	src := &fakeVolumeSource{entries: map[string][]byte{}}

	empty, err := ProbeVolumeEmpty(src, "vol")

	require.NoError(t, err)
	assert.True(t, empty)
}

func TestProbeVolumeEmpty_ContentIsFalse(t *testing.T) {
	src := &fakeVolumeSource{entries: map[string][]byte{"data.txt": []byte("x")}}

	empty, err := ProbeVolumeEmpty(src, "vol")

	require.NoError(t, err)
	assert.False(t, empty)
}

func TestProbeVolumeEmpty_ExportFailureNamesTheVolume(t *testing.T) {
	cause := errors.New("socket gone")
	src := &fakeVolumeSource{exportErr: cause}

	_, err := ProbeVolumeEmpty(src, "vol")

	require.Error(t, err)
	assert.ErrorIs(t, err, cause)
	assert.Contains(t, err.Error(), "vol")
}

// blockingSource writes a content entry and then parks on the pipe: the
// 1 MiB write cannot complete until someone reads it. The probe must
// close the pipe mid-stream so the write fails and the goroutine ends —
// the bounded-read contract.
type blockingSource struct{}

func (b *blockingSource) VolumeExport(name string, w io.Writer) error {
	tw := tar.NewWriter(w)
	_ = tw.WriteHeader(&tar.Header{Typeflag: tar.TypeDir, Name: name})
	_ = tw.WriteHeader(&tar.Header{Typeflag: tar.TypeReg, Name: "content.bin", Size: 1 << 20, Mode: 0o644})
	_, err := w.Write(bytes.Repeat([]byte("x"), 1<<20))
	return err
}

func TestProbeVolumeEmpty_StopsReadingMidStream(t *testing.T) {
	empty, err := ProbeVolumeEmpty(&blockingSource{}, "vol")

	require.NoError(t, err)
	assert.False(t, empty)
}

// --- secrets -----------------------------------------------------------

// fakeSecretSource reveals values; missing names answer a podman-shaped
// not-found error.
type fakeSecretSource struct {
	values map[string]string
}

func (f *fakeSecretSource) Reveal(name string) (string, error) {
	if value, ok := f.values[name]; ok {
		return value, nil
	}
	return "", errors.New("secret not found: no secret named " + name)
}

// fakeSecretTarget records creates; Has answers membership over what it
// holds.
type fakeSecretTarget struct {
	names  []string
	values map[string]string
}

func (f *fakeSecretTarget) Has(name string) (bool, error) {
	for _, existing := range f.names {
		if existing == name {
			return true, nil
		}
	}
	return false, nil
}

func (f *fakeSecretTarget) Create(name, value string) error {
	if f.values == nil {
		f.values = map[string]string{}
	}
	f.names = append(f.names, name)
	f.values[name] = value
	return nil
}

func TestTransferSecrets_HappyPathSkipsAndTransfers(t *testing.T) {
	src := &fakeSecretSource{values: map[string]string{"db": "v1", "tls": "v2"}}
	tgt := &fakeSecretTarget{names: []string{"existing"}}

	outcomes, err := TransferSecrets(src, tgt, []SecretRef{
		{Name: "db"},
		{Name: "existing"},
		{Name: "dangling"},
		{Name: "/etc/host.pem", HostPath: true},
		{Name: "tls"},
		{Name: "db"}, // duplicate: first reference wins
	})

	require.NoError(t, err)
	require.Len(t, outcomes, 5)
	assert.Equal(t, SecretOutcome{Name: "db", Outcome: SecretOutcomeTransferred}, outcomes[0])
	assert.Equal(t, SecretOutcome{Name: "existing", Outcome: SecretOutcomeSkipped, Reason: SecretSkipOnTarget}, outcomes[1])
	assert.Equal(t, SecretOutcome{Name: "dangling", Outcome: SecretOutcomeSkipped, Reason: SecretSkipDangling}, outcomes[2])
	assert.Equal(t, SecretOutcome{Name: "/etc/host.pem", Outcome: SecretOutcomeSkipped, Reason: SecretSkipHostPath}, outcomes[3])
	assert.Equal(t, SecretOutcome{Name: "tls", Outcome: SecretOutcomeTransferred}, outcomes[4])
	assert.Equal(t, map[string]string{"db": "v1", "tls": "v2"}, tgt.values)
}

func TestTransferSecrets_SameNameSkipHappensBeforeTheReveal(t *testing.T) {
	// The source would fail every reveal; the on-target skip must never
	// dial it.
	src := &failingSecretSource{}
	tgt := &fakeSecretTarget{names: []string{"existing"}}

	outcomes, err := TransferSecrets(src, tgt, []SecretRef{{Name: "existing"}})

	require.NoError(t, err)
	require.Len(t, outcomes, 1)
	assert.Equal(t, SecretOutcomeSkipped, outcomes[0].Outcome)
}

type failingSecretSource struct{}

func (f *failingSecretSource) Reveal(name string) (string, error) {
	return "", errors.New("reveal must not be called")
}

func TestTransferSecrets_RevealFailureIsSecretError(t *testing.T) {
	cause := errors.New("store unreachable")
	src := &failingRevealSource{err: cause}
	tgt := &fakeSecretTarget{}

	_, err := TransferSecrets(src, tgt, []SecretRef{{Name: "db"}})

	require.Error(t, err)
	secretErr, ok := err.(*SecretError)
	require.True(t, ok, "the failure is a *SecretError")
	assert.Equal(t, "db", secretErr.Name)
	assert.ErrorIs(t, secretErr, cause)
	assert.Equal(t, "Secret db: store unreachable", err.Error())
}

type failingRevealSource struct{ err error }

func (f *failingRevealSource) Reveal(name string) (string, error) { return "", f.err }

func TestTransferSecrets_CreateFailureIsSecretError(t *testing.T) {
	cause := errors.New("create refused")
	src := &fakeSecretSource{values: map[string]string{"db": "v1", "tls": "v2"}}
	tgt := &failingCreateTarget{err: cause, failName: "tls"}

	outcomes, err := TransferSecrets(src, tgt, []SecretRef{{Name: "db"}, {Name: "tls"}})

	require.Error(t, err)
	secretErr, ok := err.(*SecretError)
	require.True(t, ok)
	assert.Equal(t, "tls", secretErr.Name)
	assert.ErrorIs(t, secretErr, cause)
	// The outcomes gathered before the failure ride back alongside the
	// error — a caller that audits per secret keeps its trail when the
	// phase dies mid-way.
	require.Len(t, outcomes, 1)
	assert.Equal(t, SecretOutcome{Name: "db", Outcome: SecretOutcomeTransferred}, outcomes[0])
}

type failingCreateTarget struct {
	err      error
	failName string
}

func (f *failingCreateTarget) Has(string) (bool, error) { return false, nil }

func (f *failingCreateTarget) Create(name, value string) error {
	if f.failName == "" || name == f.failName {
		return f.err
	}
	return nil
}

func TestTransferSecrets_TargetHasFailureIsPlainError(t *testing.T) {
	cause := errors.New("store refused the check")
	src := &fakeSecretSource{values: map[string]string{"db": "v1"}}
	tgt := &failingHasTarget{err: cause, failName: "tls"}

	outcomes, err := TransferSecrets(src, tgt, []SecretRef{{Name: "db"}, {Name: "tls"}})

	require.Error(t, err)
	assert.ErrorIs(t, err, cause, "the cause is wrapped")
	var secretErr *SecretError
	assert.False(t, errors.As(err, &secretErr), "a membership-check failure is not a per-secret failure")
	assert.True(t, strings.Contains(err.Error(), "failed to check target secret"), err.Error())
	// The outcomes gathered before the failure ride back alongside the
	// error, like a reveal or create failure's do.
	require.Len(t, outcomes, 1)
	assert.Equal(t, SecretOutcome{Name: "db", Outcome: SecretOutcomeTransferred}, outcomes[0])
}

type failingHasTarget struct {
	err      error
	failName string
}

func (f *failingHasTarget) Has(name string) (bool, error) {
	if name == f.failName {
		return false, f.err
	}
	return false, nil
}

func (f *failingHasTarget) Create(name, value string) error { return nil }
