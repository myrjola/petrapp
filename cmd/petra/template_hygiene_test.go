package main

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// Static design-system / CSP hygiene rules for templates, enforced as a text
// scan over the .gohtml files (the rules live in HTML/CSS text the template AST
// does not model). main.css is the token *definition* source and is not a
// .gohtml file, so it is naturally exempt. The rules are absolute — there is no
// inline opt-out; a genuine exception means editing this test, which forces the
// discussion into a diff. See docs/adr/0008-static-template-linting-over-templ.md
// and cmd/petra/ui/templates/README.md.
var (
	// Rule 1: no inline style="…" attribute (CSP with a nonce silently drops it).
	styleAttrRe = regexp.MustCompile(`(?i)\sstyle\s*=\s*["']`)
	// Rule 2: every opening <style>/<script> tag must carry the CSP nonce.
	openTagRe = regexp.MustCompile(`(?i)<(style|script)\b`)
	// Rule 3: no mouse-only interaction (the app is mobile-first; style :active/:focus-visible).
	mouseOnlyRe = regexp.MustCompile(`(?i):hover\b|cursor\s*:\s*pointer|hover\s*:\s*hover|mouseenter|mouseleave`)
	// Rule 4: no web fonts.
	webFontRe = regexp.MustCompile(`(?i)@font-face|fonts\.(?:googleapis|gstatic)\.com`)
	// Rule 5a: the serif display stack must come from var(--font-serif), never a literal chain.
	fontChainRe = regexp.MustCompile(`(?i)\bCharter\b|\bIowan\b|"New York"`)
	// Rule 5b: colors inside a <style> block must be design tokens, never raw literals.
	colorLiteralRe = regexp.MustCompile(`#(?:[0-9a-fA-F]{8}|[0-9a-fA-F]{6}|[0-9a-fA-F]{3,4})\b|\brgba?\(|\bhsla?\(`)
	// Style-block extractor for the rule-5 scope (inner CSS is submatch 1).
	styleBlockRe = regexp.MustCompile(`(?is)<style[^>]*>(.*?)</style>`)
)

// maskComments blanks the contents of /* */ block comments, // line comments
// (CSS has none, and no template uses // outside JS/URLs; ://-prefixed URLs are
// left intact), and <!-- --> HTML comments, replacing each commented byte with a
// space while preserving newlines so byte offsets and line numbers stay aligned.
// Masking applies to every rule so a comment that merely *mentions* a forbidden
// pattern (e.g. "// the <style> above") never trips the scanner.
func maskComments(s string) string {
	b := []byte(s)
	out := make([]byte, len(b))
	copy(out, b)
	n := len(b)
	blank := func(i int) {
		if i < n && b[i] != '\n' {
			out[i] = ' '
		}
	}
	for i := 0; i < n; {
		switch {
		case i+1 < n && b[i] == '/' && b[i+1] == '*':
			blank(i)
			blank(i + 1)
			i += 2
			for i < n && (b[i] != '*' || i+1 >= n || b[i+1] != '/') {
				blank(i)
				i++
			}
			if i < n {
				blank(i)
				blank(i + 1)
				i += 2
			}
		case i+3 < n && b[i] == '<' && b[i+1] == '!' && b[i+2] == '-' && b[i+3] == '-':
			for i < n {
				if i+2 < n && b[i] == '-' && b[i+1] == '-' && b[i+2] == '>' {
					blank(i)
					blank(i + 1)
					blank(i + 2)
					i += 3
					break
				}
				blank(i)
				i++
			}
		case i+1 < n && b[i] == '/' && b[i+1] == '/' && (i == 0 || b[i-1] != ':'):
			for i < n && b[i] != '\n' {
				blank(i)
				i++
			}
		default:
			i++
		}
	}
	return string(out)
}

func TestTemplateHygiene(t *testing.T) {
	t.Parallel()

	var files []string
	err := filepath.WalkDir("ui/templates", func(p string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if !d.IsDir() && strings.HasSuffix(p, ".gohtml") {
			files = append(files, p)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk templates: %v", err)
	}
	if len(files) == 0 {
		t.Fatal("found no .gohtml templates to scan")
	}

	for _, f := range files {
		raw, readErr := os.ReadFile(f)
		if readErr != nil {
			t.Fatalf("read %s: %v", f, readErr)
		}
		src := maskComments(string(raw))
		report := func(idx int, format string, args ...any) {
			line := 1 + strings.Count(src[:idx], "\n")
			t.Errorf("%s:%d: "+format, append([]any{f, line}, args...)...)
		}

		for _, m := range styleAttrRe.FindAllStringIndex(src, -1) {
			report(m[0]+1, "inline style= attribute — the CSP nonce makes the browser drop it; "+
				"use a nonce'd <style> block (ADR 0008 rule 1)")
		}
		for _, m := range mouseOnlyRe.FindAllStringIndex(src, -1) {
			report(
				m[0],
				"mouse-only interaction %q — app is mobile-first; use :active/:focus-visible (ADR 0008 rule 3)",
				strings.TrimSpace(src[m[0]:m[1]]),
			)
		}
		for _, m := range webFontRe.FindAllStringIndex(src, -1) {
			report(m[0], "web font %q — system stacks only, no @font-face/CDN (ADR 0008 rule 4)", src[m[0]:m[1]])
		}

		// Rule 2: nonce presence on each opening <style>/<script> tag.
		for _, m := range openTagRe.FindAllStringSubmatchIndex(src, -1) {
			name := src[m[2]:m[3]]
			tag := src[m[0]:]
			if gt := strings.IndexByte(tag, '>'); gt >= 0 {
				tag = tag[:gt]
			}
			if !strings.Contains(tag, "Nonce") {
				report(m[0], "<%s> tag missing {{ $.Nonce }}/{{ .Nonce }} — CSP requires it (ADR 0008 rule 2)", name)
			}
		}

		// Rule 5: token-duplicating literals, scoped to inside <style> blocks so
		// HTML color attributes (e.g. <meta theme-color>, <link mask-icon>) don't
		// false-positive.
		for _, blk := range styleBlockRe.FindAllStringSubmatchIndex(src, -1) {
			inner := src[blk[2]:blk[3]]
			base := blk[2]
			for _, fm := range fontChainRe.FindAllStringIndex(inner, -1) {
				report(
					base+fm[0],
					"hardcoded serif font stack inside <style> — use var(--font-serif) (ADR 0008 rule 5)",
				)
			}
			for _, cm := range colorLiteralRe.FindAllStringIndex(inner, -1) {
				report(base+cm[0], "raw color literal %q inside <style> — use a design token (ADR 0008 rule 5)",
					inner[cm[0]:cm[1]])
			}
		}
	}
}
