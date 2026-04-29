# Caddy with Zeabur extensions

It supports these Zeabur extensions:

- `_headers` file
- `_redirects` file

## Usage

```bash
docker build -t zeabur/caddy-static .
docker run -p 8080:8080 -v $(pwd)/examples/caddy:/usr/share/caddy -it zeabur/caddy-static
```

## Publish

```bash
MAJOR=1 MINOR=0 PATCH=0 bash build.sh
```

## Caddyfile behavior

The server listens on `:8080` and serves files from `/usr/share/caddy`. It auto-detects
whether the site is an SPA or an MPA based on the presence of `/404.html` in the site root.

### File serving (`try_files`)

Requests are matched in order:
1. The path as-is (e.g. `/about.html`)
2. The path with a `.html` suffix (e.g. `/about` → `/about.html`)
3. A directory index (e.g. `/users/` → `/users/index.html`)

If none of the above match, a 404 is emitted and the error handler takes over.

### 404 error handling

The `handle_errors 404` block decides what to do with unmatched requests:

**Asset requests** — paths whose final segment ends with a non-`.html`/`.htm` extension
(matched by `\.[A-Za-z0-9]+$`) always receive a plain-text `404 Not Found` response.
They never fall back to `index.html` or `404.html`. This prevents broken `<script>` and
`<link>` loads from silently serving HTML.

**Document requests** — everything else:

| Site has `/404.html`? | Mode | Response |
|---|---|---|
| Yes | **MPA** | Serve `/404.html` with HTTP status 404 |
| No | **SPA** | Serve `/index.html` with HTTP status 200 |

The SPA status-200 return is intentional: client-side routers need a 200 to bootstrap.

### Sensitive paths

Requests matching `/.git/*`, `/node_modules/*`, `/vendor/*`, or `/.venv/*` are blocked
and return 404 regardless of whether the files exist on disk.

### Encoding

Responses are compressed with gzip or zstd based on the client's `Accept-Encoding` header.

### Known edge cases

- **`/api/v1.2` (or any path whose final segment ends in a digit after a dot)** — the
  asset regexp `\.[A-Za-z0-9]+$` matches the trailing `.2`, so this is classified as an
  asset miss and returns a plain 404 rather than the SPA/MPA fallback. If your SPA routes
  contain dotted numeric segments in the final position, avoid this pattern.

- **`/FILE.HTML` (uppercase extension)** — Caddy's `not path *.html *.htm` matcher is
  case-insensitive on case-insensitive filesystems but may not be on Linux. Test your
  deployment if you serve mixed-case HTML paths.

## Test

The E2E tests require the `zeabur/caddy-static` Docker image. Build it first:

```bash
docker build -t zeabur/caddy-static .
```

Then run the full suite:

```bash
go test -v ./e2etest
```

### Test inventory

#### Extension integration (`e2e_test.go`, fixture: `examples/caddy`)

| Test | Request | Expected |
|---|---|---|
| `TestRedirects` | `GET /` | 302 → `/home` (via `_redirects`) |
| `TestHeader` | `GET /test.html` | 200, `X-Caddy-Test-Passed: true` (via `_headers`) |
| `TestUnsafePath` | `GET /vendor/unsafe_path` | 404 |
| `TestMpaNotFound` | `GET /invalid_path` | body contains `404 page not found` |
| `TestRedirectToExternalUrl` | `GET /google` | 302 → `https://google.com` |

#### `TestSPA` — SPA mode (no `404.html` in site root)

**A — `try_files` hits (real files served directly)**

| Test | Request | Expected |
|---|---|---|
| A1 | `GET /` | 200, body `SPA_INDEX` |
| A2 | `GET /index.html` | 200, body `SPA_INDEX` |
| A3 | `GET /about` | 200, body `ABOUT_PAGE` (`.html` suffix match) |
| A4 | `GET /about.html` | 200, body `ABOUT_PAGE` |
| A5 | `GET /users/` | 200, body `USERS_INDEX` (directory index match) |
| A6 | `GET /users` | 200 or 308→`/users/`, final body `USERS_INDEX` |
| A7 | `GET /blog/` | 200, body `BLOG_INDEX` |
| A8 | `GET /blog/post-1` | 200, body `POST_1` |
| A9 | `GET /blog/post-1.html` | 200, body `POST_1` |
| A10 | `GET /data.json` | 200, `Content-Type: application/json`, body `{"real":true}` |
| A11 | `GET /assets/app.js` | 200, `Content-Type: *javascript*`, body `REAL_ASSET_JS` |
| A12 | `GET /assets/style.css` | 200, `Content-Type: text/css`, body `REAL_ASSET_CSS` |
| A13 | `GET /img/logo.png` | 200, `Content-Type: image/png` |
| A14 | `GET /.well-known/security.txt` | 200, body `WELL_KNOWN` |

**B — Sensitive path blocking**

| Test | Request | Expected |
|---|---|---|
| B1 | `GET /.git/config` | 404, body ≠ `SHOULD_NEVER_LEAK_GIT` |
| B2 | `GET /.git/HEAD` | 404 |
| B3 | `GET /node_modules/pkg/index.js` | 404, body ≠ `SHOULD_NEVER_LEAK_NM` |
| B4 | `GET /vendor/lib.php` | 404, body ≠ `SHOULD_NEVER_LEAK_VENDOR` |
| B5 | `GET /.venv/pyvenv.cfg` | 404, body ≠ `SHOULD_NEVER_LEAK_VENV` |
| B6 | `GET /.git` (no trailing content) | any status, body ≠ `SHOULD_NEVER_LEAK_GIT` (design note: `path /.git/*` does not match bare `/.git`) |
| B7 | `GET /.git/` (trailing slash) | 404 |
| B8 | `GET /any/.git/config` (mid-path) | logged only — `@forbidden` is prefix-anchored so this is not blocked |

**C — SPA fallback (missing document → `index.html` + 200)**

| Test | Request | Expected |
|---|---|---|
| C1 | `GET /projects` | **200**, `Content-Type: text/html`, body `SPA_INDEX` |
| C2 | `GET /projects/` | 200, body `SPA_INDEX` |
| C3 | `GET /projects/123` | 200, body `SPA_INDEX` |
| C4 | `GET /deeply/nested/spa/route` | 200, body `SPA_INDEX` |
| C5 | `GET /projects?id=1&filter=foo` | 200, body `SPA_INDEX` |
| C6 | `GET /some-page.html` (non-existent) | 200, body `SPA_INDEX` (`.html` exempt from asset rule) |
| C7 | `GET /some-page.htm` (non-existent) | 200, body `SPA_INDEX` |
| C8 | `GET /-_~!$&()*+,;=:@` | 200, body `SPA_INDEX` |
| C9 | `HEAD /projects` | 200, empty body |

**E — Missing asset → plain 404 (not SPA/MPA fallback)**

All 20 paths return 404 with plain-text `Not Found`, `Content-Type` ≠ `text/html`, body ≠ `SPA_INDEX`:

`/assets/missing.{js,mjs,css,css.map}` · `/img/missing.{png,jpg,svg,webp,avif,ico}` · `/fonts/missing.{woff2,woff,ttf}` · `/missing.{json,xml,txt,pdf,mp4,wasm,zip}`

**F — Asset/document classification boundary**

| Test | Request | Classification | Expected |
|---|---|---|---|
| F1 | `GET /file.` (trailing dot, no ext chars) | document | 200, body `SPA_INDEX` |
| F2 | `GET /article-2024` (no dot) | document | 200, body `SPA_INDEX` |
| F3 | `GET /api/v1.2` (numeric ext) | **asset** ⚠ | 404, body ≠ `SPA_INDEX` (known: `.2` matches `[A-Za-z0-9]+$`) |
| F4 | `GET /v1.0.0/page` (dot in non-final segment) | document | 200, body `SPA_INDEX` |
| F5 | `GET /file.tar.gz` (double ext) | asset | 404, body ≠ `SPA_INDEX` |
| F6 | `GET /file..js` (double dot) | asset | 404, body ≠ `SPA_INDEX` |
| F7 | `GET /FILE.HTML` (uppercase) | logged — case sensitivity is filesystem-dependent | |
| F8 | `GET /.hidden` (dotfile, no slash) | asset | 404, body ≠ `SPA_INDEX` |
| F9 | `GET /路徑/中文` (Unicode, percent-encoded) | document | 200, body `SPA_INDEX` |

**G — URL and path edge cases**

| Test | Request | Expected |
|---|---|---|
| G3 | `GET /../etc/passwd` | any, body ≠ `root:` if 200 |
| G4 | `GET /foo/../about` | logged (Caddy normalises path) |
| G7 | `GET /` + 4 KB path | logged — no crash required |
| G8 | `GET /` + `Range: bytes=0-9` | 206 or 200 |
| G9 | `GET /` + `If-None-Match: <etag>` | 304 (skipped if no ETag returned) |

**H — HTTP methods**

| Test | Request | Expected |
|---|---|---|
| H1 | `HEAD /` | 200, empty body |
| H2 | `HEAD /projects` | 200, empty body |
| H4 | `HEAD /assets/missing.js` | 404, empty body |

**I — Response headers**

| Test | Request | Expected |
|---|---|---|
| I1 | `GET /assets/app.js` + `Accept-Encoding: gzip` | `Content-Encoding: gzip`, decompressed body `REAL_ASSET_JS` (skipped if file too small) |
| I3 | `GET /assets/app.js` (no Accept-Encoding) | no `Content-Encoding` header |
| I4 | `GET /` | 200, `Content-Type: text/html`, has `ETag` or `Last-Modified` |
| I5 | `GET /data.json` | 200, `Content-Type: application/json` |
| I6 | `GET /img/logo.png` | 200, `Content-Type: image/png`, `Accept-Ranges: bytes` |
| I8 | `GET /assets/missing.js` | 404, `Content-Type` ≠ `text/html` |
| I9 | `GET /projects` | 200, `Content-Type: text/html` |

**J — Regression (bugs fixed by the current Caddyfile)**

| Test | Request | Expected | Regression |
|---|---|---|---|
| J1 | `GET /projects` | **200**, body `SPA_INDEX` | `handle_errors` used to inherit `=404` status |
| J3 | `GET /assets/missing.js` | **404**, plain `Not Found`, ≠ HTML | missing assets used to fall back to `index.html` at 200 |

---

#### `TestMPA` — MPA mode (`404.html` present in site root)

**A — `try_files` hits** — identical to SPA A1–A14 (existing files always served directly).

**B — Sensitive path blocking** — identical to SPA B1–B8.

**D — MPA fallback (missing document → `404.html` + 404)**

| Test | Request | Expected |
|---|---|---|
| D1 | `GET /projects` | **404**, `Content-Type: text/html`, body `CUSTOM_404` |
| D2 | `GET /projects/123` | 404, body `CUSTOM_404` |
| D3 | `GET /deeply/nested/missing` | 404, body `CUSTOM_404` |
| D4 | `GET /missing.html` (non-existent) | 404, body `CUSTOM_404` (`.html` exempt from asset rule) |
| D5 | `GET /missing.htm` (non-existent) | 404, body `CUSTOM_404` |
| D6 | `GET /404.html` (direct request) | **200**, body `CUSTOM_404` (real file hit, not fallback) |
| D8 | `HEAD /projects` | **404**, empty body |

**E — Missing asset** — identical to SPA E (plain 404, body ≠ `CUSTOM_404`).

**F — Asset/document boundary**

Same classification logic as SPA. Document misses show `CUSTOM_404` at 404; asset misses return plain 404 without `CUSTOM_404`.

**G — URL and path edge cases** — G3, G7, G9 (same assertions as SPA).

**H — HTTP methods**

| Test | Request | Expected |
|---|---|---|
| H1 | `HEAD /` | 200, empty body |
| H3 | `HEAD /projects` | **404**, empty body |
| H4 | `HEAD /assets/missing.js` | 404, empty body |

**I — Response headers**

| Test | Request | Expected |
|---|---|---|
| I4 | `GET /` | 200, `Content-Type: text/html`, has `ETag` or `Last-Modified` |
| I8 | `GET /assets/missing.js` | 404, `Content-Type` ≠ `text/html` |
| I10 | `GET /projects` | **404**, `Content-Type: text/html` |

**J — Regression**

| Test | Request | Expected | Regression |
|---|---|---|---|
| J2 | `GET /projects` | **404**, body `CUSTOM_404` | `try_files` used to rewrite to `404.html` and serve it at 200 |
| J4 | `GET /assets/missing.js` | **404**, plain `Not Found`, ≠ `CUSTOM_404` | missing assets used to fall back to `404.html` |
