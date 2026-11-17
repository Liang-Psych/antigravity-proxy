# Native transport fixtures

These fixtures are synthetic, contain no account data, and are not captured
provider responses. The media bytes are placeholders: these tests validate
transport preservation, not the provider's ability to decode media.

`tool-turn.json` exercises a signature-bearing tool call carried into the next
request. `media-request.json` covers inline media, video metadata, structured
output adaptation and future fields. Real provider behavior is checked separately
by the opt-in live test described in `docs/compatibility.md`.
