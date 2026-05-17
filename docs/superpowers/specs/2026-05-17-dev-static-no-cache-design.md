# Dev-mode static asset no-cache

## Problem

The static file server applies `Cache-Control: public, max-age=31536000, immutable`
to every file under `ui/static/` in both dev and prod (`cmd/web/middleware.go:88`
`cacheForever`, applied at `cmd/web/handler-fileserver.go:74`). In production this
is correct because `main.css` and `main.js` are md5-fingerprinted by the
`ui-builder` Dockerfile stage and `base.gohtml` is rewritten via `sed` to
reference the hashed names.

In dev there is no fingerprinting, so editing `ui/static/main.css` and refreshing
the browser still serves the previously cached body — `immutable` instructs the
browser to skip revalidation entirely. The dev loop documented in the top-level
`CLAUDE.md` ("Editing a template and refreshing the browser is the whole dev
loop") is broken for CSS/JS.

## Goal

In dev, every request for a static asset returns the current file from disk with
no browser caching. Production behavior is unchanged.

## Non-goals

- Changing the Dockerfile or how production fingerprints assets.
- Fingerprinting assets beyond `main.css` and `main.js`.
- File watching, live reload, or any client-side dev tooling.
- Reconciling the dev/prod paths into a single asset pipeline.

## Design

### Mode selection

Use the existing `app.devMode` flag (`cmd/web/main.go:39`, set to
`cfg.FlyAppName == ""` at `main.go:180`). The static file server middleware is
selected once at handler construction, not per request.

### Middleware

In dev the file server uses the existing `noStore` middleware from
`cmd/web/middleware.go:107`, which sets:

```
Cache-Control: no-store, max-age=0, must-revalidate
Vary: Cookie, Accept
```

`Vary: Cookie, Accept` is harmless on static assets — the response body does not
vary on either, and `no-store` already prevents shared caches from storing the
response. Reusing `noStore` keeps middleware surface area flat; we are not
adding a new `noStoreStatic` helper.

In prod the file server continues to use `cacheForever` exactly as today.

### Wiring

`fileServerHandler` in `cmd/web/handler-fileserver.go` selects the middleware
based on `app.devMode`:

```go
cache := cacheForever
if app.devMode {
    cache = noStore
}
return app.noAuthStack(cache(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
    // ...existing interceptor body...
}))), nil
```

The notFoundInterceptor comment in the same file mentions `cacheForever` by
name; update it to "the cache middleware (cacheForever in prod, noStore in dev)"
so a future reader is not misled.

### Why `no-store` and not `no-cache` + ETag

`no-store` is simpler and deterministic: browsers may not cache the response
at all, so the dev loop never depends on conditional-GET semantics. `no-cache`
+ ETag would also work (`http.FileServer` already emits `Last-Modified`), but
round-trip cost on localhost is irrelevant and the extra moving parts buy
nothing here.

## Testing

Extend `cmd/web/handler-fileserver_test.go` with a check that a successful
static-file GET in the test environment returns
`Cache-Control: no-store, max-age=0, must-revalidate`.

The test environment leaves `FLY_APP_NAME` unset, so `app.devMode` is `true`
in tests — the dev path is what gets exercised. A separate prod-path assertion
would require either injecting `devMode` into the test app or setting
`FLY_APP_NAME` and is out of scope; `cacheForever` is unchanged code and
remains covered by its existing usage.

The existing `Test_fileServer_servesExistingFile` and
`Test_fileServer_missingFileReturnsCustom404` cases stay green — neither
asserts the cache header today.

## Risk and rollout

Single small change behind an existing flag, dev path only. No production
behavior changes. Standard `make ci` is sufficient validation.
