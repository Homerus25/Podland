package transfer

import (
	"path"
	"strings"
)

// The quadlet reference-resolution seam (issue 242, §19): the one parse
// and resolution of quadlet `Volume=` and `Secret=` lines. The quadlet
// move (its preview and its volume-data plan), the volumes move preview,
// and the secrets move preview all call it — a preview that parsed
// differently from the quadlet domain would lie about what a move breaks,
// which is the drift risk that required one seam. The kit owns the
// rituals; the callers own their wire vocabulary (the quadlet preview's
// kind labels, the volumes/secrets previews' match-or-not) and map from
// the kinds below.

// Kind labels for the quadlet reference parse: a `Volume=` source with no
// path separator names a podman volume, anything else is a host bind
// path; a `Secret=` reference whose resolved source is an absolute path
// is read off the host filesystem at run time and never enters a store,
// anything else names a podman secret.
const (
	QuadletRefKindVolume   = "volume"
	QuadletRefKindSecret   = "secret"
	QuadletRefKindHostPath = "hostPath"
)

// unitEntries walks quadlet file content the way systemd's unit-file
// parser does — trimmed lines, comment lines skipped, key and value cut
// at the first "=" — and hands each entry's trimmed key and raw value to
// fn. The three reference parses below share this one scan.
func unitEntries(content string, fn func(key, value string)) {
	for line := range strings.Lines(content) {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}
		key, value, found := strings.Cut(line, "=")
		if !found {
			continue
		}
		fn(strings.TrimSpace(key), value)
	}
}

// QuadletVolumeRef is one parsed `Volume=` line: Source is the reference
// as written (the part of the value before the first ":" — the rest is
// the container mount point and options, `Volume=name:/mnt:ro`), Kind
// labels it a named podman volume or a host bind path.
type QuadletVolumeRef struct {
	Source string
	Kind   string
}

// ParseQuadletVolumes extracts every `Volume=` line from quadlet file
// content and labels each source: a source with no path separator is a
// named podman volume, anything else a host bind path. Key matching is
// case-insensitive (unitEntries's parser ritual). The result is
// file-ordered and never nil, so a quadlet without `Volume=` lines
// renders as an empty JSON array.
func ParseQuadletVolumes(content string) []QuadletVolumeRef {
	refs := []QuadletVolumeRef{}
	unitEntries(content, func(key, value string) {
		if !strings.EqualFold(key, "Volume") {
			return
		}
		value = strings.TrimSpace(value)
		if value == "" {
			return
		}
		// The source is everything before the first ":"; the rest is the
		// container mount point and options (`Volume=name:/mnt:ro`).
		source := value
		if idx := strings.Index(value, ":"); idx >= 0 {
			source = value[:idx]
		}
		kind := QuadletRefKindVolume
		if strings.ContainsRune(source, '/') {
			kind = QuadletRefKindHostPath
		}
		refs = append(refs, QuadletVolumeRef{Source: source, Kind: kind})
	})
	return refs
}

// QuadletVolumeCandidate applies the companion discovery rule to one
// parsed volume source: a source ending in ".volume" with no path
// separator references the same-named quadlet FILE on the same host —
// podman resolves exactly the suffix form (the quadlet-generated volume
// is systemd-<name> unless VolumeName= overrides), so bare names never
// match and host paths are hostPath references, not quadlet references.
// The return is the candidate file name; "" means the source references
// no quadlet. A candidate with no matching source file is dangling — the
// caller leaves such an entry ordinary.
func QuadletVolumeCandidate(source string) string {
	if source == "" || strings.ContainsRune(source, '/') || !strings.HasSuffix(source, ".volume") {
		return ""
	}
	return source
}

// ResolveQuadletVolumeName maps one `Volume=` source to the podman volume
// it names. contents carries exactly the host's quadlet files that
// exist and printed — the caller's one fetch per lease (the companion
// set, or every printable quadlet): membership IS the existence test, a
// file missing from the map — absent, or listed but unprintable
// mid-lease — counts as absent. A bare name is itself; a `.volume`
// companion source resolves through its companion file's VolumeName=
// (first non-empty wins) when the companion exists, else podman's
// systemd-<name> default — a dangling companion still resolves the
// default name, so a preview can label the reference and a data phase
// can skip it honestly instead of guessing. A host bind path (a "/" in
// the source) never resolves: ("", ""). The second return is the
// companion .volume candidate, present for dangling references too; ""
// for bare names and host paths.
func ResolveQuadletVolumeName(source string, contents map[string]string) (name, quadletFile string) {
	if source == "" || strings.ContainsRune(source, '/') {
		return "", ""
	}
	quadletFile = QuadletVolumeCandidate(source)
	if quadletFile == "" {
		return source, ""
	}
	name = "systemd-" + strings.TrimSuffix(quadletFile, ".volume")
	if content, ok := contents[quadletFile]; ok {
		if override := quadletVolumeName(content); override != "" {
			name = override
		}
	}
	return name, quadletFile
}

// quadletVolumeName extracts the first non-empty VolumeName= value from
// quadlet file content — the authoritative podman volume name for the
// volume a .volume quadlet generates. Key matching is case-insensitive
// (unitEntries's parser ritual). "" means the file sets no name and
// podman's systemd-<name> default applies.
func quadletVolumeName(content string) string {
	name := ""
	unitEntries(content, func(key, value string) {
		if name != "" || !strings.EqualFold(key, "VolumeName") {
			return
		}
		if value = strings.TrimSpace(value); value != "" {
			name = value
		}
	})
	return name
}

// QuadletSecretRef is one parsed `Secret=` line: the reference's resolved
// name and its kind.
type QuadletSecretRef struct {
	Name string
	Kind string
}

// ParseQuadletSecrets extracts every `Secret=` line from quadlet file
// content and resolves each reference's name and kind. The name is the
// token before the first comma; when that token contains `=` (the
// all-options form), `source=` governs. An absolute-path source is a
// hostPath reference — podman reads it off the host filesystem at run
// time, so it never enters a secret store; everything else is a podman
// secret. The parse is type-generic — podman honors the key in .container
// and .build files alike, and other resource types simply carry no
// `Secret=` lines. A line that resolves to no name (an empty value, or
// an all-options token without `source=`) yields no entry. Key matching
// is case-insensitive (unitEntries's parser ritual), and the result is
// file-ordered and never nil.
func ParseQuadletSecrets(content string) []QuadletSecretRef {
	refs := []QuadletSecretRef{}
	unitEntries(content, func(key, value string) {
		if !strings.EqualFold(key, "Secret") {
			return
		}
		name, kind := resolveQuadletSecretReference(strings.TrimSpace(value))
		if name == "" {
			return
		}
		refs = append(refs, QuadletSecretRef{Name: name, Kind: kind})
	})
	return refs
}

// resolveQuadletSecretReference applies the naming rule to one `Secret=`
// value: the token before the first comma is the name, unless it carries
// `=` — the all-options form, where the `source=` option governs (a token
// without `source=` names nothing). A resolved name starting with `/` is
// an absolute-path hostPath reference; anything else is a podman secret.
func resolveQuadletSecretReference(value string) (name, kind string) {
	token := value
	if idx := strings.Index(value, ","); idx >= 0 {
		token = value[:idx]
	}
	if optKey, optValue, found := strings.Cut(token, "="); found {
		if !strings.EqualFold(strings.TrimSpace(optKey), "source") {
			return "", ""
		}
		token = strings.TrimSpace(optValue)
	}
	if token == "" {
		return "", ""
	}
	kind = QuadletRefKindSecret
	if path.IsAbs(token) {
		kind = QuadletRefKindHostPath
	}
	return token, kind
}
