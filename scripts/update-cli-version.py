#!/usr/bin/env python3
"""Update the proxy's advertised CLI identity from the official stable release."""

import argparse
import json
from pathlib import Path
import re
import subprocess
import sys

ROOT = Path(__file__).resolve().parent.parent
UPSTREAM = "google-antigravity/antigravity-cli"
VERSION = r"(?:0|[1-9][0-9]*)\.(?:0|[1-9][0-9]*)\.(?:0|[1-9][0-9]*)"
DEFAULT = re.compile(r'(DefaultCLIClientVersion\s*=\s*")(' + VERSION + r')(")')


def release_version(release):
    """Return only a stable numeric version; never evaluate upstream text."""
    if not isinstance(release, dict) or release.get("draft") is not False or release.get("prerelease") is not False:
        raise ValueError("Expected an official non-draft, non-prerelease release")
    tag = release.get("tag_name")
    if not isinstance(tag, str) or len(tag) > 65 or not re.fullmatch("v?" + VERSION, tag):
        raise ValueError("Official release tag is not a stable numeric version")
    return tag.removeprefix("v"), tag


def fetch_release():
    result = subprocess.run(
        ["gh", "api", "repos/" + UPSTREAM + "/releases/latest"],
        capture_output=True, text=True, timeout=60, check=False,
    )
    if result.returncode:
        # Do not print command stderr, which can contain environment details.
        raise ValueError("Cannot query the official release; check GitHub API access and rate limits")
    return json.loads(result.stdout)


def substitute(text, pattern, current, new, count=1):
    """Fail before writing anything if source/docs no longer match the contract."""
    regex = re.compile(pattern)
    matches = list(regex.finditer(text))
    if len(matches) != count or any(match.group(2) != current for match in matches):
        raise ValueError("CLI version references have drifted; update the automation patterns before retrying")
    return regex.sub(lambda match: match.group(1) + new + match.group(3), text)


def plan_update(root, release):
    new, tag = release_version(release)
    source_path = root / "internal/config/client_identity.go"
    source = source_path.read_text()
    matches = list(DEFAULT.finditer(source))
    if len(matches) != 1:
        raise ValueError("Cannot find exactly one CLI default")
    current = matches[0].group(2)
    # Numeric comparison prevents 1.5.9 > 1.5.10 errors and accidental downgrades.
    if tuple(map(int, new.split("."))) <= tuple(map(int, current.split("."))):
        return current, new, tag, {}
    paths = [source_path, root / "internal/proxy/client_family_test.go", root / "docs/configuration.md"]
    contents = {path: path.read_text() for path in paths}
    contents[source_path] = substitute(source, DEFAULT.pattern, current, new)
    test = paths[1]
    contents[test] = substitute(contents[test], r'(return fmt\.Sprintf\("antigravity/cli/)(' + VERSION + r')( \(os_type=)', current, new)
    docs = paths[2]
    text = contents[docs]
    for pattern, count in [
        (r'("cliClientVersion": ")(' + VERSION + r')(")', 2),
        (r'(\| `ANTIGRAVITY_CLI_CLIENT_VERSION` \|[^\n]*\| `)(' + VERSION + r')(` \|)', 1),
        (r'(The default CLI User-Agent version is `)(' + VERSION + r')(`)', 1),
        (r'(ANTIGRAVITY_CLI_CLIENT_VERSION=)(' + VERSION + r')( \\\n)', 1),
    ]:
        text = substitute(text, pattern, current, new, count)
    # Tags may have a v prefix even though advertised versions never do.
    link = re.compile(r'(\[official CLI release\]\(https://github.com/' + re.escape(UPSTREAM) + r'/releases/tag/)(v?' + VERSION + r')(\))')
    found = list(link.finditer(text))
    if len(found) != 1 or found[0].group(2).removeprefix("v") != current:
        raise ValueError("Official CLI release link has drifted")
    text = link.sub(lambda match: match.group(1) + tag + match.group(3), text)
    contents[docs] = text
    return current, new, tag, contents


def pr_body(current, new, tag):
    return f"""Update the advertised CLI User-Agent version from `{current}` to `{new}` to follow the [official stable release](https://github.com/{UPSTREAM}/releases/tag/{tag}).

Updates the compiled default, transport identity test, and configuration examples together. Model routing and native request/response payloads are unchanged.

Validation: the update workflow runs race tests with coverage, strict lint, script tests, documentation checks, workflow checks, and a build before opening this PR. Real Google-account generation is **not** run by automation; verify it before merging. Historical live-verification notes are preserved.

Environment variables and a saved `cliClientVersion` still override the compiled default. This PR does not upgrade the installed official CLI, modify running proxy configuration, deploy, or merge itself.
"""


def main(argv=None, root=ROOT):
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--check", action="store_true", help="Report an available update without changing source files")
    parser.add_argument("--github-output", type=Path, help="Write validated metadata to this GitHub Actions output file")
    parser.add_argument("--pr-body", type=Path, help="Write a PR description for the candidate update")
    args = parser.parse_args(argv)
    try:
        current, new, tag, changes = plan_update(root, fetch_release())
        if changes and not args.check:
            for path, text in changes.items():
                path.write_text(text)
        if args.github_output:
            with args.github_output.open("a") as output:
                output.write(f"updated={str(bool(changes)).lower()}\nprevious={current}\nversion={new}\ntag={tag}\n")
        if args.pr_body and changes:
            args.pr_body.write_text(pr_body(current, new, tag))
        if changes:
            print(f"{'Available' if args.check else 'Updated'} CLI identity: {current} -> {new}")
        else:
            print(f"No newer stable CLI release: current={current}, upstream={new}")
        return 0
    except (OSError, ValueError, subprocess.SubprocessError):
        print("CLI version update failed: check official release access and source/documentation consistency; no PR should be published.", file=sys.stderr)
        return 1


if __name__ == "__main__":
    sys.exit(main())
