package sshagent

import (
	"fmt"
	"strings"

	"github.com/newtosh/timeshare/internal/onepassword"
)

// getItemFingerprint is a seam over onepassword.GetItemFingerprint so
// tests can exercise ResolveFingerprints' parsing/error-propagation
// behavior without shelling out to `op`.
var getItemFingerprint = onepassword.GetItemFingerprint

// listItems is a seam over onepassword.ListItems so unit tests that exercise
// resolution errors don't invoke the real `op` CLI while building suggestions.
var listItems = onepassword.ListItems

// ResolveFingerprints resolves each ref — a "<vault>/<item-title-or-id>"
// string, the same split-on-rightmost-"/" syntax timeshare's --from-item
// flag already uses — to that item's SSH key fingerprint, in order.
// Fails on the first bad ref, naming it and offering up to 3 "did you
// mean" suggestions from that ref's vault.
func ResolveFingerprints(refs []string) ([]string, error) {
	fingerprints := make([]string, 0, len(refs))
	for _, ref := range refs {
		idx := strings.LastIndex(ref, "/")
		if idx < 0 {
			return nil, fmt.Errorf("ssh key reference %q must be in <vault>/<item> form", ref)
		}
		vault, item := ref[:idx], ref[idx+1:]

		fp, err := getItemFingerprint(vault, item)
		if err != nil {
			return nil, fmt.Errorf("resolving SSH key %q: %w%s", ref, err, suggestionText(vault, item))
		}
		fingerprints = append(fingerprints, fp)
	}
	return fingerprints, nil
}

// suggestionText returns a "did you mean" block for item within vault,
// or "" if the vault itself can't be listed (the caller already has a
// real error to report) or nothing close matches.
func suggestionText(vault, item string) string {
	items, err := listItems(vault)
	if err != nil {
		return ""
	}
	matches := onepassword.SuggestMatches(item, items, 3)
	if len(matches) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("\ndid you mean:")
	for _, m := range matches {
		fmt.Fprintf(&b, "\n  - %s (id: %s)", m.Title, m.ID)
	}
	return b.String()
}
