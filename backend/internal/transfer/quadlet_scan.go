package transfer

import "fmt"

// The host-wide quadlet scan (issue 259, §19): the list → print-each
// → match ritual that finds the referencing quadlets — the quadlets
// whose `Volume=` or `Secret=` lines resolve to a given volume or
// secret. The volumes and secrets move previews each carried a private
// copy of this ritual; the kit owns the seam now, and both previews
// ride it — the secrets preview since issue 260, the volumes preview
// since issue 261. The scan answers a preview's warning, never a block:
// a listed-but-unprintable file (it vanished mid-lease) is skipped
// rather than failing the scan (§19). The quadlet move's companion
// discovery is deliberately NOT a rider of this seam — it keeps its own
// loop and its stricter fail-on-non-not-found policy (§19).

// QuadletReader is the host's quadlet-file width the scan needs:
// list the quadlet file names, print one file's content. A domain
// engine whose methods already match satisfies it structurally; the
// kit stays pure — no podman imports, no connection wiring, the
// domains keep their engine adapters.
type QuadletReader interface {
	ListQuadletNames() ([]string, error)
	PrintQuadlet(name string) (string, error)
}

// scanQuadlets owns the list → print-each ritual: one list call, every
// listed name printed exactly once into a name→content map. A
// listed-but-unprintable file (it vanished mid-lease) cannot be scanned
// and is skipped rather than failing the scan — the previews' policy
// (§19). A list failure is wrapped exactly once, with the same message
// the domains' private copies used, so internal error text is
// unchanged. The names keep file order; the map carries exactly the
// printable files — membership IS the printability test.
func scanQuadlets(r QuadletReader) ([]string, map[string]string, error) {
	names, err := r.ListQuadletNames()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to list quadlets: %w", err)
	}
	contents := make(map[string]string, len(names))
	for _, name := range names {
		content, err := r.PrintQuadlet(name)
		if err != nil {
			// Listed but unprintable — the file vanished mid-lease. It
			// cannot be scanned; skip it rather than fail the scan.
			continue
		}
		contents[name] = content
	}
	return names, contents, nil
}

// QuadletContents prints every quadlet on the host once into a
// name→content map (issue 259, §19) — the contents half of the scan,
// for callers that need the files themselves. An empty host answers
// an empty non-nil map; a listed-but-unprintable file is skipped; a
// list failure is wrapped exactly once.
func QuadletContents(r QuadletReader) (map[string]string, error) {
	_, contents, err := scanQuadlets(r)
	if err != nil {
		return nil, err
	}
	return contents, nil
}

// QuadletsReferencingVolume lists the host's quadlet files whose
// `Volume=` lines resolve to the volume (issue 259, §19) — the move
// previews' warning, never a block. A bare-name source matches
// directly; a `.volume` companion source resolves through its
// companion file's VolumeName= (first non-empty wins, podman's
// systemd-<name> default otherwise); a host bind path never matches.
// The `Volume=` parse and the source→volume resolution are the kit's
// quadlet reference seam (issue 242) — the same ritual the quadlet
// move's preview and plan read, so a preview cannot drift from what a
// quadlet move would report. The result is file-ordered and never nil;
// a file answers once however many of its references match.
func QuadletsReferencingVolume(r QuadletReader, volumeName string) ([]string, error) {
	names, contents, err := scanQuadlets(r)
	if err != nil {
		return nil, err
	}
	referencing := []string{}
	for _, name := range names {
		content, ok := contents[name]
		if !ok {
			continue
		}
		for _, ref := range ParseQuadletVolumes(content) {
			// A host bind path never resolves — the seam answers "" for
			// it, so it can never equal a volume name.
			if resolved, _ := ResolveQuadletVolumeName(ref.Source, contents); resolved == volumeName {
				referencing = append(referencing, name)
				break
			}
		}
	}
	return referencing, nil
}

// QuadletsReferencingSecret lists the host's quadlet files whose
// `Secret=` lines resolve to the secret (issue 259, §19) — the move
// previews' warning, never a block. Only a kind-secret reference names
// a store secret; a hostPath source is read off the host filesystem at
// run time and never consults the store, so it never matches. The
// `Secret=` parse and the reference naming are the kit's quadlet
// reference seam (issue 242) — the same ritual the quadlet move's
// preview reads, so a preview cannot drift from what a quadlet move
// would report. The result is file-ordered and never nil; a file
// answers once however many of its references match.
func QuadletsReferencingSecret(r QuadletReader, secretName string) ([]string, error) {
	names, contents, err := scanQuadlets(r)
	if err != nil {
		return nil, err
	}
	referencing := []string{}
	for _, name := range names {
		content, ok := contents[name]
		if !ok {
			continue
		}
		for _, ref := range ParseQuadletSecrets(content) {
			// Only a kind secret reference names a store secret — a
			// hostPath source is read off the host filesystem at run
			// time and never consults the store.
			if ref.Kind == QuadletRefKindSecret && ref.Name == secretName {
				referencing = append(referencing, name)
				break
			}
		}
	}
	return referencing, nil
}
