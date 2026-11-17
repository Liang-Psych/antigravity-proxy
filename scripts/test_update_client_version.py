"""Offline App client-version update contract tests; never mutate the real checkout."""

import contextlib
import importlib.util
import io
from pathlib import Path
import shutil
import tempfile
import unittest
from unittest.mock import patch

SAMPLE = object()

spec = importlib.util.spec_from_file_location("update_client_version", Path(__file__).with_name("update-client-version.py"))
updater = importlib.util.module_from_spec(spec)
spec.loader.exec_module(updater)


def entry(product, version):
    return f'<article class="rn-row release-entry astro-x" id="rel-{product}-{version}"><h3>v{version}</h3></article>'


def changelog(*app_versions, decoys=True):
    """Mimic the official page: App, IDE and CLI tabs share one document."""
    parts = ['<a href="#rel-hub-99.0.0">table of contents link, not a release</a>']
    if decoys:
        parts += [entry("ide", "98.0.0"), entry("cli", "97.0.0"), '<div id="rel-hub-96.0.0"></div>']
    parts += [entry("hub", version) for version in app_versions]
    return "\n".join(parts)


class UpdateClientVersionTests(unittest.TestCase):
    def setUp(self):
        temp = tempfile.TemporaryDirectory()
        self.addCleanup(temp.cleanup)
        self.root = Path(temp.name)
        self.paths = ["internal/config/config.go", "docs/configuration.md", "README.md"]
        for name in self.paths:
            target = self.root / name
            target.parent.mkdir(parents=True, exist_ok=True)
            shutil.copyfile(updater.ROOT / name, target)
        self.current = updater.DEFAULT.search((self.root / self.paths[0]).read_text()).group(2)
        major, minor, patch = map(int, self.current.split("."))
        self.new = f"{major}.{minor}.{patch + 1}"
        self.page = changelog(self.new, self.current, f"{major}.{minor - 1}.0")

    def invoke(self, args=(), page=SAMPLE):
        with patch.object(updater, "fetch_changelog", return_value=self.page if page is SAMPLE else page), contextlib.redirect_stdout(io.StringIO()), contextlib.redirect_stderr(io.StringIO()):
            return updater.main(args, self.root)

    def snapshot(self):
        return {name: (self.root / name).read_bytes() for name in self.paths}

    def test_updates_all_references_without_rewriting_live_verification(self):
        self.assertEqual(self.invoke(), 0)
        source, docs, readme = ((self.root / name).read_text() for name in self.paths)
        self.assertEqual(updater.DEFAULT.search(source).group(2), self.new)
        self.assertIn('"clientVersion": "' + self.new + '"', docs)
        self.assertIn("This fork defaults to `" + self.new + "`", docs)
        self.assertIn("ANTIGRAVITY_CLIENT_VERSION=" + self.new + " ./antigravity-proxy serve", docs)
        self.assertIn("verified with App identity `2.32.0` on November", docs)
        self.assertIn("client version to `" + self.new + "`", readme)
        self.assertIn("`ANTIGRAVITY_CLIENT_VERSION=" + self.new + "`", readme)
        before = self.snapshot()
        self.assertEqual(self.invoke(), 0)
        self.assertEqual(self.snapshot(), before)

    def test_selects_only_app_release_entries(self):
        self.assertEqual(updater.latest_app_version(self.page), self.new)

    def test_numeric_order_not_page_order(self):
        self.assertEqual(updater.latest_app_version(changelog("2.32.9", "2.32.10", "2.31.0")), "2.32.10")

    def test_check_mode_writes_metadata_and_body_without_source_changes(self):
        before = self.snapshot()
        output, body = self.root / "output", self.root / "pr.md"
        self.assertEqual(self.invoke(["--check", "--github-output", str(output), "--pr-body", str(body)]), 0)
        self.assertEqual(self.snapshot(), before)
        self.assertIn("updated=true\n", output.read_text())
        self.assertIn("Real Google-account generation is **not** run", body.read_text())
        self.assertIn("#rel-hub-" + self.new, body.read_text())

    def test_current_and_older_releases_never_downgrade_or_write(self):
        for page in [changelog(self.current, "1.0.0", "0.9.0"), changelog("1.0.0", "0.9.0", "0.8.0")]:
            with self.subTest(page=page):
                before = self.snapshot()
                self.assertEqual(self.invoke(page=page), 0)
                self.assertEqual(self.snapshot(), before)

    def test_redesigned_or_unsafe_page_fails_instead_of_reporting_no_update(self):
        # Too few App entries (only decoys, or a partial page), non-text, and
        # non-stable or non-canonical versions must all fail loudly.
        for page in [changelog(), changelog(self.new, self.current), "", None, entry("hub", "2.33.0-rc.1") * 3, entry("hub", "02.33.0") * 3]:
            with self.subTest(page=page):
                before = self.snapshot()
                self.assertEqual(self.invoke(page=page), 1)
                self.assertEqual(self.snapshot(), before)

    def test_documentation_drift_fails_before_any_file_is_written(self):
        path = self.root / "README.md"
        path.write_text(path.read_text().replace("updates the default advertised client version", "pins the client version"))
        before = self.snapshot()
        self.assertEqual(self.invoke(), 1)
        self.assertEqual(self.snapshot(), before)

    def test_fetch_failure_is_not_treated_as_no_update(self):
        with patch.object(updater, "fetch_changelog", side_effect=ValueError("offline")), contextlib.redirect_stderr(io.StringIO()):
            self.assertEqual(updater.main([], self.root), 1)

    def test_workflow_checks_before_publish_and_never_merges(self):
        text = (updater.ROOT / ".github/workflows/update-client-version.yml").read_text()
        self.assertIn("workflow_dispatch:", text)
        self.assertIn("scripts/update-client-version.py", text)
        self.assertLess(text.index("./scripts/ci/test.sh"), text.index("git push origin"))
        self.assertLess(text.index("./scripts/ci/check-quality.sh"), text.index("gh pr create"))
        self.assertIn("--state all", text)
        self.assertNotIn("--force", text)
        self.assertNotIn("gh pr merge", text)


if __name__ == "__main__":
    unittest.main()
