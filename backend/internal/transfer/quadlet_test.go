package transfer

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The quadlet reference-resolution seam (issue 242, §19): the rituals
// every `Volume=`/`Secret=`-reading surface shares, pinned here once —
// the quadlet move's preview/plan and the volumes and secrets move
// previews all call these, so their suites pin their own projections,
// not the parse itself.

// TestParseQuadletVolumes pins the `Volume=` line parse: the source is
// the token before the first ":", a source with no path separator is a
// named podman volume, anything else a host bind path; every line counts
// in file order, key matching is case-insensitive, and comment lines are
// skipped.
func TestParseQuadletVolumes(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    []QuadletVolumeRef
	}{
		{
			name:    "named volume with mount point and options",
			content: "[Container]\nVolume=pgdata:/var/lib/pg:Z\n",
			want:    []QuadletVolumeRef{{Source: "pgdata", Kind: QuadletRefKindVolume}},
		},
		{
			name:    "host path with mount point",
			content: "[Container]\nVolume=/srv/data:/data:ro\n",
			want:    []QuadletVolumeRef{{Source: "/srv/data", Kind: QuadletRefKindHostPath}},
		},
		{
			name:    "bare named volume without mount point",
			content: "[Container]\nVolume=mydata\n",
			want:    []QuadletVolumeRef{{Source: "mydata", Kind: QuadletRefKindVolume}},
		},
		{
			name:    "bare host path without mount point",
			content: "[Container]\nVolume=/srv/data\n",
			want:    []QuadletVolumeRef{{Source: "/srv/data", Kind: QuadletRefKindHostPath}},
		},
		{
			name: "every Volume= line counts, in file order, mixed kinds",
			content: "[Container]\nImage=busybox\n" +
				"Volume=alpha:/a\n" +
				"Volume=/host/b:/b\n" +
				"Volume=gamma\n",
			want: []QuadletVolumeRef{
				{Source: "alpha", Kind: QuadletRefKindVolume},
				{Source: "/host/b", Kind: QuadletRefKindHostPath},
				{Source: "gamma", Kind: QuadletRefKindVolume},
			},
		},
		{
			name:    "comments and other keys are ignored, key match is case-insensitive",
			content: "[Container]\n# Volume=commented:/out\n; Volume=also-commented:/out\nImage=busybox\nvolume=lowercase\nNetwork=host\n",
			want:    []QuadletVolumeRef{{Source: "lowercase", Kind: QuadletRefKindVolume}},
		},
		{
			name:    "empty value yields no entry",
			content: "[Container]\nVolume=\n",
			want:    []QuadletVolumeRef{},
		},
		{
			name:    "no Volume lines is an empty non-nil slice",
			content: "[Container]\nImage=busybox\nSecret=token\n",
			want:    []QuadletVolumeRef{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ParseQuadletVolumes(tt.content)
			require.NotNil(t, got, "the parse must never return nil")
			assert.Equal(t, tt.want, got)
		})
	}
}

// TestQuadletVolumeCandidate pins the companion discovery rule: only a
// source ending in ".volume" with no path separator names a quadlet file.
func TestQuadletVolumeCandidate(t *testing.T) {
	assert.Equal(t, "app-data.volume", QuadletVolumeCandidate("app-data.volume"))
	assert.Empty(t, QuadletVolumeCandidate("pgdata"), "a bare name references no quadlet file")
	assert.Empty(t, QuadletVolumeCandidate("/srv/app.volume"), "a path separator makes it a host path")
	assert.Empty(t, QuadletVolumeCandidate(""), "an empty source references nothing")
}

// TestResolveQuadletVolumeName pins the source→volume resolution: a bare
// name is itself; a `.volume` companion source resolves through its
// companion's VolumeName= when the companion printed, else podman's
// systemd-<name> default (a dangling companion included); a host path
// never resolves.
func TestResolveQuadletVolumeName(t *testing.T) {
	contents := map[string]string{
		"app-data.volume": "[Volume]\nVolumeName=custom-name\n",
		"plain.volume":    "[Volume]\nDriver=local\n",
	}
	tests := []struct {
		name            string
		source          string
		wantName        string
		wantQuadletFile string
	}{
		{
			name:            "bare name is itself",
			source:          "pgdata",
			wantName:        "pgdata",
			wantQuadletFile: "",
		},
		{
			name:            "host path never resolves",
			source:          "/srv/data",
			wantName:        "",
			wantQuadletFile: "",
		},
		{
			name:            "companion VolumeName= overrides the default",
			source:          "app-data.volume",
			wantName:        "custom-name",
			wantQuadletFile: "app-data.volume",
		},
		{
			name:            "companion without VolumeName= resolves the systemd default",
			source:          "plain.volume",
			wantName:        "systemd-plain",
			wantQuadletFile: "plain.volume",
		},
		{
			name:            "dangling companion still resolves the default",
			source:          "missing.volume",
			wantName:        "systemd-missing",
			wantQuadletFile: "missing.volume",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			name, quadletFile := ResolveQuadletVolumeName(tt.source, contents)
			assert.Equal(t, tt.wantName, name)
			assert.Equal(t, tt.wantQuadletFile, quadletFile)
		})
	}
}

// TestQuadletVolumeName pins the VolumeName= extraction: the first
// non-empty value wins, key matching is case-insensitive, comment lines
// are skipped, and a file that sets no name answers "".
func TestQuadletVolumeName(t *testing.T) {
	assert.Equal(t, "custom", quadletVolumeName("[Volume]\nVolumeName=custom\n"))
	assert.Equal(t, "lowercase", quadletVolumeName("[Volume]\n# VolumeName=commented\nvolumename=lowercase\n"),
		"key matching is case-insensitive and comment lines are skipped")
	assert.Equal(t, "second", quadletVolumeName("[Volume]\nVolumeName=\nVolumeName=second\n"),
		"the first non-empty value wins")
	assert.Empty(t, quadletVolumeName("[Volume]\nDriver=local\n"), "no VolumeName= means the default applies")
}

// TestParseQuadletSecrets pins the `Secret=` line parse: the token before
// the first comma is the name unless it carries `=` (the all-options
// form, where source= governs); an absolute-path source is a hostPath
// reference; every line counts in file order, key matching is
// case-insensitive, and comment lines are skipped.
func TestParseQuadletSecrets(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    []QuadletSecretRef
	}{
		{
			name:    "plain token",
			content: "[Container]\nImage=busybox\nSecret=token1\n",
			want:    []QuadletSecretRef{{Name: "token1", Kind: QuadletRefKindSecret}},
		},
		{
			name:    "options after the token keep the token the name",
			content: "Secret=token1,target=/run/secrets/tok,uid=0\n",
			want:    []QuadletSecretRef{{Name: "token1", Kind: QuadletRefKindSecret}},
		},
		{
			name:    "all-options form resolves through source=",
			content: "Secret=source=token1,target=/run/secrets/tok\n",
			want:    []QuadletSecretRef{{Name: "token1", Kind: QuadletRefKindSecret}},
		},
		{
			name:    "all-options form without source= names nothing",
			content: "Secret=target=/run/secrets/tok\n",
			want:    []QuadletSecretRef{},
		},
		{
			name:    "absolute path source is hostPath",
			content: "Secret=/etc/containers/certs/token\n",
			want:    []QuadletSecretRef{{Name: "/etc/containers/certs/token", Kind: QuadletRefKindHostPath}},
		},
		{
			name:    "all-options form with absolute source is hostPath",
			content: "Secret=source=/etc/certs/token,target=/run/secrets/tok\n",
			want:    []QuadletSecretRef{{Name: "/etc/certs/token", Kind: QuadletRefKindHostPath}},
		},
		{
			name:    "key matching is case-insensitive",
			content: "secret=token1\nSECRET=token2\n",
			want: []QuadletSecretRef{
				{Name: "token1", Kind: QuadletRefKindSecret},
				{Name: "token2", Kind: QuadletRefKindSecret},
			},
		},
		{
			name:    "comments and other keys are skipped",
			content: "# Secret=commented\n; Secret=also-commented\nImage=busybox\n\nSecret=token1\n",
			want:    []QuadletSecretRef{{Name: "token1", Kind: QuadletRefKindSecret}},
		},
		{
			name:    "empty value yields no entry",
			content: "Secret=\n",
			want:    []QuadletSecretRef{},
		},
		{
			name:    "no Secret lines is an empty non-nil slice",
			content: "[Container]\nImage=busybox\nVolume=data:/data\n",
			want:    []QuadletSecretRef{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ParseQuadletSecrets(tt.content)
			require.NotNil(t, got, "the parse must never return nil")
			assert.Equal(t, tt.want, got)
		})
	}
}

// TestParseQuadletSecrets_TypeGeneric pins the one-rule ruling: the scan
// is content-generic — a .build quadlet's Secret= lines parse exactly
// like a .container's (podman honors the key in both today).
func TestParseQuadletSecrets_TypeGeneric(t *testing.T) {
	container := ParseQuadletSecrets("[Container]\nImage=busybox\nSecret=token1\n")
	build := ParseQuadletSecrets("[Build]\nImage=busybox\nSecret=token1\n")
	assert.Equal(t, container, build)
}
