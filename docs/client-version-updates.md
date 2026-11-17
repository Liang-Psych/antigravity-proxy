# Automatic Client Version Updates

The `Update App Client Version` GitHub Actions workflow keeps the advertised
Antigravity App identity (`clientVersion`, `X-Client-Version` and the standard
User-Agent) current. It checks the
[official Antigravity changelog](https://antigravity.google/docs/changelog) every
six hours, at 43 minutes past the hour (UTC), and can also be run manually from the
Actions tab. The proxy itself never queries the changelog while serving requests.

This is separate from the agy CLI identity (`cliClientVersion`), which follows
GitHub releases; see [Automatic CLI Version Updates](cli-version-updates.md).

## Update Process

1. Fetch the changelog page (60-second timeout, 8 MiB limit) and read only the
   Antigravity App release entries (`<article class="release-entry" id="rel-hub-X.Y.Z">`).
   IDE, CLI, SDK and plugin entries use unrelated version lines and are ignored.
   Fewer than three App entries, non-numeric versions or fetch failures fail the
   run instead of being treated as "no update".
2. Select the numerically newest App version. Only a newer version is eligible;
   older versions never downgrade the default.
3. Update `DefaultClientVersion`, the configuration examples and the README in one
   candidate change. Pattern drift fails before any file is written. Historical
   live-account verification notes keep their original version and date.
4. Run offline race tests with coverage, strict Go lint, Python tests, script
   syntax checks, documentation/workflow checks, the secret scan and a build.
5. Push `automation/app-client-version-VERSION` and open a `fix:` PR. One proposal
   is made per version; an existing open, merged or closed PR is left alone.

The workflow does not merge, deploy or alter saved proxy configuration. Before
merging, run the [live account smoke test](compatibility.md#live-account-smoke-test)
with `ANTIGRAVITY_CLIENT_VERSION` set to the proposed version.

Permissions, token use and scheduled-workflow caveats match the
[CLI update workflow](cli-version-updates.md#github-permissions).

## Local Check

```sh
python3 scripts/update-client-version.py --check
```

Without `--check`, the script updates local source and documentation when a newer
App release exists. It never commits, pushes, or opens a PR itself.

A saved `clientVersion` in `config.json` or the `ANTIGRAVITY_CLIENT_VERSION`
environment variable still overrides the compiled default. Remove the saved value
to follow the default after upgrading the proxy.
