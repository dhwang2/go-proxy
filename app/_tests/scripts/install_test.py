import hashlib
import os
from pathlib import Path
import subprocess
import sys
import tempfile
import unittest


INSTALLER = Path(__file__).resolve().parents[2] / "install.sh"


class InstallerHelpTests(unittest.TestCase):
    def test_help_without_root_or_runtime(self):
        result = subprocess.run(["bash", str(INSTALLER), "--help"], capture_output=True, text=True, timeout=5)
        self.assertEqual(result.returncode, 0)
        self.assertIn("go-proxy installer", result.stdout)
        self.assertEqual(result.stderr, "")


@unittest.skipUnless(sys.platform.startswith("linux") and os.geteuid() == 0, "isolated Linux root installer fixture")
class InstallerTests(unittest.TestCase):
    def setUp(self):
        self.temporary = tempfile.TemporaryDirectory(prefix="gproxy-installer-test-")
        self.addCleanup(self.temporary.cleanup)
        self.directory = Path(self.temporary.name)
        self.bin = self.directory / "tools"
        self.bin.mkdir()
        self.staging = self.directory / "temporary"
        self.staging.mkdir()
        self.target = self.directory / "installed" / "gproxy"
        self.target.parent.mkdir()
        self.target.write_text("original executable")
        self.original = self.target.read_bytes()
        self.marker = self.directory / "initialized"
        self.env = dict(os.environ, PATH=str(self.bin) + os.pathsep + os.environ["PATH"],
                        REPO="fixture/go-proxy", VERSION="v0.2.0", INSTALL_PATH=str(self.target),
                        FIXTURE=str(self.directory), INIT_MARKER=str(self.marker), TMPDIR=str(self.staging))
        self.binary = self.directory / "binary"
        self.binary.write_text('#!/bin/sh\ncase "$1" in version) echo "go-proxy v0.2.0" ;; init) echo initialized > "$INIT_MARKER" ;; *) exit 9 ;; esac\n')
        self.checksum = self.directory / "checksum"
        self.write_checksum()
        self.tool("uname", '#!/bin/sh\ncase "$1" in -s) echo Linux ;; -m) echo x86_64 ;; *) exit 2 ;; esac\n')
        self.tool("curl", '''#!/bin/sh
output=""
url=""
while [ "$#" -gt 0 ]; do
  case "$1" in -o) shift; output="$1" ;; https://*) url="$1" ;; esac
  shift
done
case "$url" in
  https://api.github.com/repos/fixture/go-proxy/releases/latest) printf '{"tag_name":"v0.2.0"}\\n' > "$output" ;;
  https://github.com/fixture/go-proxy/releases/download/v0.2.0/gproxy-linux-amd64.sha256) cp "$FIXTURE/checksum" "$output" ;;
  https://github.com/fixture/go-proxy/releases/download/v0.2.0/gproxy-linux-amd64) cp "$FIXTURE/binary" "$output" ;;
  *) exit 22 ;;
esac
''')

    def tool(self, name, script):
        path = self.bin / name
        path.write_text(script)
        path.chmod(0o755)

    def write_checksum(self):
        self.checksum.write_text(hashlib.sha256(self.binary.read_bytes()).hexdigest() + "  gproxy-linux-amd64\n")

    def run_installer(self):
        result = subprocess.run(["bash", str(INSTALLER)], env=self.env, capture_output=True, text=True, timeout=10)
        self.assertEqual(list(self.staging.iterdir()), [])
        self.assertEqual(list(self.target.parent.glob(".gproxy-install.*")), [])
        return result

    def assert_preserved(self, expected_message):
        result = self.run_installer()
        self.assertNotEqual(result.returncode, 0)
        self.assertIn(expected_message, result.stderr)
        self.assertEqual(self.target.read_bytes(), self.original)
        self.assertFalse(self.marker.exists())

    def test_verified_install_and_init(self):
        for version in ("v0.2.0", "latest"):
            with self.subTest(version=version):
                self.env["VERSION"] = version
                result = self.run_installer()
                self.assertEqual(result.returncode, 0, result.stderr)
                self.assertEqual(self.target.read_bytes(), self.binary.read_bytes())
                self.assertEqual(self.target.stat().st_mode & 0o777, 0o755)
                self.assertEqual(self.marker.read_text(), "initialized\n")

    def test_bad_checksum_preserves_executable(self):
        self.checksum.write_text("0" * 64 + "  gproxy-linux-amd64\n")
        self.assert_preserved("release checksum mismatch")

    def test_wrong_asset_checksum_preserves_executable(self):
        self.checksum.write_text("0" * 64 + "  other-asset\n")
        self.assert_preserved("invalid release checksum")

    def test_bad_version_preserves_executable(self):
        self.binary.write_text('#!/bin/sh\necho "go-proxy v0.1.59"\n')
        self.write_checksum()
        self.assert_preserved("release version mismatch")

    def test_download_failure_preserves_executable(self):
        self.tool("curl", "#!/bin/sh\necho fixture-download-failed >&2\nexit 22\n")
        self.assert_preserved("fixture-download-failed")

    def test_invalid_repository_fails_before_download(self):
        self.env["REPO"] = "../../invalid"
        self.assert_preserved("invalid repository")


if __name__ == "__main__":
    unittest.main()
