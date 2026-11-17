"""Offline release-update contract tests; never mutate the real checkout."""

import contextlib
import importlib.util
import io
from pathlib import Path
import shutil
import tempfile
import unittest
from unittest.mock import patch

spec = importlib.util.spec_from_file_location("update_cli_version", Path(__file__).with_name("update-cli-version.py"))
updater = importlib.util.module_from_spec(spec)
spec.loader.exec_module(updater)


def release(tag):
    return {"tag_name": tag, "draft": False, "prerelease": False}


class UpdateCLIVersionTests(unittest.TestCase):
    def setUp(self):
        temp = tempfile.TemporaryDirectory()
        self.addCleanup(temp.cleanup)
        self.root = Path(temp.name)
        self.paths = ["internal/config/client_identity.go", "internal/proxy/client_family_test.go", "docs/configuration.md"]
        for name in self.paths:
            target = self.root / name
            target.parent.mkdir(parents=True, exist_ok=True)
            shutil.copyfile(updater.ROOT / name, target)
        self.current = updater.DEFAULT.search((self.root / self.paths[0]).read_text()).group(2)
        major, minor, patch = map(int, self.current.split("."))
        self.new = f"{major}.{minor}.{patch + 1}"

    def invoke(self, args=(), candidate=None):
        with patch.object(updater, "fetch_release", return_value=candidate or release(self.new)), contextlib.redirect_stdout(io.StringIO()), contextlib.redirect_stderr(io.StringIO()):
            return updater.main(args, self.root)

    def snapshot(self):
        return {name: (self.root / name).read_bytes() for name in self.paths}

    def test_updates_all_references_without_rewriting_live_verification(self):
        self.assertEqual(self.invoke(), 0)
        source = (self.root / self.paths[0]).read_text()
        test = (self.root / self.paths[1]).read_text()
        docs = (self.root / self.paths[2]).read_text()
        self.assertEqual(updater.DEFAULT.search(source).group(2), self.new)
        self.assertIn("antigravity/cli/" + self.new, test)
        self.assertIn('"cliClientVersion": "' + self.new + '"', docs)
        self.assertIn("CLI identity `1.5.10` on November 17, 2026", docs)
        self.assertIn("ANTIGRAVITY_CLI_CLIENT_VERSION=" + self.new, docs)
        before = self.snapshot()
        self.assertEqual(self.invoke(), 0)
        self.assertEqual(self.snapshot(), before)

    def test_check_mode_writes_metadata_and_body_without_source_changes(self):
        before = self.snapshot()
        output, body = self.root / "output", self.root / "pr.md"
        self.assertEqual(self.invoke(["--check", "--github-output", str(output), "--pr-body", str(body)]), 0)
        self.assertEqual(self.snapshot(), before)
        self.assertIn("updated=true\n", output.read_text())
        self.assertIn("Real Google-account generation is **not** run", body.read_text())
        self.assertIn(self.new, body.read_text())

    def test_current_and_older_releases_never_downgrade_or_write(self):
        for tag in [self.current, "0.0.1"]:
            with self.subTest(tag=tag):
                before = self.snapshot()
                self.assertEqual(self.invoke(candidate=release(tag)), 0)
                self.assertEqual(self.snapshot(), before)

    def test_numeric_order_and_v_prefixed_tag(self):
        candidate = release("v" + self.new)
        current, new, tag, changes = updater.plan_update(self.root, candidate)
        self.assertEqual((current, new, tag), (self.current, self.new, "v" + self.new))
        self.assertIn("/releases/tag/v" + self.new, changes[self.root / self.paths[2]])
        # Exercise the 9 -> 10 comparison independently of the repository default.
        for name in self.paths:
            path = self.root / name
            path.write_text(path.read_text().replace(self.current, "1.5.9"))
        self.assertTrue(updater.plan_update(self.root, release("1.5.10"))[3])

    def test_rejects_drafts_prereleases_and_unsafe_tags(self):
        for candidate in [release("1.6.0-rc.1"), release("1.6.0\nupdated=true"), release("$(touch /tmp/unwanted)"), release("01.6.0"), {**release(self.new), "draft": True}, {**release(self.new), "prerelease": True}, {"tag_name": self.new}, [], None]:
            with self.subTest(candidate=candidate):
                with self.assertRaises(ValueError):
                    updater.release_version(candidate)

    def test_documentation_drift_fails_before_any_file_is_written(self):
        path = self.root / self.paths[2]
        path.write_text(path.read_text().replace('"cliClientVersion"', '"renamedSetting"'))
        before = self.snapshot()
        self.assertEqual(self.invoke(), 1)
        self.assertEqual(self.snapshot(), before)

    def test_api_failure_is_not_treated_as_no_update(self):
        with patch.object(updater, "fetch_release", side_effect=ValueError("rate limited")), contextlib.redirect_stderr(io.StringIO()):
            self.assertEqual(updater.main([], self.root), 1)

    def test_workflow_checks_before_publish_and_never_merges(self):
        text = (updater.ROOT / ".github/workflows/update-cli-version.yml").read_text()
        self.assertIn("cron: '17 */6 * * *'", text)
        self.assertIn("workflow_dispatch:", text)
        self.assertLess(text.index("./scripts/ci/test.sh"), text.index("git push origin"))
        self.assertLess(text.index("./scripts/ci/check-quality.sh"), text.index("gh pr create"))
        self.assertIn('--state all', text)
        self.assertNotIn("--force", text)
        self.assertNotIn("gh pr merge", text)


if __name__ == "__main__":
    unittest.main()
