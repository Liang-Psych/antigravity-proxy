# Configuration Guide

Antigravity Proxy can be configured via a JSON configuration file, environment variables, or a combination of both. Environment variables take precedence over file-based values.

---

## Configuration File

The default configuration file is located at:
```text
~/.config/antigravity-proxy/config.json
```

On Unix systems, this file must have owner-only permissions (`chmod 600`). The proxy rejects files that are world- or group-readable.

### Full Schema & Example

```json
{
  "host": "127.0.0.1",
  "port": 8080,
  "adminKeyHash": "sha256:d8a2...3f1c",
  "accounts": [
    {
      "id": "work",
      "label": "Work Account",
      "enabled": true,
      "refreshToken": "1//04...",
      "oauthClientId": "107100...",
      "oauthClientSecret": "GOCSPX-...",
      "accountId": "1123456789...",
      "accountEmail": "developer@company.com",
      "accountName": "Jane Developer",
      "projectId": "",
      "keys": [
        {
          "id": "key_3f9a1c2e",
          "label": "bifrost",
          "hash": "sha256:4b8e...",
          "last4": "2m3f",
          "createdAt": "2026-10-13T12:00:00Z"
        }
      ]
    }
  ],
  "clientVersion": "2.32.0",
  "cliClientVersion": "1.3.4",
  "discoveryClientFamily": "cli",
  "generationClientRules": [{"modelPrefix": "gemini-4-", "family": "cli"}],
  "maxConcurrentGenerations": 2,
  "quotaPollIntervalSeconds": 300,
  "quotaHistoryMaxSamples": 10000,
  "quotaHistoryPath": "/home/your-user/.local/share/antigravity-proxy/usage.jsonl",
  "dailyEndpoint": "https://daily-cloudcode-pa.googleapis.com",
  "prodEndpoint": "https://cloudcode-pa.googleapis.com"
}
```

### OAuth Recovery

If Google rejects a request with HTTP 401, the proxy refreshes OAuth credentials
and retries the same request once, before returning any upstream response to the
client. Concurrent requests reuse an already refreshed token. The recovery limit
applies across both upstream endpoints. Explicit access tokens are not refreshed;
403 responses, errors inside an accepted stream, and interrupted streams do not
trigger OAuth recovery. Quota polls never wait for another request's refresh.

### Migration from Single-Account Configurations
Antigravity Proxy automatically migrates existing single-account configurations on load. If your configuration file contains flat `refreshToken`, `apiKey`, and `accountId` fields without an `accounts` array:
- It is converted in-memory into a single account named `default`.
- Your existing `apiKey` becomes its first key (`key_legacy`).
- The next administrative update (e.g. adding a new account, rotating keys) rewrites the file in the new multi-account format and strips the legacy flat fields.

---

## Environment Variables

Every setting can be specified or overridden using environment variables:

| Environment Variable | Description | Default |
|---|---|---|
| `HOST` | Network interface to bind to (`127.0.0.1`, `0.0.0.0`, etc.) | `127.0.0.1` (Binary) / `0.0.0.0` (Docker) |
| `PORT` | Listening port for the HTTP server | `8080` |
| `ADMIN_API_KEY` | Secret key for `/admin/*` management routes and CLI | Unset |
| `API_KEY` | Legacy/default account client authentication key | Unset |
| `ANTIGRAVITY_ACCESS_TOKEN` | Direct short-lived Google access token (bypasses OAuth refresh) | Unset |
| `ANTIGRAVITY_REFRESH_TOKEN` | Google OAuth refresh token | Saved in config |
| `ANTIGRAVITY_OAUTH_CLIENT_ID` | OAuth Client ID paired with the refresh token | Auto-discovered / Saved |
| `ANTIGRAVITY_OAUTH_CLIENT_SECRET` | OAuth Client Secret paired with the refresh token | Auto-discovered / Saved |
| `ANTIGRAVITY_PROJECT_ID` | Cloud Code project ID (auto-discovered if empty) | Auto-discovered |
| `ANTIGRAVITY_CLIENT_VERSION` | Standard client version and `X-Client-Version` header | `2.32.0` |
| `ANTIGRAVITY_CLI_CLIENT_VERSION` | CLI-family User-Agent version | `1.3.4` |
| `ANTIGRAVITY_DISCOVERY_CLIENT_FAMILY` | Model discovery identity: `standard` or `cli` | `cli` |
| `ANTIGRAVITY_GENERATION_CLIENT_RULES` | JSON array of ordered model-prefix rules | `[{"modelPrefix":"gemini-4-","family":"cli"}]` |
| `ANTIGRAVITY_MAX_CONCURRENT_GENERATIONS` | Maximum parallel in-flight generation requests | `2` |
| `ANTIGRAVITY_QUOTA_POLL_INTERVAL_SECONDS` | Interval between background quota polls (in seconds) | `300` |
| `ANTIGRAVITY_QUOTA_HISTORY_MAX_SAMPLES` | Maximum quota observation entries retained | `10000` |
| `ANTIGRAVITY_QUOTA_HISTORY_PATH` | Path to persistent quota history JSONL file | `~/.local/share/antigravity-proxy/usage.jsonl` |
| `ANTIGRAVITY_DAILY_ENDPOINT` | Daily Cloud Code endpoint override | `https://daily-cloudcode-pa.googleapis.com` |
| `ANTIGRAVITY_PROD_ENDPOINT` | Production Cloud Code endpoint override | `https://cloudcode-pa.googleapis.com` |
| `OAUTH_CALLBACK_PORT` | Local port for browser OAuth callback | `51121` (falls back to 51122–51126) |

---

## Troubleshooting Client Version Rejections

Google can reject an outdated advertised client version even when OAuth login and
model discovery succeed. Generation may return an HTTP 200 response containing
"This version of Antigravity is no longer supported" instead of an answer.
Streaming can then end with a truncated-response error because the rejection
does not contain the normal completion metadata.

This fork defaults to `2.32.0`. Ordinary and streaming Gemini generation were
verified with that setting on November 14, 2026. The upstream default of `1.15.8`
was rejected during streaming in that verification.

For an existing installation, set `clientVersion` in
`~/.config/antigravity-proxy/config.json` to a supported version and restart the
proxy. A saved value overrides the compiled default, so rebuilding alone does not
update an existing configuration. To override the saved value for one run:

```sh
ANTIGRAVITY_CLIENT_VERSION=2.32.0 ./antigravity-proxy serve
```

This setting changes the version advertised in upstream request headers; it does
not install or update the Antigravity application. If Google rejects this version
in the future, check the [official Antigravity changelog](https://antigravity.google/docs/changelog)
and verify generation with the supported version before updating the setting.

## Security & Access Control

1. **Non-Loopback Listener Safeguard**:
   If `HOST` is configured to `0.0.0.0` or any non-loopback IP, the proxy will refuse to start unless an `API_KEY` is provided. This prevents accidental public exposure of your Antigravity account.

2. **Owner-Only Permissions**:
   The proxy strictly verifies permissions on `config.json` (`0600`) to ensure local users cannot access stored Google credentials.

3. **Isolated Client Authentication**:
   The `API_KEY` you define only authenticates clients to this proxy (e.g., Bifrost, curl, or your coding agent). It is never forwarded to Google. Google requests use the OAuth bearer token obtained via Google OAuth.

4. **Structured Logging**:
   All logs use Go's standard `log/slog` in structured JSON format. Credentials, prompts, responses, and authorization tokens are never logged.

   Failed streaming requests add a `stream_failure` category to the access log.
   For example, `upstream_timeout` means the upstream read timed out,
   `upstream_incomplete` means the stream lacked completion metadata, and
   `downstream_write_failed` means delivery to the client failed. Categories are
   fixed labels; raw errors and response content are not recorded. A committed
   stream can still have HTTP status 200 with `aborted: true`: it is terminated
   rather than having a synthetic error or completion inserted into model output.

## Client Family Rules

Defaults preserve the previously verified behavior: discovery uses the CLI family,
`gemini-4-*` generation uses CLI, and unmatched models use standard identity.
`cliClientVersion` controls only the CLI User-Agent; `clientVersion` still controls
`X-Client-Version` for both families. These settings do not change OAuth clients,
model IDs, project IDs, or native generation payloads.

Rules match case-sensitive prefixes in order; the first match wins. An empty
prefix matches all models. An explicit empty array disables generation CLI rules;
an omitted or null list uses the default Gemini 4 rule. For example:

```json
{
  "cliClientVersion": "1.3.4",
  "discoveryClientFamily": "cli",
  "generationClientRules": [
    {"modelPrefix": "gemini-4-special", "family": "standard"},
    {"modelPrefix": "gemini-4-", "family": "cli"}
  ]
}
```

The three new environment variables override saved values. For one run:

```sh
ANTIGRAVITY_CLI_CLIENT_VERSION=1.3.4 \
ANTIGRAVITY_GENERATION_CLIENT_RULES='[{"modelPrefix":"gemini-4-","family":"cli"}]' \
./antigravity-proxy serve
```

Restart after editing these settings. The admin account model catalog includes
`client_identity` with discovery family, both versions, and generation families
for the current page. The public Gemini catalog remains unchanged. A model being
listed does not prove generation works; use the [compatibility checks](compatibility.md).

Version-rejection diagnostics describe non-success HTTP errors only. A provider
message inside a successful native response remains untouched; stream failures
continue to be reported by transport interruption rather than injected content.
