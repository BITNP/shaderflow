# ShaderFlow backend HTTP and WebSocket API

Resolve production secret references and configure valid certificate files, then run `go run ./cmd/shaderflow -config server.example.json -preflight` (development `server.dev.example.json` intentionally permits the localhost HTTP path and reports `tls.optional`). It prints a machine-readable report for required static assets, TLS files, parsed certificate/key match, certificate validity window and DNS names, and Archive writability, exiting non-zero if any check fails. Start normally only after it reports `ready: true`. On a LAN use HTTPS (`tlsCert`, `tlsKey`); WebGPU requires a secure browser context. Only localhost development should use plain HTTP. `staticDir` is served from `/` when present; configure its path to the built frontend assets. Fresh server processes begin in `idle` (archives remain on disk under `dataDir`). Startup probes archive writability before listening and fails closed when the probe cannot create/remove a protected temporary file. A runtime write failure permanently degrades that process's Archive adapter and the active Game state, blocks later authoritative mutations, and is exposed by `GET /api/health` as HTTP 503 plus by admin state as `degraded/storageError`. The HTTP server applies read-header, read, write and idle deadlines.

## Authentication

- `POST /api/admin/login` with `{"password":"..."}` rotates to a new opaque HttpOnly SameSite=Strict session cookie and returns only `{ "role": "admin" }`. Never send password over plaintext LAN HTTP.
- `GET /auth/login` redirects to OIDC authorization, and `/auth/callback` checks issuer, audience, signature, state and nonce. Successful callback creates an opaque HttpOnly player session and redirects to `/player/`; no credential is placed in the URL or browser storage. `GET /api/auth/me` returns `{sub,name,role}`.
- When `enableTestPlayerLogin` is explicitly true (development only), `POST /api/test/player-login` with `{"name":"..."}` issues a temporary player token. This endpoint is not registered in production configuration.
- Browser APIs and WebSocket authenticate only through the opaque session cookie; legacy bearer/HMAC credentials are no longer accepted. Sessions have a 12-hour absolute expiry and 2-hour idle expiry, are atomically persisted in `dataDir/sessions.json`, and survive a clean process restart. Creating a player session for the same OIDC subject atomically revokes its previous player session, enforcing one active player device; administrator sessions are unaffected. `POST /api/auth/logout` revokes the current session, persists the revocation, and clears the cookie. Admin and player identities are distinct.
- Non-GET mutations with a cross-origin `Origin` are rejected. Responses include `nosniff`, `no-referrer`, and clickjacking protection headers. Admin login accepts bcrypt `adminPasswordHash`; plaintext `adminPassword` is development-only. Failed admin logins are limited to five attempts per client address per minute.

## Production configuration

With `environment: "production"`, startup rejects test player login, missing TLS, plaintext `adminPassword`, inline production secrets, a non-bcrypt resolved administrator hash, or missing OIDC issuer/client ID. Production secrets use `adminPasswordHashRef` and `oidc.clientSecretRef`, each selecting exactly one `{ "env": "NAME" }` or `{ "file": "/run/secrets/name" }`; file values are trimmed and empty/missing references fail startup. Production also requires `publicUrl` to be an HTTPS origin, and requires the OIDC redirect URL to use the same origin. Preflight verifies the certificate SAN matches the `publicUrl` hostname. `config.example.json` remains a localhost development example; `server.example.json` is the production template.

## Archive diagnosis and repair

Use `go run ./cmd/shaderflow-archive -data data -round <roundId>` for a read-only report covering event JSONL continuity/truncated tails and content-addressed blob hashes. It exits non-zero for an unhealthy archive. After preserving a backup and reviewing the report, `-repair` explicitly truncates only an invalid JSONL tail at the last newline, marks the Round complete, updates event statistics, and rebuilds the Catalog. It never invents missing events or repairs sequence gaps/blob corruption.

## State and routes

`GET /api/state` yields `{phase, settings, graph?, registrations, players?, ownNode?, codes?, seq}`. `phase`: `idle`, `registration`, `graph`, `running`. Admin sees registrations, player assignments (`players`), full graph and current latest code per node (`codes`). Player sees only their own registration, `ownNode`, their own latest `ownCode`, and the active `trajectory` during running; no graph or other players' sources. `ownCode` is the player's Accepted Shader Version and allows reconnect recovery without implicitly uploading a generated template. `trajectory` is `{version,name,initSource,frameSource,validatorVersion,updatedAt,active}`; opting in does not replace the player's local editor or local time origin. Registration contains `{sub,name,nodeId?,times}`; `times` counts started rounds in this process. `ownNode` has `{id,inputs:[sourceTextureId],outputs:[textureId]}`.

| Method / route | Body | Meaning |
| --- | --- | --- |
| `POST /api/admin/round` | `settings` JSON | Begin enrollment, replacing prior draft; no running round may be replaced. |
| `POST /api/register` | none | OIDC player registers, idempotent if already registered; closes at capacity. |
| `DELETE /api/admin/registrations/{sub}` | none | Remove registration before start, invalidate draft graph; may re-register. |
| `POST /api/admin/graph/generate` | none | Generate connected valid graph after all registered. |
| `PUT /api/admin/graph` | `graph` JSON | Replace a valid draft graph; node count fixed, external input count and bindings may change. |
| `POST /api/admin/start` | none | Check all graph terminals and start with randomized node assignments. |
| `POST /api/code` | `{ "version": 1, "code": "..." }` | Assigned player submits locally validated code, strictly increasing per-player client version. Response `{ok:true,version}`. |
| `PUT /api/admin/inputs/{inputId}` | multipart `file`, `version`, `resizeMode` | Store a PNG/JPEG as a content-addressed Round blob and publish a versioned authoritative External Input reference. |
| `POST /api/admin/snapshots` | multipart image plus `id`, `textureId`, dimensions, frozen time and version manifest | Persist a PNG/JPEG Render Snapshot blob and append `snapshot.created`. |
| `PUT /api/admin/time-control` | `{version,action,timeMs}` | Publish a versioned `pause`, `resume`, or `reset` event for archive/replay and connected clients. |
| `PUT /api/admin/trajectory` | `{version,name,initSource,frameSource,validatorVersion,active}` | Publish or deactivate the versioned Camera Program. Active versions appear in player snapshots; each player keeps a local time origin. |
| `POST /api/admin/terminate` | none | End running round and return to enrollment retaining registrations/history and settings, clearing graph/assignments/code/trajectory. |

Settings schema: `{players,inputCount,minInputs,maxInputs,minOutputs,maxOutputs,width,height,format,blockSize,maxCodeBytes,compileDebounceMs,passTimeoutMs}`. Defaults are `2,1,1,2,1,2,960,540,"rgba32float",8,65536,500,1000`. Provide all fields when creating a round. `players` 1–10, textures min 200×100 max 3840×2160; formats `rgba32float`, `rgba16float`, `rgba8unorm`; block sizes 4, 8 or 16. Code validation is deliberately limited to identity, game state, version, size and ASCII WGSL tokens (Unicode comments/text allowed); browser validates compilation and one dispatch before submission. Server cannot compile WGSL. A failed browser compile must not be uploaded: the latest good server version stays active.

Graph JSON is directly consumable by the renderer:

```json
{"inputs":["input0"],"nodes":[{"id":"n0","inputs":["input0"],"outputs":["n0:0"]}],"output":"n0:0"}
```

Each `inputs` entry on a node names one upstream texture; distinct node output names identify textures that may be read by multiple downstream bindings. All external images and all shader outputs must be consumed and reach the unique final output. All shader nodes require inputs and outputs; cycles, unknown texture IDs, disconnected/dead terminals or direct external-input-to-final-output are rejected. The graph generator may raise bindings outside admin min/max to meet connectivity; always render the actual returned counts.

## Archive and replay routes

Admin-only routes:

- `GET /api/admin/archives`: list the persistent `dataDir/catalog.json` catalog, including active/completed/legacy/corrupt status and event/checkpoint/blob statistics. The catalog is atomically replaced and rebuilt from manifests if missing or unreadable.
- `GET /api/admin/archives/{roundId}`: read a schema v1 manifest, start snapshot and ordered durable events.
- `GET /api/admin/archives/{roundId}/state?through=<seq>`: apply the pure replay reducer through the requested event sequence. Omitting `through` returns the latest reducible logical state.
- `GET /api/admin/archives/{roundId}/blobs/{sha256}?size=<bytes>&contentType=<mime>`: read and verify a content-addressed Round blob.
- `POST /api/admin/archives/gc` with `{ "dryRun": true, "minimumAgeHours": 24 }`: scan completed Rounds, preserve blobs referenced by start state or durable input/snapshot events, skip active Rounds, and report or delete old orphans. A deleting run repairs Catalog blob counts and bytes.

The replay reducer covers Accepted Shader Version, authoritative External Input references, Camera Program updates, versioned pause/resume/reset time controls, and termination phase. Input blobs are SHA-256 content-addressed and checked against recorded size/hash when read. The admin replay panel can select a schema v1 Round and event boundary, reconstruct the logical state, load verified input blobs, reinstall every Accepted Shader Version into an isolated WebGPU Renderer, evaluate the recorded Camera Program at the recorded time, and render the historical Final Output to a dedicated canvas. Missing shader facts are rejected rather than replaced with fallback output. The replay panel also supports previous/next Round Event stepping, timestamp-paced play/pause, 0.5×/1×/2×/4× speed, and displays the current event sequence/type/time. Long event gaps are capped so review remains interactive. Schema v1 archives now persist a logical state checkpoint every 50 Round Events under `checkpoints/<20-digit-seq>.json`. State replay chooses the nearest checkpoint at or before `through` and applies only later events. Missing or unreadable checkpoint files are ignored so `start.json` plus durable events remain the source-of-truth fallback. Incremental Renderer reuse and arbitrary wall-clock interpolation remain later archive work.

## WebSocket protocol

Connect same-origin `GET /api/ws?since=<lastSeq>` with the auth cookie (or `&token=<token>`). `since=0` or omitted causes `{ "type":"snapshot", "seq":42, "state":{...} }`. Valid recent `since` causes replay of event envelopes `{ "type":"event", "seq":43, "event":{ "type":"graph.updated", "at":"...", "data":{...} } }`. Admin sees `code.updated` with `{nodeId,code:{version,code,updatedAt,sub}}`. Player events are redacted to state snapshots containing only the player's own information. Reconnect with last received sequence. If the bounded in-memory replay window (4096 events) has expired, server falls back to a snapshot. Server sends WebSocket ping frames every 25 seconds; clients should respond with pong (browser clients do automatically) and reconnect on close. Always use `seq` to discard stale/out-of-order messages. Fetch `/api/state` after reconnect if a local UI needs a fresh full state regardless of event history.

## Persistence

Each started round creates `dataDir/YYYYMMDD-HHMMSS.nanoseconds/` with a versioned `manifest.json` (`schemaVersion:1`, round ID, timestamps and active/complete status), `start.json` (graph, assignments, settings), append-only `events.jsonl`, and `codes/<node>-<clientVersion>-<timestamp>.json` with every accepted full source. Event payloads include accepted full code. A crash can leave the final JSONL record incomplete. Historic archives are scanned on demand: directories without a manifest are reported as `legacy`, invalid manifests as `corrupt`, and schema v1 archives can be listed/read and reduced to logical state at a requested event sequence. Player-local test images and player exports remain browser-local. If an authoritative input or Render Snapshot blob is stored but its subsequent Round Event is rejected, the server removes that newly stored blob to avoid an unreferenced upload. Admin saves during a running Round are uploaded as content-addressed PNG/JPEG Render Snapshot blobs and recorded with texture ID, dimensions, frozen time, source event sequence, Accepted Shader Version map, External Input version map, Camera Program version and TimeControl version.
