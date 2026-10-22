# FSFW porting contract

Internal contract between the Go API (`backend/`) and the Vue UI (`frontend/`).
Both sides are changed independently; keep this file authoritative.

## Product

- Short name: **FSFW**
- Full name: **fpkstudio for Windows**
- Binary / process: `fsfw.exe` (Tauri shell), `fsfw-backend.exe` (Go API)
- Data root: `%USERPROFILE%\Documents\fpkstudio` (OneDrive-redirected Documents
  resolved by the Tauri layer, passed to the backend explicitly)
- Install root: `%LOCALAPPDATA%\fpkstudio` (per-user MSI)
- No fnOS / `trim.*` open-API calls, no directory authorization, no unix socket.

## Process model

The Tauri shell starts the Go API as a child process:

```
fsfw-backend.exe --host 127.0.0.1 --port 0 --token <hex> \
  --data-dir "C:\Users\me\Documents\fpkstudio" \
  --runtime-file "...\run\server.json" \
  --parent-pid <pid>
```

The backend binds a random loopback port and atomically writes
`run/server.json`:

```json
{
  "port": 51234,
  "token": "…",
  "pid": 1234,
  "version": "1.0.0",
  "dataDir": "C:\\Users\\me\\Documents\\fpkstudio",
  "startedAt": "2026-10-22 22:00:00"
}
```

The shell deletes a stale `server.json` before spawning the backend, then polls
the file until it appears (20 s timeout), then loads the UI and injects the
config into the page **before any page script runs**:

```js
window.__FSFW__ = {
  apiBase: "http://127.0.0.1:51234",
  token: "…",
  version: "1.0.0",
  dataDir: "C:\\Users\\me\\Documents\\fpkstudio"
}
```

Browser-only dev (`npm run dev` without Tauri) falls back to
`VITE_FSFW_API_BASE` / `VITE_FSFW_TOKEN`.

## HTTP API

All routes are `http://127.0.0.1:<port>/api/...`. Every request must carry the
token, either:

- header `X-FSFW-Token: <token>` (JSON/XHR calls), or
- query parameter `?token=<token>` (downloads, `<img>`, EventSource).

`OPTIONS` preflight returns 204 with CORS headers. Allowed origins:
`http://tauri.localhost`, `https://tauri.localhost`, `tauri://localhost`, and —
only with `--dev` / `FSFW_DEV=1` or an empty token — `http://localhost:<any>` /
`http://127.0.0.1:<any>` (Vite dev). When `--token` is empty (plain browser dev)
authentication is disabled.

### Removed from fpkstudio

- `POST /api/trim/*` (folders / ACL / semantic)
- `GET /api/settings` fields `user`, `trim`
- `GET /api/system/info` fields `socket`, `baseURL`, `pidFile`
- static file serving, `go:embed`, `baseURL`, unix socket, PID file

### Added

- `POST /api/system/reveal` `{ "path": "<absolute path>" }` → open the path in
  Windows Explorer (`explorer.exe`). Returns `{ "ok": true }`.

### Settings

`GET /api/settings`:

```json
{
  "dirs": {
    "projectRoot": "C:\\Users\\me\\Documents\\fpkstudio\\projects",
    "outputDir": "C:\\Users\\me\\Documents\\fpkstudio\\output",
    "projectRootFromUser": false,
    "outputDirFromUser": false,
    "defaultProjectRoot": "C:\\Users\\me\\Documents\\fpkstudio\\projects",
    "defaultOutputDir": "C:\\Users\\me\\Documents\\fpkstudio\\output",
    "outputEnvIgnored": false
  },
  "dataDir": "C:\\Users\\me\\Documents\\fpkstudio"
}
```

`PUT /api/settings` accepts `{ "projectRoot"?: string, "outputDir"?: string }`;
an empty string restores the default. Same response shape.

`POST /api/projects/import-path` keeps working with an absolute Windows path;
the fnOS ACL check is gone.

## Environment variables

Prefix `FSFW_` (was `FPKSTUDIO_`):

| Variable | Meaning |
| --- | --- |
| `FSFW_DATA_DIR` | data root (default `Documents\fpkstudio` on Windows) |
| `FSFW_PROJECTS_DIR` | projects root (default `<data>/projects`) |
| `FSFW_OUTPUT_DIR` | package output (default `<data>/output`) |
| `FSFW_FNPACK` | fnpack executable (default `fnpack`) |
| `FSFW_LOG_LEVEL` | `debug` / `info` / `warn` / `error` |
| `FSFW_LOG_RING` | in-memory log ring size |
| `FSFW_TOKEN` | API token, same as `--token` |
| `FSFW_PARENT_PID` | watchdog parent PID, same as `--parent-pid` |
| `FSFW_HOST` | listen host, same as `--host` (loopback only) |
| `FSFW_PORT` | listen port, same as `--port` (0 = system-assigned) |
| `FSFW_RUNTIME_FILE` | runtime JSON path, same as `--runtime-file` |
| `FSFW_DEV` | `1` = dev mode, same as `--dev` (allow any localhost CORS origin) |

## Frontend host layer

`src/host/index.ts` replaces `@trimjs/web-app` and `src/trim/*`:

```ts
export function isDesktopHost(): boolean
export function isHostAvailable(): boolean        // alias, true in Tauri
export async function openInFileManager(path: string): Promise<void>
export async function pickUserDirectory(title: string): Promise<string[]>
export async function pickUserFiles(title: string, accept: string[]): Promise<string[]>
export function hostErrorMessage(err: unknown): string
```

- `openInFileManager` → `POST /api/system/reveal` (works in dev too).
- pickers → `@tauri-apps/plugin-dialog`; outside Tauri they reject with
  `HostUnavailableError` and callers show the localized hint.
- theme follows `prefers-color-scheme` only; no host theme bridge.
- `src/utils/semantic.ts`, `src/trim/**`, `scripts/check-semantic.mjs` are deleted.
