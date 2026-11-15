import contextlib
import importlib.util
import io
import json
from pathlib import Path
import tempfile
import unittest
from unittest.mock import patch
import urllib.error

spec = importlib.util.spec_from_file_location("agy_models", Path(__file__).with_name("agy-models.py"))
models = importlib.util.module_from_spec(spec)
spec.loader.exec_module(models)


class ModelsTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.folder = Path(self.temp.name)
        (self.folder / "client.json").write_text(json.dumps({"base_url": "http://127.0.0.1:8878/v1beta", "api_key": "fixture-data-key"}))
        (self.folder / "admin-key").write_text("fixture-admin-key")

    def invoke(self, args, query):
        stdout, stderr = io.StringIO(), io.StringIO()
        with patch.object(models, "fetch_json", side_effect=query), contextlib.redirect_stdout(stdout), contextlib.redirect_stderr(stderr):
            status = models.main(args, self.folder)
        return status, stdout.getvalue(), stderr.getvalue()

    def test_union_preserves_account_membership_and_uses_admin_key(self):
        def query(url, headers):
            self.assertEqual(headers, {"Authorization": "Bearer fixture-admin-key"})
            if url.endswith("/admin/accounts"):
                return [{"id": name, "enabled": True, "credential_configured": True} for name in ["first", "second"]]
            names = ["shared", "second-only"] if "/second/" in url else ["shared"]
            return {"models": [{"name": "models/" + name} for name in names]}
        status, output, error = self.invoke([], query)
        self.assertEqual(status, 0)
        self.assertIn("2/2", output)
        self.assertIn("shared  [first, second]", output)
        self.assertIn("second-only  [second]", output)
        self.assertNotIn("fixture-", output + error)

    def test_partial_failure_is_reported_and_not_silently_complete(self):
        def query(url, headers):
            if url.endswith("/accounts"):
                return [{"id": name, "enabled": True, "credential_configured": True} for name in ["first", "second"]]
            if "/second/" in url:
                raise urllib.error.HTTPError(url, 503, "private error", {}, None)
            return {"models": [{"name": "models/shared"}]}
        status, output, error = self.invoke([], query)
        self.assertEqual(status, 1)
        self.assertIn("1/2", output)
        self.assertIn("second", error)
        self.assertNotIn("private error", error)

    def test_current_uses_data_key_without_reading_admin_key(self):
        (self.folder / "admin-key").unlink()
        def query(url, headers):
            self.assertEqual(url, "http://127.0.0.1:8878/v1beta/models")
            self.assertEqual(headers, {"x-goog-api-key": "fixture-data-key"})
            return {"models": [{"name": "models/current"}]}
        self.assertEqual(self.invoke(["--current"], query)[0], 0)

    def test_account_filter_skips_other_and_disabled_accounts(self):
        def query(url, headers):
            if url.endswith("/accounts"):
                return [{"id": "first", "enabled": False, "credential_configured": True}, {"id": "second", "enabled": True, "credential_configured": True}]
            self.assertIn("/second/models", url)
            return {"models": []}
        self.assertEqual(self.invoke(["--account", "second"], query)[0], 0)
        self.assertEqual(self.invoke(["--account", "missing"], query)[0], 1)

    def test_admin_key_never_sent_to_remote_origin(self):
        (self.folder / "client.json").write_text(json.dumps({"base_url": "https://remote.example/v1beta"}))
        def query(*args):
            self.fail("remote admin query must be refused")
        self.assertEqual(self.invoke([], query)[0], 1)

    def test_catalog_pagination_and_cycle_detection(self):
        pages = [{"models": [{"name": "models/a"}], "nextPageToken": "next"}, {"models": [{"name": "models/b"}]}]
        with patch.object(models, "fetch_json", side_effect=pages):
            self.assertEqual(models.read_catalog("http://localhost/models", {}), {"a", "b"})
        with patch.object(models, "fetch_json", return_value={"models": [], "nextPageToken": "loop"}):
            with self.assertRaises(ValueError):
                models.read_catalog("http://localhost/models", {})


if __name__ == "__main__":
    unittest.main()
