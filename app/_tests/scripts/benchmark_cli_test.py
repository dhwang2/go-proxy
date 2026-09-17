import argparse
import base64
import contextlib
import importlib.util
import io
import json
from pathlib import Path
import sys
import tempfile
import unittest


spec = importlib.util.spec_from_file_location(
    "benchmark_cli", Path(__file__).resolve().parents[2] / "scripts" / "benchmark-cli.py")
benchmark = importlib.util.module_from_spec(spec)
spec.loader.exec_module(benchmark)


class BenchmarkTests(unittest.TestCase):
    def test_exit_output_and_timeout(self):
        success = benchmark.measure([sys.executable, "-c", "print('synthetic-secret')"], 2, 0.002)
        self.assertEqual(success["exit_code"], 0)
        self.assertEqual(success["stdout_bytes"], 17)
        self.assertNotIn("synthetic-secret", json.dumps(success))
        failure = benchmark.measure([sys.executable, "-c", "raise SystemExit(7)"], 2, 0.002)
        self.assertEqual(failure["exit_code"], 7)
        timeout = benchmark.measure([sys.executable, "-c", "import time; time.sleep(5)"], 0.05, 0.002)
        self.assertTrue(timeout["timed_out"])
        self.assertNotEqual(timeout["exit_code"], 0)
        self.assertLess(timeout["elapsed_ms"], 2000)

    def test_rusage_is_per_process(self):
        large = benchmark.measure([sys.executable, "-c", "data = bytearray(128 * 1024 * 1024)"], 3, 0.002)
        small = benchmark.measure([sys.executable, "-c", "pass"], 3, 0.002)
        self.assertGreater(large["wait4_maxrss_kib"], small["wait4_maxrss_kib"] + 20 * 1024)

    @unittest.skipUnless(sys.platform.startswith("linux"), "Linux /proc CPU accounting")
    def test_waited_child_cpu(self):
        program = "import subprocess,sys; subprocess.run([sys.executable,'-c','sum(i*i for i in range(2000000))'],check=True)"
        result = benchmark.measure([sys.executable, "-c", program], 5, 0.002)
        self.assertEqual(result["exit_code"], 0)
        self.assertGreater(result["children_cpu_ms"], 0)
        self.assertGreater(result["sampled_peak_children"], 0)
        self.assertGreaterEqual(result["sampled_tree_peak_rss_kib"], result["sampled_parent_peak_rss_kib"])

    def test_fixtures(self):
        for size, counts in [("F1", (10, 50, 1000)), ("F2", (50, 500, 10000))]:
            with self.subTest(size=size), tempfile.TemporaryDirectory() as temporary:
                destination = Path(temporary) / size
                with contextlib.redirect_stdout(io.StringIO()):
                    benchmark.fixture(argparse.Namespace(size=size, directory=str(destination)))
                manifest = json.loads((destination / "manifest.json").read_text())
                self.assertEqual((manifest["nodes"], manifest["registered_users"], manifest["source_routing_rules"]), counts)
                self.assertLessEqual(manifest["configuration_bytes"], manifest["limit_bytes"])
                actual = sum(path.stat().st_size for path in (destination / "root").rglob("*") if path.is_file())
                self.assertEqual(actual, manifest["configuration_bytes"])
                config = json.loads((destination / "root/conf/sing-box.json").read_text())
                self.assertEqual(len(config["inbounds"]), counts[0])
                self.assertEqual(len(config["route"]["rules"]), counts[2])
                for inbound in config["inbounds"]:
                    self.assertEqual(len(inbound["users"]), counts[1])
                    self.assertEqual(len(base64.b64decode(inbound["password"])), 16)
                self.assertTrue((destination / "locks/.state.lock").is_file())
                with self.assertRaises(FileExistsError):
                    benchmark.fixture(argparse.Namespace(size=size, directory=str(destination)))

    def test_nearest_rank(self):
        self.assertEqual(benchmark.distribution(list(range(1, 101))), {"p50": 50, "p95": 95, "max": 100})
        self.assertIsNone(benchmark.distribution([]))


if __name__ == "__main__":
    unittest.main()
