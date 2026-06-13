package main

import (
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
)

// embeddedUI bakes the template and static trees into the binary so production
// runs as a single self-contained executable with no ui/ directory on disk.
// Dev reads the same files live from the filesystem instead (see uiFilesystems)
// so edits hot-reload without a rebuild.
//
//go:embed ui/templates ui/static
var embeddedUI embed.FS

// assetHashLen is the number of hex chars of the SHA-256 content hash kept in
// fingerprinted asset URLs (e.g. /main.<8 hex>.css). 32 bits is ample to
// distinguish a handful of static files; it's a cache-busting fingerprint, not
// a cryptographic identifier.
const assetHashLen = 8

// processedAsset is an asset whose body references other assets and is therefore
// served from a rewritten in-memory copy rather than straight from the FS.
type processedAsset struct {
	body        []byte
	contentType string
}

// assetManifest maps plain asset paths (e.g. "/main.css") to content-hashed
// URLs (e.g. "/main.<hash>.css") and back. The asset template func emits the
// hashed URL so browsers cache-bust on content change; the file server resolves
// a hashed (or stale, or plain) request path back to the real file. It also
// holds rewritten copies of assets whose body links other assets — currently
// only manifest.json, whose icon srcs are rewritten to their hashed URLs.
//
// All methods are nil-safe: a zero-value/nil manifest behaves as identity
// (no fingerprinting), so template-rendering tests that build an *application
// literal without a manifest still work.
type assetManifest struct {
	// toHashed maps "/main.css" -> "/main.<hash>.css".
	toHashed map[string]string
	// toReal maps "/main.<hash>.css" -> "/main.css" for the current hashes.
	toReal map[string]string
	// processed holds rewritten bodies keyed by real path (e.g. "/manifest.json").
	processed map[string]processedAsset
}

// URL returns the content-hashed URL for a plain asset path, or the input
// unchanged when the asset is unknown (an unregistered path, or a file added
// during a dev session after startup).
func (m *assetManifest) URL(p string) string {
	if m == nil {
		return p
	}
	if hashed, ok := m.toHashed[p]; ok {
		return hashed
	}
	return p
}

// resolve maps a request path to the real asset path and reports whether the
// request carried the current content hash. Only an exact hash match is safe to
// cache immutably; a plain path or a stale hash resolves to the real file with
// exact == false so the response is merely revalidated.
func (m *assetManifest) resolve(reqPath string) (string, bool) {
	if m != nil {
		if realPath, ok := m.toReal[reqPath]; ok {
			return realPath, true
		}
	}
	if stripped := stripAssetHash(reqPath); stripped != reqPath {
		return stripped, false
	}
	return reqPath, false
}

// processedAssetFor returns the rewritten in-memory copy for a real asset path,
// if one exists.
func (m *assetManifest) processedAssetFor(realPath string) (processedAsset, bool) {
	if m == nil {
		return processedAsset{}, false
	}
	pa, ok := m.processed[realPath]
	return pa, ok
}

// register records the content hash of data under urlPath, replacing any prior
// hashed alias (manifest.json is registered twice: raw, then rewritten).
func (m *assetManifest) register(urlPath string, data []byte) {
	sum := sha256.Sum256(data)
	hash := hex.EncodeToString(sum[:])[:assetHashLen]
	if old, ok := m.toHashed[urlPath]; ok {
		delete(m.toReal, old)
	}
	hashed := hashedURL(urlPath, hash)
	m.toHashed[urlPath] = hashed
	m.toReal[hashed] = urlPath
}

// hashedURL inserts a hash segment before the file extension:
// "/main.css" + "abc12345" -> "/main.abc12345.css".
func hashedURL(plain, hash string) string {
	ext := path.Ext(plain)
	return strings.TrimSuffix(plain, ext) + "." + hash + ext
}

// stripAssetHash removes a fingerprint segment of the form ".<assetHashLen hex>"
// sitting immediately before the file extension, recovering the plain asset
// path. Returns the input unchanged when no such segment is present, so plain
// paths and non-fingerprinted names (e.g. "/logo-maskable.svg") pass through.
func stripAssetHash(p string) string {
	ext := path.Ext(p)
	if ext == "" {
		return p
	}
	base := strings.TrimSuffix(p, ext)
	dot := strings.LastIndex(base, ".")
	if dot < 0 {
		return p
	}
	maybeHash := base[dot+1:]
	if len(maybeHash) != assetHashLen {
		return p
	}
	if _, err := hex.DecodeString(maybeHash); err != nil {
		return p
	}
	return base[:dot] + ext
}

// buildAssetManifest walks the static filesystem, content-hashes every file, and
// rewrites manifest.json's icon srcs to their hashed URLs. The two passes matter:
// every plain file must be hashed first so the rewrite can resolve any reference,
// and manifest.json is then re-registered from its rewritten bytes so its own
// hash reflects the new content.
func buildAssetManifest(staticFS fs.FS) (*assetManifest, error) {
	m := &assetManifest{
		toHashed:  make(map[string]string),
		toReal:    make(map[string]string),
		processed: make(map[string]processedAsset),
	}

	contents := make(map[string][]byte)
	err := fs.WalkDir(staticFS, ".", func(p string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}
		data, readErr := fs.ReadFile(staticFS, p)
		if readErr != nil {
			return fmt.Errorf("read %s: %w", p, readErr)
		}
		urlPath := "/" + p
		contents[urlPath] = data
		m.register(urlPath, data)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walk static fs: %w", err)
	}

	const manifestPath = "/manifest.json"
	if raw, ok := contents[manifestPath]; ok {
		rewritten := m.rewriteReferences(raw)
		m.processed[manifestPath] = processedAsset{
			body:        rewritten,
			contentType: "application/manifest+json",
		}
		m.register(manifestPath, rewritten)
	}

	return m, nil
}

// rewriteReferences replaces every plain asset path in body with its hashed URL.
// Longer paths are substituted first so a shorter path can never partially match
// inside a longer one (e.g. "/logo.svg" vs a hypothetical "/logo.svg.map").
func (m *assetManifest) rewriteReferences(body []byte) []byte {
	plains := make([]string, 0, len(m.toHashed))
	for plain := range m.toHashed {
		plains = append(plains, plain)
	}
	sort.Slice(plains, func(i, j int) bool { return len(plains[i]) > len(plains[j]) })

	out := string(body)
	for _, plain := range plains {
		out = strings.ReplaceAll(out, plain, m.toHashed[plain])
	}
	return []byte(out)
}

// uiFilesystems returns the template and static filesystems. In production the
// trees embedded by //go:embed are used so the deployment is a single binary.
// In dev they are read live from disk so template and static edits hot-reload
// without a rebuild; the disk root is the template path's parent ui/ directory.
func uiFilesystems(devMode bool, templatePath string) (fs.FS, fs.FS, error) {
	if !devMode {
		templateFS, err := fs.Sub(embeddedUI, "ui/templates")
		if err != nil {
			return nil, nil, fmt.Errorf("sub embedded templates: %w", err)
		}
		staticFS, err := fs.Sub(embeddedUI, "ui/static")
		if err != nil {
			return nil, nil, fmt.Errorf("sub embedded static: %w", err)
		}
		return templateFS, staticFS, nil
	}

	resolved, err := resolveAndVerifyTemplatePath(templatePath)
	if err != nil {
		return nil, nil, fmt.Errorf("resolve template path: %w", err)
	}
	staticDir := filepath.Join(filepath.Dir(resolved), "static")
	info, statErr := os.Stat(staticDir)
	if statErr != nil || !info.IsDir() {
		return nil, nil, fmt.Errorf("static dir %s does not exist or is not a directory", staticDir)
	}
	return os.DirFS(resolved), os.DirFS(staticDir), nil
}

// setupUI resolves the template and static filesystems for the current mode and
// builds the asset manifest from the static tree.
func setupUI(devMode bool, templatePath string) (fs.FS, fs.FS, *assetManifest, error) {
	templateFS, staticFS, err := uiFilesystems(devMode, templatePath)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("resolve ui filesystems: %w", err)
	}
	assets, err := buildAssetManifest(staticFS)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("build asset manifest: %w", err)
	}
	return templateFS, staticFS, assets, nil
}
