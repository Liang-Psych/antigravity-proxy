# Compatibility Checks

The proxy preserves native Gemini payloads. Offline tests prove transport
behavior; live tests check whether the configured account and upstream still work.
Neither a catalog entry nor a mock response proves a model is callable.

## Offline Regression Suite

```sh
go test -race ./...
```

Existing tests cover opaque signatures, large numeric literals, unknown fields,
SSE fragmentation, completion detection, cancellation, OAuth recovery, endpoint
fallback and request bounds. Additional replay tests carry a signature-bearing
model tool call into the next native request, and exercise inline video/image
parts, video metadata and structured output schema adaptation.

Fixtures in `internal/proxy/testdata/native/` are synthetic and contain no account
data. Their media bytes are placeholders. They test payload preservation, not
actual media decoding or provider support. Native SDK and gateway versions are
not yet exercised by this suite.

## Live Account Smoke Test

This test consumes quota: it makes a model catalog query, an exact model lookup,
one short generation and one short streaming generation. It is skipped unless
`AGY_LIVE_MODEL` is set. With multiple usable accounts, specify `AGY_LIVE_ACCOUNT`.
Use the exact upstream wire ID without the `models/` prefix:

```sh
AGY_LIVE_ACCOUNT=work \
AGY_LIVE_MODEL=gemini-3.8-flash-tiered \
go test ./internal/proxy -run '^TestLiveNativeCompatibility$' -v -count=1
```

The test loads the existing private proxy config and uses the newly compiled
proxy implementation directly. It does not replace the running service, write
credentials, start quota polling, or change history. It reports catalog visibility,
non-streaming generation, and streaming completion separately; provider content
is not printed. The selected model must support text and native SSE generation.
Real tool signatures and actual image/video decoding still need separate live
validation; offline fixtures do not establish those capabilities.

Use client-family settings from the [configuration guide](configuration.md#client-family-rules)
when a published model requires a different transport identity. Rules describe
configured behavior; verify actual generation after changing them.
