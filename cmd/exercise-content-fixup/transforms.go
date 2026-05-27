package main

import (
	"regexp"
	"strings"
)

// resourceLinkPattern matches a markdown list item of the form
// `- [Title](https://...)`. The URL is captured in group 1.
var resourceLinkPattern = regexp.MustCompile(`^\s*-\s*\[[^\]]*\]\((https?://[^\s)]+)\)\s*$`)

// placeholderURLPrefix marks URLs the AI generator emits as unfilled
// placeholders. They are dead by definition; never HEAD-check them.
const placeholderURLPrefix = "https://example.com/"

// isResourceLinkAlive reports whether the markdown list item line should be
// retained. The second return value is true when the line matches
// resourceLinkPattern; in that case the first value reflects whether the URL
// passes aliveness checks.
func isResourceLinkAlive(line string, aliveURLs map[string]bool) (bool, bool) {
	m := resourceLinkPattern.FindStringSubmatch(line)
	if m == nil {
		return true, false
	}
	url := m[1]
	if strings.HasPrefix(url, placeholderURLPrefix) {
		return false, true
	}
	return aliveURLs[url], true
}

// StripDeadResourceLinks removes resource list items whose URLs are not in
// aliveURLs (or whose URLs match the placeholder prefix). If no list items
// survive in the ## Resources section, the heading is also removed.
//
// The caller is responsible for pre-populating aliveURLs by HEAD-checking
// every URL it cares about; transforms.go has no network dependency.
func StripDeadResourceLinks(desc string, aliveURLs map[string]bool) string {
	lines := strings.Split(desc, "\n")
	var (
		out                []string
		inResources        bool
		resourcesHeaderIdx = -1
		survivors          int
	)

	for _, line := range lines {
		switch {
		case strings.HasPrefix(line, "## Resources"):
			inResources = true
			resourcesHeaderIdx = len(out)
			out = append(out, line)
		case inResources && strings.HasPrefix(line, "##"):
			inResources = false
			if survivors == 0 {
				out = dropResourcesHeader(out, resourcesHeaderIdx)
			}
			resourcesHeaderIdx = -1
			out = append(out, line)
		case inResources:
			keep, isLink := isResourceLinkAlive(line, aliveURLs)
			if !isLink {
				// Non-link content (blank lines, prose) — keep as-is.
				out = append(out, line)
			} else if keep {
				out = append(out, line)
				survivors++
			}
		default:
			out = append(out, line)
		}
	}

	// Resources at EOF with no trailing section header to flush.
	if inResources && survivors == 0 {
		out = dropResourcesHeader(out, resourcesHeaderIdx)
	}

	return strings.Join(out, "\n")
}

// dropResourcesHeader removes the "## Resources" line at idx and any
// immediately-trailing blank lines that were the separator before the
// (now empty) section content. Returns the trimmed slice.
func dropResourcesHeader(out []string, idx int) []string {
	if idx < 0 || idx >= len(out) {
		return out
	}
	// Drop the header itself.
	out = append(out[:idx], out[idx+1:]...)
	// Drop one trailing blank line if present (visual separator).
	if idx > 0 && idx <= len(out) && idx-1 < len(out) && out[idx-1] == "" {
		out = append(out[:idx-1], out[idx:]...)
	}
	return out
}
