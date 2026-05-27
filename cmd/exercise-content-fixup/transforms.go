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

// repGuidancePatterns lists patterns that identify ordered-list items in the
// ## Instructions section that convey rep / set / duration targets. These
// targets are tracked separately by the app and must not appear in prose.
//
// Each pattern is intentionally narrow so it does not fire on:
//   - "Take 2 deep breaths" (no reps/sets keyword)
//   - "3-second tempo" (no hold/perform/do/complete verb)
//   - Common Mistakes items like "Doing 50 reps at once: pace yourself"
//     (those are outside ## Instructions, handled by inInstructions guard).
//
//nolint:gochecknoglobals // compiled once at startup; patterns are read-only.
var repGuidancePatterns = []*regexp.Regexp{
	// "perform 8-12 reps", "perform 10 repetitions", "perform 8 times"
	regexp.MustCompile(`(?i)\bperform\s+\d+(?:\s*[-–]\s*\d+)?\s*(?:reps?|repetitions?|times)\b`),
	// "do 8-12 reps", "do 10 repetitions"
	regexp.MustCompile(`(?i)\bdo\s+\d+(?:\s*[-–]\s*\d+)?\s*(?:reps?|repetitions?)\b`),
	// "complete 3 sets"
	regexp.MustCompile(`(?i)\bcomplete\s+\d+(?:\s*[-–]\s*\d+)?\s*sets?\b`),
	// "8-12 reps" / "8 reps" / "3 sets" at the start of an instruction step
	regexp.MustCompile(`(?i)^\s*\d+\.\s*\d+(?:\s*[-–]\s*\d+)?\s*(?:reps?|repetitions?|sets?)\b`),
	// "hold for 30 seconds", "hold for 5s"
	regexp.MustCompile(`(?i)\bhold\s+for\s+\d+\s*(?:seconds?|s)\b`),
	// Literal template-leak phrase
	regexp.MustCompile(`(?i)\brepetition guidance\b`),
}

// orderedListItemPattern matches a markdown ordered-list item like "5. text".
var orderedListItemPattern = regexp.MustCompile(`^\s*\d+\.\s+`)

// StripRepGuidanceLines drops ordered-list items in the ## Instructions
// section whose text matches any repGuidancePatterns entry. Other sections
// (## Common Mistakes, ## Resources) are passed through unchanged — rep
// mentions there describe errors to avoid, not targets to hit.
func StripRepGuidanceLines(desc string) string {
	lines := strings.Split(desc, "\n")
	out := make([]string, 0, len(lines))
	inInstructions := false

	for _, line := range lines {
		if strings.HasPrefix(line, "## Instructions") {
			inInstructions = true
			out = append(out, line)
			continue
		}
		if inInstructions && strings.HasPrefix(line, "##") {
			inInstructions = false
		}

		if inInstructions && orderedListItemPattern.MatchString(line) && matchesRepGuidance(line) {
			continue
		}
		out = append(out, line)
	}

	return strings.Join(out, "\n")
}

func matchesRepGuidance(line string) bool {
	for _, p := range repGuidancePatterns {
		if p.MatchString(line) {
			return true
		}
	}
	return false
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
