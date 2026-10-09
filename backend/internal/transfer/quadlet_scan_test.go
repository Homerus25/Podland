package transfer

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The machine-wide quadlet scan (issue 259, §19): the list → print-each
// → match ritual that finds the referencing quadlets, pinned here once
// against a fake reader. The volumes and secrets move previews ride
// this seam — the secrets preview since issue 260, the volumes preview
// since issue 261 — so the suite pins the ritual both domains consume.

// fakeQuadletReader is the scan's fake machine: a fixed name→content
// set, per-name print failures (a file that vanished mid-lease), a list
// failure, and per-name counters so the suite can pin the one-list-call,
// print-exactly-once ritual.
type fakeQuadletReader struct {
	names       []string
	contents    map[string]string
	unprintable map[string]bool
	listErr     error
	listCalls   int
	prints      map[string]int
}

func (f *fakeQuadletReader) ListQuadletNames() ([]string, error) {
	f.listCalls++
	if f.listErr != nil {
		return nil, f.listErr
	}
	return append([]string{}, f.names...), nil
}

func (f *fakeQuadletReader) PrintQuadlet(name string) (string, error) {
	if f.prints == nil {
		f.prints = map[string]int{}
	}
	f.prints[name]++
	if f.unprintable[name] {
		return "", errors.New("quadlet file vanished mid-lease")
	}
	content, ok := f.contents[name]
	if !ok {
		return "", errors.New("no such quadlet file")
	}
	return content, nil
}

// fakeListError is the list failure's typed cause, so the suite can pin
// that the kit's wrap preserves it through errors.As.
type fakeListError struct{ msg string }

func (e *fakeListError) Error() string { return e.msg }

// assertSingleScan pins the scan's fetch ritual: one list call, and
// every listed name printed exactly once per scan.
func assertSingleScan(t *testing.T, f *fakeQuadletReader) {
	t.Helper()
	assert.Equal(t, 1, f.listCalls, "the scan lists the machine exactly once")
	for _, name := range f.names {
		assert.Equal(t, 1, f.prints[name], "quadlet %s must be printed exactly once per scan", name)
	}
}

// assertListFailureWrappedOnce pins the list-failure contract: the kit
// wraps the cause exactly once — the message carries the wrap once and
// the cause survives errors.Is and errors.As.
func assertListFailureWrappedOnce(t *testing.T, err error, cause *fakeListError) {
	t.Helper()
	require.Error(t, err)
	assert.Equal(t, "failed to list quadlets: "+cause.msg, err.Error(),
		"the message carries the wrap exactly once")
	assert.ErrorIs(t, err, cause)
	var typed *fakeListError
	assert.ErrorAs(t, err, &typed, "the cause survives the wrap")
}

// TestQuadletContents pins the contents ritual: every listed quadlet
// printed once into a name→content map; a listed-but-unprintable file
// (it vanished mid-lease) skipped rather than failing the scan; an
// empty machine an empty non-nil map; a list failure wrapped exactly
// once.
func TestQuadletContents(t *testing.T) {
	t.Run("empty machine is an empty non-nil map", func(t *testing.T) {
		r := &fakeQuadletReader{contents: map[string]string{}}

		got, err := QuadletContents(r)

		require.NoError(t, err)
		require.NotNil(t, got, "the map is never nil")
		assert.Empty(t, got)
		assertSingleScan(t, r)
	})

	t.Run("listed but unprintable is skipped, the scan succeeds", func(t *testing.T) {
		r := &fakeQuadletReader{
			names: []string{"web.container", "gone.container", "db.container"},
			contents: map[string]string{
				"web.container": "[Container]\nImage=busybox\n",
				"db.container":  "[Container]\nImage=postgres\n",
			},
			unprintable: map[string]bool{"gone.container": true},
		}

		got, err := QuadletContents(r)

		require.NoError(t, err, "an unprintable file is skipped, never fails the scan")
		assert.Equal(t, map[string]string{
			"web.container": "[Container]\nImage=busybox\n",
			"db.container":  "[Container]\nImage=postgres\n",
		}, got)
		assertSingleScan(t, r)
	})

	t.Run("every listed quadlet is printed exactly once", func(t *testing.T) {
		r := &fakeQuadletReader{
			names: []string{"a.container", "b.container", "c.volume"},
			contents: map[string]string{
				"a.container": "[Container]\nImage=busybox\n",
				"b.container": "[Container]\nImage=postgres\n",
				"c.volume":    "[Volume]\nDriver=local\n",
			},
		}

		got, err := QuadletContents(r)

		require.NoError(t, err)
		assert.Len(t, got, 3)
		assertSingleScan(t, r)
	})

	t.Run("list failure is wrapped exactly once", func(t *testing.T) {
		cause := &fakeListError{msg: "connection lost"}
		r := &fakeQuadletReader{listErr: cause}

		got, err := QuadletContents(r)

		assert.Nil(t, got)
		assertListFailureWrappedOnce(t, err, cause)
	})
}

// TestQuadletsReferencingVolume pins the volume match rules: a bare
// name matches directly; a `.volume` companion source resolves through
// its companion file's VolumeName= (else podman's systemd-<name>
// default); a host bind path never matches. The answer is file-ordered
// and never nil, a file answers once however many of its references
// match, a listed-but-unprintable file is skipped, a list failure is
// wrapped exactly once, and each quadlet is printed exactly once per
// scan.
func TestQuadletsReferencingVolume(t *testing.T) {
	t.Run("bare name matches directly", func(t *testing.T) {
		r := &fakeQuadletReader{
			names: []string{"web.container"},
			contents: map[string]string{
				"web.container": "[Container]\nImage=busybox\nVolume=pgdata:/var/lib/pg:Z\n",
			},
		}

		got, err := QuadletsReferencingVolume(r, "pgdata")

		require.NoError(t, err)
		assert.Equal(t, []string{"web.container"}, got)
		assertSingleScan(t, r)
	})

	t.Run("companion resolves through its VolumeName=", func(t *testing.T) {
		r := &fakeQuadletReader{
			names: []string{"app.container", "app-data.volume"},
			contents: map[string]string{
				"app.container":   "[Container]\nImage=busybox\nVolume=app-data.volume:/data\n",
				"app-data.volume": "[Volume]\nVolumeName=custom-name\n",
			},
		}

		got, err := QuadletsReferencingVolume(r, "custom-name")

		require.NoError(t, err)
		assert.Equal(t, []string{"app.container"}, got)
		assertSingleScan(t, r)

		other, err := QuadletsReferencingVolume(r, "systemd-app-data")
		require.NoError(t, err)
		assert.Empty(t, other, "the VolumeName= override replaces the systemd default")
	})

	t.Run("companion without VolumeName= resolves the systemd default", func(t *testing.T) {
		r := &fakeQuadletReader{
			names: []string{"app.container", "plain.volume"},
			contents: map[string]string{
				"app.container": "[Container]\nImage=busybox\nVolume=plain.volume:/data\n",
				"plain.volume":  "[Volume]\nDriver=local\n",
			},
		}

		got, err := QuadletsReferencingVolume(r, "systemd-plain")

		require.NoError(t, err)
		assert.Equal(t, []string{"app.container"}, got)
		assertSingleScan(t, r)
	})

	t.Run("host bind path never matches", func(t *testing.T) {
		r := &fakeQuadletReader{
			names: []string{"web.container"},
			contents: map[string]string{
				"web.container": "[Container]\nImage=busybox\nVolume=/srv/data:/data:ro\n",
			},
		}

		got, err := QuadletsReferencingVolume(r, "/srv/data")

		require.NoError(t, err)
		require.NotNil(t, got, "the answer is never nil")
		assert.Empty(t, got, "a host bind path resolves no volume name, so it can never match")
		assertSingleScan(t, r)
	})

	t.Run("several referencing quadlets answer in file order", func(t *testing.T) {
		r := &fakeQuadletReader{
			names: []string{"zeta.container", "alpha.container", "mid.container"},
			contents: map[string]string{
				"zeta.container":  "[Container]\nImage=busybox\nVolume=pgdata:/data\n",
				"alpha.container": "[Container]\nImage=busybox\n",
				"mid.container":   "[Container]\nImage=busybox\nVolume=pgdata:/data\nVolume=other:/other\n",
			},
		}

		got, err := QuadletsReferencingVolume(r, "pgdata")

		require.NoError(t, err)
		assert.Equal(t, []string{"zeta.container", "mid.container"}, got,
			"the answer keeps the list's file order, not sorted order")
		assertSingleScan(t, r)
	})

	t.Run("a file with several matching references answers once", func(t *testing.T) {
		r := &fakeQuadletReader{
			names: []string{"web.container"},
			contents: map[string]string{
				"web.container": "[Container]\nImage=busybox\nVolume=pgdata:/a\nVolume=pgdata:/b\n",
			},
		}

		got, err := QuadletsReferencingVolume(r, "pgdata")

		require.NoError(t, err)
		assert.Equal(t, []string{"web.container"}, got)
		assertSingleScan(t, r)
	})

	t.Run("empty machine is an empty non-nil slice", func(t *testing.T) {
		r := &fakeQuadletReader{contents: map[string]string{}}

		got, err := QuadletsReferencingVolume(r, "pgdata")

		require.NoError(t, err)
		require.NotNil(t, got, "the answer is never nil")
		assert.Empty(t, got)
		assertSingleScan(t, r)
	})

	t.Run("listed but unprintable is skipped, the scan succeeds", func(t *testing.T) {
		r := &fakeQuadletReader{
			names: []string{"gone.container", "web.container"},
			contents: map[string]string{
				"web.container": "[Container]\nImage=busybox\nVolume=pgdata:/data\n",
			},
			unprintable: map[string]bool{"gone.container": true},
		}

		got, err := QuadletsReferencingVolume(r, "pgdata")

		require.NoError(t, err, "an unprintable file is skipped, never fails the scan")
		assert.Equal(t, []string{"web.container"}, got)
		assertSingleScan(t, r)
	})

	t.Run("list failure is wrapped exactly once", func(t *testing.T) {
		cause := &fakeListError{msg: "connection lost"}
		r := &fakeQuadletReader{listErr: cause}

		got, err := QuadletsReferencingVolume(r, "pgdata")

		assert.Nil(t, got)
		assertListFailureWrappedOnce(t, err, cause)
	})
}

// TestQuadletsReferencingSecret pins the secret match rules: only a
// kind-secret reference names a store secret and matches — a hostPath
// source is read off the host filesystem at run time and never
// consults the store, so it never matches. The answer is file-ordered
// and never nil, a file answers once however many of its references
// match, a listed-but-unprintable file is skipped, a list failure is
// wrapped exactly once, and each quadlet is printed exactly once per
// scan.
func TestQuadletsReferencingSecret(t *testing.T) {
	t.Run("kind-secret reference matches", func(t *testing.T) {
		r := &fakeQuadletReader{
			names: []string{"web.container"},
			contents: map[string]string{
				"web.container": "[Container]\nImage=busybox\nSecret=token1\n",
			},
		}

		got, err := QuadletsReferencingSecret(r, "token1")

		require.NoError(t, err)
		assert.Equal(t, []string{"web.container"}, got)
		assertSingleScan(t, r)
	})

	t.Run("all-options source= form matches", func(t *testing.T) {
		r := &fakeQuadletReader{
			names: []string{"web.container"},
			contents: map[string]string{
				"web.container": "[Container]\nImage=busybox\nSecret=source=token1,target=/run/secrets/tok\n",
			},
		}

		got, err := QuadletsReferencingSecret(r, "token1")

		require.NoError(t, err)
		assert.Equal(t, []string{"web.container"}, got)
		assertSingleScan(t, r)
	})

	t.Run("hostPath source never matches", func(t *testing.T) {
		r := &fakeQuadletReader{
			names: []string{"web.container"},
			contents: map[string]string{
				"web.container": "[Container]\nImage=busybox\nSecret=/etc/certs/token\n",
			},
		}

		got, err := QuadletsReferencingSecret(r, "/etc/certs/token")

		require.NoError(t, err)
		require.NotNil(t, got, "the answer is never nil")
		assert.Empty(t, got, "a hostPath source never consults the store, so it can never match")
		assertSingleScan(t, r)
	})

	t.Run("several referencing quadlets answer in file order", func(t *testing.T) {
		r := &fakeQuadletReader{
			names: []string{"zeta.container", "alpha.container", "mid.container"},
			contents: map[string]string{
				"zeta.container":  "[Container]\nImage=busybox\nSecret=token1\n",
				"alpha.container": "[Container]\nImage=busybox\n",
				"mid.container":   "[Container]\nImage=busybox\nSecret=token1\nSecret=other\n",
			},
		}

		got, err := QuadletsReferencingSecret(r, "token1")

		require.NoError(t, err)
		assert.Equal(t, []string{"zeta.container", "mid.container"}, got,
			"the answer keeps the list's file order, not sorted order")
		assertSingleScan(t, r)
	})

	t.Run("a file with several matching references answers once", func(t *testing.T) {
		r := &fakeQuadletReader{
			names: []string{"web.container"},
			contents: map[string]string{
				"web.container": "[Container]\nImage=busybox\nSecret=token1\nSecret=token1,target=/run/secrets/tok\n",
			},
		}

		got, err := QuadletsReferencingSecret(r, "token1")

		require.NoError(t, err)
		assert.Equal(t, []string{"web.container"}, got)
		assertSingleScan(t, r)
	})

	t.Run("empty machine is an empty non-nil slice", func(t *testing.T) {
		r := &fakeQuadletReader{contents: map[string]string{}}

		got, err := QuadletsReferencingSecret(r, "token1")

		require.NoError(t, err)
		require.NotNil(t, got, "the answer is never nil")
		assert.Empty(t, got)
		assertSingleScan(t, r)
	})

	t.Run("listed but unprintable is skipped, the scan succeeds", func(t *testing.T) {
		r := &fakeQuadletReader{
			names: []string{"gone.container", "web.container"},
			contents: map[string]string{
				"web.container": "[Container]\nImage=busybox\nSecret=token1\n",
			},
			unprintable: map[string]bool{"gone.container": true},
		}

		got, err := QuadletsReferencingSecret(r, "token1")

		require.NoError(t, err, "an unprintable file is skipped, never fails the scan")
		assert.Equal(t, []string{"web.container"}, got)
		assertSingleScan(t, r)
	})

	t.Run("list failure is wrapped exactly once", func(t *testing.T) {
		cause := &fakeListError{msg: "connection lost"}
		r := &fakeQuadletReader{listErr: cause}

		got, err := QuadletsReferencingSecret(r, "token1")

		assert.Nil(t, got)
		assertListFailureWrappedOnce(t, err, cause)
	})
}
