#!/usr/bin/env python3
"""Update the proxy's advertised Antigravity App identity from the official changelog."""

import argparse
from pathlib import Path
import re
import sys
import urllib.request

ROOT = Path(__file__).resolve().parent.parent
CHANGELOG = "https://antigravity.google/docs/changelog"
VERSION = r"(?:0|[1-9][0-9]*)\.(?:0|[1-9][0-9]*)\.(?:0|[1-9][0-9]*)"
DEFAULT = re.compile(r'(const DefaultClientVersion\s*=\s*")(' + VERSION + r')(")')
# The changelog has one tab per product. Only Antigravity App ("hub") entries
# identify the desktop client advertised in X-Client-Version; IDE, CLI and SDK
# entries use unrelated version lines.
APP_RELEASE = re.compile(r'<article\b[^>]*\bclass="[^"]*\brelease-entry\b[^"]*"[^>]*\bid="rel-hub-(' + VERSION + r')"')
MAX_PAGE_BYTES = 8 << 20
# A redesign that drops most entries must fail rather than select a stale version.
MIN_APP_RELEASES = 3


def numeric(version):
    return tuple(map(int, version.split(".")))


def latest_app_version(page):
    """Return the newest App version from the changelog without evaluating page text."""
    if not isinstance(page, str):
        raise ValueError("Changelog page is not text")
    versions = APP_RELEASE.findall(page)
    if len(versions) < MIN_APP_RELEASES:
        raise ValueError("Changelog no longer lists Antigravity App releases in the expected format")
    return max(versions, key=numeric)


def fetch_changelog():
    request = urllib.request.Request(CHANGELOG, headers={"User-Agent": "antigravity-proxy-version-check"})
    try:
        with urllib.request.urlopen(request, timeout=60) as response:
            if response.status != 200:
                raise ValueError("Changelog request failed")
            body = response.read(MAX_PAGE_BYTES + 1)
    except OSError as error:
        raise ValueError("Cannot fetch the official changelog") from error
    if len(body) > MAX_PAGE_BYTES:
        raise ValueError("Changelog page exceeds size limit")
    return body.decode("utf-8")


def substitute(text, pattern, current, new, count=1):
    """Fail before writing anything if source/docs no longer match the contract."""
    regex = re.compile(pattern)
    matches = list(regex.finditer(text))
    if len(matches) != count or any(match.group(2) != current for match in matches):
        raise ValueError("Client version references have drifted; update the automation patterns before retrying")
    return regex.sub(lambda match: match.group(1) + new + match.group(3), text)


def plan_update(root, page):
    new = latest_app_version(page)
    source_path = root / "internal/config/config.go"
    source = source_path.read_text()
    matches = list(DEFAULT.finditer(source))
    if len(matches) != 1:
        raise ValueError("Cannot find exactly one client version default")
    current = matches[0].group(2)
    # Numeric comparison prevents 2.32.9 > 2.32.10 errors and accidental downgrades.
    if numeric(new) <= numeric(current):
        return current, new, {}
    docs, readme = root / "docs/configuration.md", root / "README.md"
    contents = {path: path.read_text() for path in [source_path, docs, readme]}
    contents[source_path] = substitute(source, DEFAULT.pattern, current, new)
    text = contents[docs]
    for pattern in [
        r'("clientVersion": ")(' + VERSION + r')(")',
        r'(\| `ANTIGRAVITY_CLIENT_VERSION` \|[^\n]*\| `)(' + VERSION + r')(` \|)',
        r'(This fork defaults to `)(' + VERSION + r')(`)',
        r'(ANTIGRAVITY_CLIENT_VERSION=)(' + VERSION + r')( \./antigravity-proxy serve)',
    ]:
        text = substitute(text, pattern, current, new)
    contents[docs] = text
    text = contents[readme]
    for pattern in [
        r'(updates the default advertised client version to `)(' + VERSION + r')(`)',
        r'(`ANTIGRAVITY_CLIENT_VERSION=)(' + VERSION + r')(` to use)',
    ]:
        text = substitute(text, pattern, current, new)
    contents[readme] = text
    return current, new, contents


def pr_body(current, new):
    return f"""Update the advertised Antigravity App client version from `{current}` to `{new}` to follow the latest App release in the [official Antigravity changelog]({CHANGELOG}#rel-hub-{new}).

Updates the compiled `DefaultClientVersion` and documentation examples together. Model routing and native request/response payloads are unchanged.

Validation: the update workflow runs race tests with coverage, strict lint, script tests, documentation checks, workflow checks, and a build before opening this PR. Real Google-account generation is **not** run by automation; verify it before merging. Historical live-verification notes are preserved.

Environment variables and a saved `clientVersion` still override the compiled default. This PR does not modify running proxy configuration, deploy, or merge itself.
"""


def main(argv=None, root=ROOT):
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--check", action="store_true", help="Report an available update without changing source files")
    parser.add_argument("--github-output", type=Path, help="Write validated metadata to this GitHub Actions output file")
    parser.add_argument("--pr-body", type=Path, help="Write a PR description for the candidate update")
    args = parser.parse_args(argv)
    try:
        current, new, changes = plan_update(root, fetch_changelog())
        if changes and not args.check:
            for path, text in changes.items():
                path.write_text(text)
        if args.github_output:
            with args.github_output.open("a") as output:
                output.write(f"updated={str(bool(changes)).lower()}\nprevious={current}\nversion={new}\n")
        if args.pr_body and changes:
            args.pr_body.write_text(pr_body(current, new))
        if changes:
            print(f"{'Available' if args.check else 'Updated'} App client identity: {current} -> {new}")
        else:
            print(f"No newer App release: current={current}, changelog={new}")
        return 0
    except (OSError, ValueError, UnicodeDecodeError):
        print("Client version update failed: check changelog access and source/documentation consistency; no PR should be published.", file=sys.stderr)
        return 1


if __name__ == "__main__":
    sys.exit(main())
