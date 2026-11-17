# Automatic CLI Version Updates

The `Update CLI Client Version` GitHub Actions workflow checks the official
`google-antigravity/antigravity-cli` latest stable release every six hours, at
17 minutes past the hour (UTC). You can also run it manually from the Actions tab.
It does not query releases during normal proxy startup or generation requests.

## Update Process

1. Read the official GitHub latest-release API. Drafts, prereleases, unexpected
   tag formats and API failures are rejected. Only a numerically newer stable
   version is eligible; older versions never downgrade the default.
2. Update `DefaultCLIClientVersion`, the transport-identity test and configuration
   examples in one candidate change. Pattern drift fails before source writes.
   Historical live-account verification notes keep their original version/date.
3. Run offline race tests with coverage, strict Go lint, Python tests, script
   syntax checks, documentation/workflow checks, the secret scan and a build.
4. After those checks succeed, push `automation/cli-client-version-VERSION` and
   open a `fix:` PR. One proposal is made per version. An existing open, merged
   or closed PR is left alone, so rejecting a version does not cause daily spam.

A branch left behind by a failed PR-creation attempt can be reused only when its
entire tree matches the newly checked candidate. Different branch contents are
left untouched and reported as a workflow warning. No branch is force-pushed.
Newer versions can produce separate PRs; older open proposals are not auto-closed.

The workflow does not merge, deploy, alter saved proxy configuration, upgrade the
installed official CLI, or send prompts to Google. Before merging an update,
run the [live account smoke test](compatibility.md#live-account-smoke-test) with
`ANTIGRAVITY_CLI_CLIENT_VERSION` set to the proposed version. Offline success does
not establish that a newly advertised identity is accepted by every account/model.

## GitHub Permissions

The workflow grants its own job `contents: write` and `pull-requests: write` while
keeping the repository's default workflow token permissions read-only.
Enable **Settings > Actions > General > Workflow permissions > Allow GitHub
Actions to create and approve pull requests** to permit PR creation. The workflow
itself never approves or merges a PR.

It uses `GITHUB_TOKEN` by default. GitHub may require a maintainer to approve the
subsequent PR workflows, so candidate checks run inside the scheduled workflow
before publishing. If the existing `RELEASE_PLEASE_TOKEN` secret is configured,
it is used instead; a suitable PAT or App token can allow ordinary PR CI to run
without that approval. See [GitHub's workflow-trigger documentation](https://docs.github.com/en/actions/how-tos/write-workflows/choose-when-workflows-run/trigger-a-workflow).

GitHub can disable scheduled workflows in inactive repositories or forks. A
manual run verifies the installed workflow; keep its schedule enabled in Actions.

## Local Check

With authenticated `gh` available:

```sh
python3 scripts/update-cli-version.py --check
```

Without `--check`, the script updates local source, tests and examples when a
newer stable release exists. It never commits, pushes, or opens a PR itself.
Environment variables and saved `cliClientVersion` values still override the
compiled default, so rebuilding does not replace an explicitly configured value.
