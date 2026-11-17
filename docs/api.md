# API Reference

Antigravity Proxy exposes a native Google Gemini `/v1beta` HTTP interface along with quota monitoring and health endpoints.

## Endpoints Overview

### Data API (Account Key authenticated)

| Method and Path | Description | Authentication |
|---|---|---|
| `GET /health` | Health check & credential presence | None (Public) |
| `GET /models` | Raw Cloud Code `fetchAvailableModels` JSON catalog | Account `API_KEY` |
| `GET /status/limit` | Latest persisted quota snapshot for caller's account | Account `API_KEY` |
| `GET /status/usage` | Historical quota observations for caller's account | Account `API_KEY` |
| `GET /status/account` | Account details for caller's account | Account `API_KEY` |
| `GET /v1beta/models` | Native Gemini model list with keyset pagination | Account `API_KEY` |
| `GET /v1beta/models/{id}` | Native Gemini model details | Account `API_KEY` |
| `POST /v1beta/models/{id}:generateContent` | Native Gemini content generation | Account `API_KEY` |
| `POST /v1beta/models/{id}:streamGenerateContent?alt=sse` | Native Gemini streaming generation (SSE) | Account `API_KEY` |

The Interactions API (`/v1beta/interactions` and its child paths) is not
implemented. After authentication these routes return Gemini JSON with HTTP 501
and instructions to use `generateContent` or `streamGenerateContent`. Clients
must select the `google-generative` protocol rather than Interactions; the proxy
does not translate between those protocols.

### Admin API (Admin API Key authenticated)

| Method and Path | Description | Authentication |
|---|---|---|
| `GET /admin/accounts` | List all configured accounts and statuses | `ADMIN_API_KEY` |
| `GET /admin/accounts/{id}` | Get account details and subscription | `ADMIN_API_KEY` |
| `PATCH /admin/accounts/{id}` | Update account label or enabled status | `ADMIN_API_KEY` |
| `DELETE /admin/accounts/{id}` | Delete account from proxy | `ADMIN_API_KEY` |
| `POST /admin/login-sessions` | Initiate OAuth login session for an account | `ADMIN_API_KEY` |
| `GET /admin/login-sessions/{id}` | Get session status & one-time issued key | `ADMIN_API_KEY` |
| `POST /admin/login-sessions/{id}/complete` | Complete OAuth login session with code/state or redirect URL | `ADMIN_API_KEY` |
| `GET /admin/accounts/{id}/keys` | List API keys for an account | `ADMIN_API_KEY` |
| `POST /admin/accounts/{id}/keys` | Create a new API key for an account | `ADMIN_API_KEY` |
| `DELETE /admin/accounts/{id}/keys/{keyId}` | Revoke an API key | `ADMIN_API_KEY` |
| `GET /admin/accounts/{id}/quota` | Latest quota snapshot for an account | `ADMIN_API_KEY` |
| `GET /admin/accounts/{id}/usage` | Historical quota observations for an account | `ADMIN_API_KEY` |
| `GET /admin/quota` | Multi-account quota overview across all accounts | `ADMIN_API_KEY` |

---

## Authentication

### Data Routes (`/v1beta/*`, `/models`, `/status/*`)
Requests to data routes must include an account-scoped API key. The key determines which Google account serves the request. The key can be passed via:

1. `x-goog-api-key: <KEY>` (standard Gemini header)
2. `?key=<KEY>` query parameter
3. `x-api-key: <KEY>`
4. `Authorization: Bearer <KEY>`

If no keys are configured across any account on localhost/loopback and exactly one account is configured, requests are allowed without keys for convenient local development. When multiple accounts are configured, or when listening on non-loopback interfaces, an API key is required.

### Admin Routes (`/admin/*`)
Administration routes require the `ADMIN_API_KEY`. It can be passed via headers:
- `x-goog-api-key: <ADMIN_KEY>`
- `x-api-key: <ADMIN_KEY>`
- `Authorization: Bearer <ADMIN_KEY>`

*Note:* URL query parameters (`?key=...`) are explicitly rejected on admin routes to prevent secrets from appearing in URLs or logs. If `ADMIN_API_KEY` is not configured on the server, all `/admin/*` routes return `503 Service Unavailable`.

---

## Model Discovery

### `GET /v1beta/models`

Lists all available external generative models dynamically discovered from Google Cloud Code. The list includes Gemini, Claude, and GPT models configured for your account.

Discovery uses the Antigravity CLI client family so Google also returns account-published
Gemini 4 wire IDs. Generation selects that family only for `gemini-4-*`; other models retain
the standard client fingerprint. This changes HTTP client identification, not the Gemini
request body or model ID. Use the exact discovered wire IDs, including tier suffixes.
Entries explicitly marked `disabled: true` by Google are excluded from this native list;
the raw `/models` endpoint continues returning Google's unfiltered response.

Each account caches its catalog for five minutes. A generation request that ends in a
terminal upstream 404 (other than a rejected project) discards that cache immediately,
so a model withdrawn by Google stops being listed on the next request.


**Query Parameters:**
- `pageSize`: Optional integer (1–1000, default: 50).
- `pageToken`: Keyset cursor from a previous `nextPageToken`.

**Example:**
```sh
curl http://127.0.0.1:8080/v1beta/models?pageSize=100 \
  -H "x-goog-api-key: $API_KEY"
```

**Response Format:**
```json
{
  "models": [
    {
      "name": "models/gemini-3.8-flash-tiered",
      "displayName": "Gemini 3.8 Flash (Tiered)",
      "supportedGenerationMethods": ["generateContent", "streamGenerateContent"]
    }
  ],
  "nextPageToken": "..."
}
```

---

## Content Generation

### `POST /v1beta/models/{id}:generateContent`

Generates content synchronously using the native Gemini request/response envelope.

**Example Request:**
```sh
curl http://127.0.0.1:8080/v1beta/models/gemini-3.8-flash-tiered:generateContent \
  -H "x-goog-api-key: $API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "contents": [
      {
        "role": "user",
        "parts": [{"text": "Explain quantum computing in one sentence."}]
      }
    ]
  }'
```

### `POST /v1beta/models/{id}:streamGenerateContent?alt=sse`

Streams responses incrementally using Server-Sent Events (SSE). The `alt=sse` query parameter is required.

**Example Request:**
```sh
curl --no-buffer "http://127.0.0.1:8080/v1beta/models/gemini-3.8-flash-tiered:streamGenerateContent?alt=sse" \
  -H "x-goog-api-key: $API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "contents": [
      {
        "role": "user",
        "parts": [{"text": "Count from 1 to 5."}]
      }
    ]
  }'
```

Each event is delivered as a JSON string prefixed with `data: `, followed by a blank line.

### Multimodal & Video Understanding

Videos and images are passed as native content parts with base64-encoded `inlineData`:

```json
{
  "contents": [
    {
      "role": "user",
      "parts": [
        {
          "inlineData": {
            "mimeType": "video/mp4",
            "data": "<base64_encoded_mp4>"
          }
        },
        {
          "text": "Summarize what happens in this clip."
        }
      ]
    }
  ]
}
```
*Note: Request bodies are capped at 50 MiB, including base64 expansion.*

---

## Quota & Rate Limit Status

### `GET /status/limit`

Returns the latest observed snapshot of your Antigravity quotas. Data is polled in the background from Google's `retrieveUserQuotaSummary` endpoint.

**Example:**
```sh
curl http://127.0.0.1:8080/status/limit \
  -H "x-goog-api-key: $API_KEY"
```

**Response Format:**
```json
{
  "observed_at": "2026-10-07T09:59:38Z",
  "source": "retrieveUserQuotaSummary",
  "buckets": [
    {
      "id": "gemini-5h",
      "pool_display_name": "Gemini Models",
      "window": "5h",
      "display_name": "Five Hour Limit Remaining",
      "status": "available",
      "remaining_fraction": 0.998,
      "reset_at": "2026-10-07T12:20:15Z"
    },
    {
      "id": "gemini-weekly",
      "pool_display_name": "Gemini Models",
      "window": "weekly",
      "display_name": "Weekly Limit Remaining",
      "status": "available",
      "remaining_fraction": 0.998,
      "reset_at": "2026-10-11T16:02:27Z"
    },
    {
      "id": "3p-5h",
      "pool_display_name": "Claude and GPT models",
      "window": "5h",
      "display_name": "Five Hour Limit Remaining",
      "status": "available",
      "remaining_fraction": 0.931,
      "reset_at": "2026-10-07T14:18:50Z"
    },
    {
      "id": "3p-weekly",
      "pool_display_name": "Claude and GPT models",
      "window": "weekly",
      "display_name": "Weekly Limit Remaining",
      "status": "available",
      "remaining_fraction": 0.953,
      "reset_at": "2026-10-13T04:42:09Z"
    }
  ],
  "polling": {
    "interval_seconds": 300,
    "last_attempt_at": "2026-10-07T09:59:37Z",
    "last_success_at": "2026-10-07T09:59:38Z"
  },
  "stale": false
}
```

### `GET /status/usage`

Queries durable historical quota observations persisted in `usage.jsonl`.

**Query Parameters:**
| Parameter | Accepted Values | Default |
|---|---|---|
| `from` | RFC3339 timestamp with timezone | None |
| `to` | RFC3339 timestamp with timezone | None |
| `pool` | `all`, `gemini`, `third_party` | `all` |
| `window` | `all`, `5h`, `weekly` | `all` |
| `limit` | Integer `1`–`1000` | `100` |
| `offset` | Integer `0` or greater | `0` |
| `order` | `asc`, `desc` by observation timestamp | `desc` |

**Example:**
```sh
curl --get "http://127.0.0.1:8080/status/usage" \
  -H "x-goog-api-key: $API_KEY" \
  --data-urlencode "pool=third_party" \
  --data-urlencode "window=weekly" \
  --data-urlencode "limit=10"
```

### `GET /status/account`

Returns details about the configured Google account including email, display name, and active subscription tier. Fails fast with `503` if no account has been configured.

**Example:**
```sh
curl http://127.0.0.1:8080/status/account \
  -H "x-goog-api-key: $API_KEY"
```

**Response Format:**
```json
{
  "credential_configured": true,
  "email": "developer@example.com",
  "name": "Example Developer",
  "subscription": {
    "id": "g1-pro-tier",
    "display_name": "Google AI Pro",
    "source_field": "paidTier"
  },
  "subscription_status": "known"
}
```

The profile is bound to the active account snapshot. Subscription metadata prefers `paidTier`, then `currentTier`; the latter is an access tier, not independent proof of a paid billing plan. Missing provider tier metadata produces `subscription: null` and `subscription_status: "not_reported"`; lookup failures produce `"unavailable"` without hiding a known profile. No account onboarding is performed to display account details.

Quota observations are stored in account-specific journals with hashed identity suffixes. Successful login saves account identity with credentials so restart reopens the same journal. Unlabelled legacy `usage.jsonl` is retained untouched, never attributed to another account. Environment token overrides discard saved profile attribution; read-only identity lookup resolves those credentials before account quota polling begins.

---

## Admin Management API

All endpoints under `/admin/*` require the `ADMIN_API_KEY` header (`x-goog-api-key`, `x-api-key`, or `Authorization: Bearer`). If `ADMIN_API_KEY` is not configured, these endpoints return `503`.

### Account Management

#### `GET /admin/accounts`
Lists all accounts configured on the proxy, including status and live quota observations.

**Response:**
```json
[
  {
    "id": "work",
    "label": "Work Account",
    "email": "work@example.com",
    "name": "Work Developer",
    "enabled": true,
    "credential_configured": true,
    "key_count": 2,
    "quota": { ... },
    "polling": { ... }
  }
]
```

#### `GET /admin/accounts/{id}`
Returns details for a single account including its active subscription tier. Returns `404` if not found.

#### `PATCH /admin/accounts/{id}`
Updates an account's display label or enabled status. Disabled accounts immediately reject their API keys with `403 PERMISSION_DENIED`, prompting AI gateways like Bifrost to rotate to the next key.

**Request Body:**
```json
{
  "label": "New Label",
  "enabled": false
}
```

#### `DELETE /admin/accounts/{id}`
Permanently removes the account and its associated API keys from the configuration. Retains historical quota journals on disk. Returns `204 No Content`.

---

### OAuth Login Sessions

#### `POST /admin/login-sessions`
Initiates a Google OAuth authorization session. Accepts optional target account ID, replace flag, and key issuance specification.

**Request Body:**
```json
{
  "account": "work",
  "replace": false,
  "issue_key": "bifrost"
}
```

| Field | Description |
|---|---|
| `account` | Target account slug (`a-z0-9-`). If omitted, matches an existing account with the same Google ID or derives a slug from email. |
| `replace` | Set `true` to allow re-linking an existing account ID to a different Google account. Otherwise returns `409` on identity mismatch. |
| `issue_key` | Optional key label. When set, automatically mints a new API key upon successful login. |

**Response Format:**
```json
{
  "id": "ls_7f8a9b0c1d2e3f4a",
  "login_url": "https://accounts.google.com/o/oauth2/v2/auth?...",
  "expires_at": "2026-10-13T16:30:00Z"
}
```

#### `GET /admin/login-sessions/{id}`
Checks the status of an ongoing or completed session. If the login completed via the loopback callback with `issue_key` requested, the newly minted API key secret is returned **once** on the first call to this endpoint and then permanently erased from memory.

**Response Format:**
```json
{
  "id": "ls_7f8a9b0c1d2e3f4a",
  "status": "completed",
  "account_id": "work",
  "email": "developer@example.com",
  "name": "Example Developer",
  "key": {
    "id": "key_3f9a1c2e",
    "label": "bifrost",
    "secret": "agp_7x9k2m..."
  }
}
```

#### `POST /admin/login-sessions/{id}/complete`
Manually completes authorization when the browser cannot reach the local loopback callback port (e.g., remote Docker container). Submits `code` and `state`, or the full callback `url`. If `issue_key` was requested at session creation, the response returns the secret key directly.

---

### API Key Management

#### `GET /admin/accounts/{id}/keys`
Lists all API keys configured for an account. Never exposes key secrets or hashes.

**Response:**
```json
[
  {
    "id": "key_3f9a1c2e",
    "label": "bifrost",
    "last4": "2m3f",
    "created_at": "2026-10-13T12:00:00Z",
    "last_used_at": "2026-10-13T12:15:30Z"
  }
]
```

#### `POST /admin/accounts/{id}/keys`
Mints a new cryptographically secure API key for the account. Displays the plaintext secret key once.

**Request Body:**
```json
{
  "label": "open-webui"
}
```

**Response:**
```json
{
  "id": "key_8b2c4d6f",
  "label": "open-webui",
  "key": "agp_k9x2m4p7..."
}
```

#### `DELETE /admin/accounts/{id}/keys/{keyId}`
Revokes an API key. Requests using this key immediately fail with `401 Unauthorized`. Returns `204 No Content`.

---

### Multi-Account Quota Overview

#### `GET /admin/accounts/{id}/models`

Returns the selected enabled account's upstream-published model catalog in the same Gemini
format as `/v1beta/models`, including `pageSize` and `pageToken` pagination. Requires the admin
key; data-plane keys cannot query another account through this route. Each account retains
its own credentials, project and catalog cache. Unknown accounts return 404, disabled accounts
403, and unavailable runtimes 503.

The local helper can inspect every enabled account without changing client routing:

```bash
install -m 755 scripts/agy-models.py ~/.local/bin/agy-models
agy-models                    # Union of enabled accounts, with source account IDs
agy-models --account google2  # One specific account
agy-models --current          # Original client.json API key and its bound account
```

The helper reads `~/.config/antigravity-proxy/client.json` for the local proxy address and
`admin-key` for management authentication. It does not mint keys or change client settings.
Model listing does not combine account permissions: inference still requires the API key of
an account that supports the chosen model. A partial catalog failure is reported and exits
nonzero rather than presenting the remaining accounts as a complete result.

#### `GET /admin/quota`
Returns live quota snapshot across all configured accounts in a single call.

#### `GET /admin/accounts/{id}/quota` and `GET /admin/accounts/{id}/usage`
Inspects live bucket limits and historical observations for a specific account without requiring that account's API key.

## Sanitized Error Diagnostics

When a caller disconnects before any response is sent, the proxy cancels the upstream
request and records HTTP 499 (`status: "CANCELLED"`) in its access log instead of a
gateway error; no upstream failure warning is logged. Upstream timeouts remain warnings
with `failure: "timeout"`.

Non-success upstream HTTP responses retain their HTTP status and the existing
Gemini `error.code`, `error.message`, and `error.status` fields, and add
`error.diagnostic`. For example:

```json
{
  "error": {
    "code": 400,
    "message": "Google upstream request failed",
    "status": "INVALID_ARGUMENT",
    "diagnostic": {
      "origin": "upstream",
      "category": "invalid_request",
      "action": "Check the native Gemini request fields and model capabilities.",
      "fields": ["generationConfig.thinkingConfig.thinkingBudget"]
    }
  }
}
```

Categories include `invalid_request`, `authentication_failed`, `permission_denied`,
`not_found`, `rate_limited`, `client_version_rejected`, and `upstream_error`.
Version rejection is a diagnostic hint, not a trigger for automatic replay.
Each account admits `maxConcurrentGenerations` generation requests at once;
additional requests wait in arrival order for up to 30 seconds before the body is
read. Local generation-capacity 429 responses, returned only after that wait,
instead use `origin: "proxy"` and `category: "generation_capacity_exhausted"`.

Provider error bodies are read with a 16 KiB / 100 ms bound and discarded after
classification. Field paths use an allowlist of native schema segments; arbitrary
property names, descriptions, account/project identifiers, and raw messages are
omitted. Up to 16 distinct paths are retained. Validated Google `RetryInfo` delays
are exposed as `retry_delay_seconds` (rounded up, at most seven days), and supply
`Retry-After` only when no valid upstream header exists. Malformed, oversized or
slow error bodies fall back to status-only classification. None of this changes
successful response bodies or committed streams.

The admin `GET /admin/accounts/{id}/models` response adds `client_identity` alongside
the usual `models` and `nextPageToken`. It contains `discovery_family`,
`standard_version`, `cli_version`, and a `generation_families` map keyed by model
name for the current page. Data-key model routes do not include this management
metadata. The values describe configured transport identity, not model health.
