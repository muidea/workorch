import pathlib
import shutil
import subprocess
import tempfile
import unittest


class GoFormatSourceBoundaryTest(unittest.TestCase):
    def setUp(self):
        self.directory = tempfile.TemporaryDirectory(prefix="workorch-format-boundary-")
        self.addCleanup(self.directory.cleanup)
        self.root = pathlib.Path(self.directory.name)
        self.makefile = pathlib.Path(__file__).resolve().parents[1] / "Makefile"
        self.gofmt = shutil.which("gofmt")
        if not self.gofmt:
            self.skipTest("gofmt required")

    def write_fixture(self, name, content):
        path = self.root / name
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_bytes(content)
        return path

    def invoke(self, target):
        # No dependency fetch or compiler is needed for format boundary tests.
        return subprocess.run(
            ["make", "-s", "-f", str(self.makefile), target, "GO=true", f"GOFMT={self.gofmt}"],
            cwd=self.root, stdout=subprocess.PIPE, stderr=subprocess.PIPE, check=False,
        )

    def test_check_preserves_complete_raw_runtime_material(self):
        self.write_fixture("internal/owner/owner.go", b"package owner\n\nconst Value = 1\n")
        raw = b"original diagnostic bytes\x00\xff\r\nnot Go source"
        artifact = self.write_fixture(".validation-tmp-fixture/workspace/input_test.go", raw)
        result = self.invoke("fmt-check")
        self.assertEqual(result.returncode, 0, result.stderr.decode())
        self.assertEqual(artifact.read_bytes(), raw)

    def test_check_still_rejects_unformatted_product_source(self):
        self.write_fixture("internal/owner/owner.go", b"package owner\nconst Value=1\n")
        result = self.invoke("fmt-check")
        self.assertNotEqual(result.returncode, 0)
        self.assertIn(b"internal/owner/owner.go", result.stdout)

    def test_similar_named_source_file_is_not_excluded(self):
        self.write_fixture(".validation-tmp-source.go", b"package fixture\nconst Value=1\n")
        result = self.invoke("fmt-check")
        self.assertNotEqual(result.returncode, 0)
        self.assertIn(b".validation-tmp-source.go", result.stdout)

    def test_write_formats_source_without_changing_raw_runtime_material(self):
        source = self.write_fixture("internal/owner/owner.go", b"package owner\nconst Value=1\n")
        raw = b"original diagnostic bytes\x00\xff\r\nnot Go source"
        artifact = self.write_fixture(".validation-tmp-fixture/workspace/input_test.go", raw)
        result = self.invoke("fmt")
        self.assertEqual(result.returncode, 0, result.stderr.decode())
        self.assertEqual(source.read_bytes(), b"package owner\n\nconst Value = 1\n")
        self.assertEqual(artifact.read_bytes(), raw)


if __name__ == "__main__":
    unittest.main()
