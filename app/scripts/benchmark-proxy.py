#!/usr/bin/env python3
"""Observe a rate-limited proxy workload with and without lightweight CLI queries."""

import argparse
import hashlib
import json
import math
import os
from pathlib import Path
import signal
import socket
import subprocess
import sys
import tempfile
import threading
import time


def summarize(samples):
    values = sorted(sample["elapsed_ms"] for sample in samples if sample["ok"])
    return {
        "requests": len(samples), "successes": len(values), "failures": len(samples) - len(values),
        "p50_ms": values[math.ceil(len(values) * .5) - 1] if values else None,
        "p95_ms": values[math.ceil(len(values) * .95) - 1] if values else None,
        "max_ms": values[-1] if values else None,
        "response_bytes": sorted({sample["response_bytes"] for sample in samples if sample["ok"]}),
    }


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--gproxy", required=True)
    parser.add_argument("--core", required=True, help="existing sing-box client binary")
    parser.add_argument("--user", required=True)
    parser.add_argument("--node", required=True)
    parser.add_argument("--target", required=True)
    parser.add_argument("--url", default="https://example.com")
    parser.add_argument("--requests", type=int, default=30, help="requests per window")
    parser.add_argument("--repeats", type=int, default=2, help="baseline/loaded window pairs")
    parser.add_argument("--request-rate", type=float, default=2)
    parser.add_argument("--query-rate", type=float, default=4)
    args = parser.parse_args()
    if args.requests < 1 or args.repeats < 1 or not 0 < args.request_rate <= 10 or not 0 < args.query_rate <= 10:
        parser.error("request/repeat counts must be positive and rates must be between 0 and 10 per second")
    export = [args.gproxy, "sub", args.user, "--node", args.node, "--target", args.target, "--sing-box"]
    config = json.loads(subprocess.check_output(export, stdin=subprocess.DEVNULL, stderr=subprocess.DEVNULL, timeout=10))
    if len(config.get("outbounds", [])) != 1:
        raise ValueError("select exactly one exported outbound")
    outbound = config["outbounds"][0]
    if outbound.get("tls", {}).get("insecure", False):
        raise ValueError("client export disables certificate verification")
    protocol = outbound["type"]
    outbound["tag"] = "benchmark-proxy"
    with socket.socket() as listener:
        listener.bind(("127.0.0.1", 0))
        port = listener.getsockname()[1]
    config["log"] = {"level": "error"}
    config["inbounds"] = [{"type": "socks", "tag": "benchmark-socks", "listen": "127.0.0.1", "listen_port": port}]
    config["route"] = {"final": "benchmark-proxy"}
    commands = [[args.gproxy, *arguments] for arguments in [
        ["status", "--json"], ["user", "--json"], ["protocol", "--json"],
        ["routing", "--json"], ["sub", args.user, "--node", args.node, "--target", args.target, "--sing-box"],
    ]]
    results = []
    with tempfile.TemporaryDirectory(prefix="gproxy-proxy-benchmark-") as temporary:
        path = Path(temporary) / "client.json"
        path.write_text(json.dumps(config))
        path.chmod(0o600)
        subprocess.run([args.core, "check", "-c", str(path)], stdin=subprocess.DEVNULL,
                       stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL, check=True, timeout=10)
        with tempfile.TemporaryFile() as log:
            client = subprocess.Popen([args.core, "run", "-c", str(path)], stdin=subprocess.DEVNULL,
                                      stdout=log, stderr=log, start_new_session=True)
            try:
                deadline = time.monotonic() + 5
                while True:
                    try:
                        with socket.create_connection(("127.0.0.1", port), timeout=.2):
                            break
                    except OSError:
                        if time.monotonic() >= deadline or client.poll() is not None:
                            raise RuntimeError("client did not become ready")
                        time.sleep(.05)
                for repeat in range(args.repeats):
                    for loaded in [False, True]:
                        label = "loaded" if loaded else "baseline"
                        print(f"proxy: {label} window {repeat + 1}/{args.repeats}", file=sys.stderr, flush=True)
                        stop = threading.Event()
                        counts = {"successes": 0, "failures": 0}

                        def queries():
                            index = 0
                            while not stop.is_set():
                                began = time.monotonic()
                                try:
                                    result = subprocess.run(commands[index % len(commands)], stdin=subprocess.DEVNULL,
                                                            stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL, timeout=5)
                                    counts["successes" if result.returncode == 0 else "failures"] += 1
                                except subprocess.TimeoutExpired:
                                    counts["failures"] += 1
                                index += 1
                                stop.wait(max(0, 1 / args.query_rate - (time.monotonic() - began)))

                        worker = threading.Thread(target=queries)
                        if loaded:
                            worker.start()
                        samples, started = [], time.monotonic()
                        try:
                            for index in range(args.requests):
                                time.sleep(max(0, started + index / args.request_rate - time.monotonic()))
                                began = time.monotonic()
                                result = subprocess.run([
                                    "curl", "--silent", "--show-error", "--noproxy", "",
                                    "--proxy", f"socks5h://127.0.0.1:{port}", "--connect-timeout", "3", "--max-time", "8",
                                    "--output", "/dev/null", "--write-out", "%{http_code} %{time_total} %{size_download}",
                                    "--url", args.url,
                                ], stdin=subprocess.DEVNULL, stdout=subprocess.PIPE, stderr=subprocess.DEVNULL, timeout=10, text=True)
                                status, elapsed, size = result.stdout.split()
                                samples.append({"ok": result.returncode == 0 and 200 <= int(status) < 300,
                                                "exit_code": result.returncode, "http_status": int(status),
                                                "elapsed_ms": float(elapsed) * 1000, "response_bytes": int(size),
                                                "process_elapsed_ms": (time.monotonic() - began) * 1000})
                        finally:
                            stop.set()
                            if loaded:
                                worker.join()
                        results.append({"mode": label, "seconds": time.monotonic() - started,
                                        "queries": counts, "summary": summarize(samples), "samples": samples})
            finally:
                if client.poll() is None:
                    os.killpg(client.pid, signal.SIGTERM)
                    try:
                        client.wait(timeout=3)
                    except subprocess.TimeoutExpired:
                        os.killpg(client.pid, signal.SIGKILL)
                        client.wait()
    summary = {mode: summarize([sample for window in results if window["mode"] == mode for sample in window["samples"]])
               for mode in ["baseline", "loaded"]}
    print(json.dumps({
        "binary_sha256": hashlib.sha256(Path(args.gproxy).read_bytes()).hexdigest(), "protocol": protocol,
        "request_rate": args.request_rate, "query_rate": args.query_rate, "summary": summary, "windows": results,
        "notes": ["rate-limited same-host client observation, not a saturated throughput-capacity acceptance test",
                  "external destination latency/noise is included; ratios do not establish a universal 5%/10% bound",
                  "generated client TLS verification settings are preserved; temporary client and credentials are removed",
                  "configured service state is not modified; only temporary client processes are started"],
    }, indent=2))
    return int(any(summary[mode]["failures"] for mode in summary) or any(window["queries"]["failures"] for window in results))


if __name__ == "__main__":
    try:
        sys.exit(main())
    except (OSError, ValueError, RuntimeError, subprocess.SubprocessError) as error:
        print(f"proxy benchmark error: {type(error).__name__}", file=sys.stderr)
        sys.exit(2)
