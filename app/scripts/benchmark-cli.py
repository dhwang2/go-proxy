#!/usr/bin/env python3
"""Measure fresh CLI processes; generate isolated, synthetic capacity fixtures."""

import argparse
import base64
import hashlib
import json
import math
import os
from pathlib import Path
import platform
import resource
import signal
import subprocess
import sys
import tempfile
import threading
import time


STARTUP = {"help": ["--help"], "version": ["version"]}
LOCAL = {
    "status": ["status"],
    "status_json": ["status", "--json"],
    "user": ["user"],
    "user_json": ["user", "--json"],
    "protocol": ["protocol"],
    "protocol_json": ["protocol", "--json"],
    "routing": ["routing"],
    "routing_json": ["routing", "--json"],
    "config": ["config", "view", "sing-box"],
    "config_json": ["config", "view", "sing-box", "--json"],
    "sub": ["sub", "--target", "192.0.2.1"],
    "sub_json": ["sub", "--target", "192.0.2.1", "--json"],
    "sub_sing_box": ["sub", "--target", "192.0.2.1", "--sing-box"],
}


def distribution(values):
    values = sorted(v for v in values if v is not None)
    if not values:
        return None
    return {
        "p50": round(values[math.ceil(len(values) * 0.50) - 1], 3),
        "p95": round(values[math.ceil(len(values) * 0.95) - 1], 3),
        "max": round(values[-1], 3),
    }


def proc_stat(pid):
    try:
        raw = Path(f"/proc/{pid}/stat").read_text()
        fields = raw[raw.rfind(")") + 2:].split()
        ticks = os.sysconf("SC_CLK_TCK")
        return {
            "parent_cpu_ms": (int(fields[11]) + int(fields[12])) * 1000 / ticks,
            "children_cpu_ms": (int(fields[13]) + int(fields[14])) * 1000 / ticks,
            "rss_kib": int(fields[21]) * os.sysconf("SC_PAGE_SIZE") / 1024,
        }
    except (FileNotFoundError, ProcessLookupError):
        return None


def observe_tree(pid):
    pending, seen = [pid], set()
    total, parent, highwater = 0, 0, 0
    while pending:
        current = pending.pop()
        if current in seen:
            continue
        seen.add(current)
        stat = proc_stat(current)
        if stat is None:
            continue
        total += stat["rss_kib"]
        if current == pid:
            parent = stat["rss_kib"]
            try:
                for line in Path(f"/proc/{pid}/status").read_text().splitlines():
                    if line.startswith("VmHWM:"):
                        highwater = int(line.split()[1])
            except (FileNotFoundError, ProcessLookupError):
                pass
        try:
            for task in Path(f"/proc/{current}/task").iterdir():
                pending.extend(int(p) for p in (task / "children").read_text().split())
        except (FileNotFoundError, ProcessLookupError):
            pass
    return max(parent, highwater), total, max(0, len(seen) - 1)


def measure(argv, timeout, interval):
    linux = sys.platform.startswith("linux")
    finished = threading.Event()
    observed = {"parent_rss": 0, "tree_rss": 0, "children": 0, "timed_out": False}
    with tempfile.TemporaryFile() as output, tempfile.TemporaryFile() as errors:
        started = time.perf_counter_ns()
        child = subprocess.Popen(argv, stdin=subprocess.DEVNULL, stdout=output,
                                 stderr=errors, start_new_session=True)

        def sample():
            while not finished.is_set():
                if linux:
                    parent, tree, children = observe_tree(child.pid)
                    observed["parent_rss"] = max(observed["parent_rss"], parent)
                    observed["tree_rss"] = max(observed["tree_rss"], tree)
                    observed["children"] = max(observed["children"], children)
                if time.perf_counter_ns() - started > timeout * 1e9:
                    observed["timed_out"] = True
                    try:
                        os.killpg(child.pid, signal.SIGKILL)
                    except ProcessLookupError:
                        pass
                    return
                finished.wait(interval)

        observer = threading.Thread(target=sample, daemon=True)
        observer.start()
        final = None
        try:
            if linux:
                # Preserve the zombie long enough to read final parent/child CPU ticks.
                os.waitid(os.P_PID, child.pid, os.WEXITED | os.WNOWAIT)
                elapsed = (time.perf_counter_ns() - started) / 1e6
                final = proc_stat(child.pid)
                _, status, usage = os.wait4(child.pid, 0)
            else:
                _, status, usage = os.wait4(child.pid, 0)
                elapsed = (time.perf_counter_ns() - started) / 1e6
            child.returncode = os.waitstatus_to_exitcode(status)
        except BaseException:
            try:
                os.killpg(child.pid, signal.SIGKILL)
            except ProcessLookupError:
                pass
            child.wait()
            raise
        finally:
            finished.set()
            observer.join()
        return {
            "exit_code": child.returncode,
            "timed_out": observed["timed_out"],
            "elapsed_ms": elapsed,
            "stdout_bytes": os.fstat(output.fileno()).st_size,
            "stderr_bytes": os.fstat(errors.fileno()).st_size,
            "wait4_cpu_ms": (usage.ru_utime + usage.ru_stime) * 1000,
            "wait4_user_ms": usage.ru_utime * 1000,
            "wait4_system_ms": usage.ru_stime * 1000,
            "wait4_maxrss_kib": usage.ru_maxrss / (1024 if sys.platform == "darwin" else 1),
            "parent_cpu_ms": final["parent_cpu_ms"] if final else None,
            "children_cpu_ms": final["children_cpu_ms"] if final else None,
            "sampled_parent_peak_rss_kib": observed["parent_rss"] if linux else None,
            "sampled_tree_peak_rss_kib": observed["tree_rss"] if linux else None,
            "sampled_peak_children": observed["children"] if linux else None,
        }


def fixture(args):
    nodes, users, rules, limit = {"F1": (10, 50, 1000, 5), "F2": (50, 500, 10000, 25)}[args.size]
    destination = Path(args.directory).resolve()
    destination.mkdir(mode=0o700)
    root = destination / "root"
    for directory in [root, root / "conf", root / "bin", root / "logs", destination / "locks"]:
        directory.mkdir(mode=0o700)
    for name in [".state.lock", ".operation.lock"]:
        (destination / "locks" / name).touch(mode=0o600)
    names = [f"bench-user-{i:04d}" for i in range(users)]
    inbounds = []
    for node in range(nodes):
        members = []
        for name in names:
            key = hashlib.sha256(f"synthetic-only:{node}:{name}".encode()).digest()[:16]
            members.append({"name": name, "password": base64.b64encode(key).decode()})
        server_key = hashlib.sha256(f"synthetic-only:server:{node}".encode()).digest()[:16]
        inbounds.append({"type": "shadowsocks", "tag": f"ss_{20000 + node}",
                         "listen": "::", "listen_port": 20000 + node,
                         "method": "2022-blake3-aes-128-gcm",
                         "password": base64.b64encode(server_key).decode(), "users": members})
    route_rules = [{"action": "route", "outbound": "🐸 direct",
                    "auth_user": [names[i % users]],
                    "domain": [f"route-{i:05d}.benchmark.invalid"]} for i in range(rules)]
    configuration = {
        "log": {"disabled": False, "level": "error", "output": "/etc/go-proxy/logs/sing-box.service.log", "timestamp": True},
        "experimental": {"cache_file": {"enabled": True, "cache_id": "cache.db", "path": "/etc/go-proxy/cache.db", "store_fakeip": False, "store_rdrc": True}},
        "dns": {"servers": [{"tag": "public4", "type": "https", "server": "8.8.8.8", "server_port": 443,
                             "path": "/dns-query", "tls": {"enabled": True, "server_name": "dns.google"}}],
                "rules": [], "final": "public4", "strategy": "ipv4_only", "reverse_mapping": True,
                "independent_cache": True, "cache_capacity": 8192},
        "inbounds": inbounds,
        "outbounds": [{"type": "direct", "tag": "🐸 direct"}],
        "route": {"final": "🐸 direct", "default_domain_resolver": "public4", "rules": route_rules},
    }
    files = {
        "conf/sing-box.json": configuration,
        "user-management.json": {"schema": 3, "groups": {"default": names}},
        "user-route-rules.json": route_rules,
        "user-route-templates.json": {"templates": {}},
        "firewall-ports.json": {"ports": []},
    }
    sizes = {}
    for name, value in files.items():
        data = (json.dumps(value, ensure_ascii=False, indent=2) + "\n").encode()
        path = root / name
        with path.open("xb") as output:
            output.write(data)
        path.chmod(0o600)
        sizes[name] = len(data)
    manifest = {"fixture": args.size, "nodes": nodes, "registered_users": users,
                "memberships": nodes * users, "source_routing_rules": rules,
                "compiled_routing_rules": rules, "configuration_bytes": sum(sizes.values()),
                "file_bytes": sizes, "limit_bytes": limit * 1024 * 1024,
                "credentials": "deterministic synthetic test values; never deploy as real credentials"}
    if manifest["configuration_bytes"] > manifest["limit_bytes"]:
        raise ValueError("fixture exceeds configuration size budget")
    (destination / "manifest.json").write_text(json.dumps(manifest, indent=2) + "\n")
    print(json.dumps(manifest, indent=2))


def run(args):
    if args.fixture:
        if not sys.platform.startswith("linux"):
            raise ValueError("fixture mounts require Linux")
        mount_ns = os.readlink("/proc/self/ns/mnt")
        if not args.parent_mount_ns:
            command = ["unshare", "--mount", "--propagation", "private", "--", sys.executable,
                       str(Path(__file__).resolve()), *sys.argv[1:], "--parent-mount-ns", mount_ns]
            return subprocess.call(command)
        if args.parent_mount_ns == mount_ns:
            raise ValueError("refusing fixture mounts without a private mount namespace")
        for source, target in [("root", "/etc/go-proxy"), ("locks", "/run/lock/go-proxy")]:
            subprocess.run(["mount", "--bind", str(Path(args.fixture).resolve() / source), target], check=True)
        # The benchmark executes only queries and locks are pre-created.
        subprocess.run(["mount", "-o", "remount,bind,ro", "/etc/go-proxy"], check=True)
    commands = {**STARTUP, **LOCAL} if args.suite == "all" else (STARTUP if args.suite == "startup" else LOCAL)
    if args.commands:
        commands = json.loads(Path(args.commands).read_text())
        if not isinstance(commands, dict) or not commands:
            raise ValueError("commands must be a nonempty JSON object of names to argument arrays")
        if any(not isinstance(k, str) or not isinstance(v, list) or any(not isinstance(x, str) for x in v)
               for k, v in commands.items()):
            raise ValueError("commands must map names to string argument arrays")
    binary = Path(args.binary).resolve(strict=True)
    digest = hashlib.sha256()
    with binary.open("rb") as source:
        for chunk in iter(lambda: source.read(1024 * 1024), b""):
            digest.update(chunk)
    host = {"architecture": platform.machine(), "os": platform.system(), "logical_cpus": os.cpu_count(),
            "affinity_cpus": len(os.sched_getaffinity(0)) if hasattr(os, "sched_getaffinity") else None}
    if Path("/proc/meminfo").exists():
        host["memory_kib"] = {line.split(":")[0]: int(line.split()[1])
                              for line in Path("/proc/meminfo").read_text().splitlines()
                              if line.split(":")[0] in {"MemTotal", "MemAvailable", "SwapTotal", "SwapFree"}}
        host["cpu_ticks"] = [int(value) for value in Path("/proc/stat").read_text().splitlines()[0].split()[1:]]
    report = {"binary_sha256": digest.hexdigest(), "host_before": host, "warmups": args.warmups,
              "samples": args.samples, "process_sample_interval_ms": args.interval_ms,
              "percentile_method": "nearest rank; successful measured samples only",
              "measurement_notes": [
                  "first_launch is a fresh process, not a forced cold-page-cache launch",
                  "wait4 CPU includes waited descendants; its max RSS is the largest process, not simultaneous tree RSS",
                  "wait4 max RSS can include inherited pre-exec memory; use post-exec /proc RSS sampling for the CLI parent",
                  "Linux final /proc CPU separates parent and waited children at clock-tick resolution",
                  "sampled RSS and child counts are lower bounds and may miss short-lived processes",
                  "RSS sums double-count shared pages; this is not cgroup charged memory",
                  "no stdout, stderr, arguments, host addresses or credentials are retained",
                  "output is drained to private temporary files; terminal rendering is excluded",
              ], "commands": {}}
    if args.fixture:
        report["fixture"] = json.loads((Path(args.fixture) / "manifest.json").read_text())
    harness_before = resource.getrusage(resource.RUSAGE_SELF)
    started = time.monotonic()
    failed = False
    for name, arguments in commands.items():
        print(f"measuring {name}", file=sys.stderr, flush=True)
        argv = [str(binary), *arguments]
        first = measure(argv, args.timeout, args.interval_ms / 1000)
        warmup_failures = 0
        for _ in range(args.warmups):
            if measure(argv, args.timeout, args.interval_ms / 1000)["exit_code"] != 0:
                warmup_failures += 1
        samples = []
        progress = time.monotonic()
        for i in range(args.samples):
            samples.append(measure(argv, args.timeout, args.interval_ms / 1000))
            if time.monotonic() - progress >= 5:
                print(f"{name}: {i + 1}/{args.samples}", file=sys.stderr, flush=True)
                progress = time.monotonic()
        successful = [sample for sample in samples if sample["exit_code"] == 0 and not sample["timed_out"]]
        failures = [sample for sample in samples if sample["exit_code"] != 0 or sample["timed_out"]]
        exit_counts = {}
        for sample in samples:
            code = str(sample["exit_code"])
            exit_counts[code] = exit_counts.get(code, 0) + 1
        report["commands"][name] = {
            "first_launch": first, "warmup_failures": warmup_failures,
            "successes": len(successful), "failures": len(failures), "exit_counts": exit_counts,
            "timeouts": sum(sample["timed_out"] for sample in samples),
            "success": {metric: distribution([sample[metric] for sample in successful])
                        for metric in first if metric not in {"exit_code", "timed_out"}},
            "failure": {metric: distribution([sample[metric] for sample in failures])
                        for metric in first if metric not in {"exit_code", "timed_out"}},
            "failure_elapsed_ms": distribution([sample["elapsed_ms"] for sample in failures]),
        }
        latency = report["commands"][name]["success"]["elapsed_ms"]
        rss = report["commands"][name]["success"]["sampled_parent_peak_rss_kib"]
        print(f"{name}: successes={len(successful)} failures={len(failures)} "
              f"p95_ms={latency['p95'] if latency else None} "
              f"parent_rss_max_kib={rss['max'] if rss else None}", file=sys.stderr, flush=True)
        failed = failed or bool(failures or warmup_failures or first["exit_code"])
    if Path("/proc/meminfo").exists():
        report["memory_kib_after"] = {line.split(":")[0]: int(line.split()[1])
                                      for line in Path("/proc/meminfo").read_text().splitlines()
                                      if line.split(":")[0] in {"MemAvailable", "SwapFree"}}
        report["cpu_ticks_after"] = [int(value) for value in Path("/proc/stat").read_text().splitlines()[0].split()[1:]]
    harness_after = resource.getrusage(resource.RUSAGE_SELF)
    report["harness_elapsed_seconds"] = time.monotonic() - started
    report["harness_cpu_ms"] = ((harness_after.ru_utime - harness_before.ru_utime)
                                + (harness_after.ru_stime - harness_before.ru_stime)) * 1000
    print(json.dumps(report, indent=2))
    return 1 if failed else 0


def watchdog(args):
    if not sys.platform.startswith("linux"):
        raise ValueError("watchdog observation requires Linux")
    properties = ["MainPID", "ControlGroup", "MemoryCurrent", "MemoryPeak", "CPUUsageNSec"]
    command = ["systemctl", "show", args.unit, "--property=" + ",".join(properties), "--no-pager"]
    before = dict(line.split("=", 1) for line in subprocess.check_output(command, text=True).splitlines())
    pid = int(before["MainPID"])
    if pid <= 0:
        raise ValueError("watchdog unit is not running")
    cgroup = Path("/sys/fs/cgroup") / before["ControlGroup"].lstrip("/")
    started, first = time.monotonic(), proc_stat(pid)
    parent_peak, tree_peak, child_peak, cgroup_peak = 0, 0, 0, 0
    rss_values, available, vm_first = [], [], {}
    for line in Path("/proc/vmstat").read_text().splitlines():
        key, value = line.split()
        if key in {"oom_kill", "pswpin", "pswpout"}:
            vm_first[key] = int(value)
    progress = started
    while time.monotonic() - started < args.seconds:
        parent, tree, children = observe_tree(pid)
        stat = proc_stat(pid)
        if stat is None:
            raise ValueError("watchdog process exited during observation")
        parent_peak, tree_peak = max(parent_peak, parent), max(tree_peak, tree)
        child_peak = max(child_peak, children)
        rss_values.append(stat["rss_kib"])
        if (cgroup / "memory.current").exists():
            cgroup_peak = max(cgroup_peak, int((cgroup / "memory.current").read_text()))
        for line in Path("/proc/meminfo").read_text().splitlines():
            if line.startswith("MemAvailable:"):
                available.append(int(line.split()[1]))
        if time.monotonic() - progress >= 30:
            print(f"watchdog: {time.monotonic() - started:.0f}/{args.seconds:g}s", file=sys.stderr, flush=True)
            progress = time.monotonic()
        time.sleep(min(1, max(0, args.seconds - (time.monotonic() - started))))
    elapsed, last = time.monotonic() - started, proc_stat(pid)
    after = dict(line.split("=", 1) for line in subprocess.check_output(command, text=True).splitlines())
    if last is None or after["MainPID"] != before["MainPID"]:
        raise ValueError("watchdog process changed during observation")
    counters = {}
    for line in Path("/proc/vmstat").read_text().splitlines():
        key, value = line.split()
        if key in vm_first:
            counters[key] = int(value) - vm_first[key]
    cpu_ns = int(after["CPUUsageNSec"]) - int(before["CPUUsageNSec"])
    print(json.dumps({
        "unit": args.unit, "observation_label": args.label, "seconds": elapsed,
        "samples": len(rss_values), "sample_interval_seconds": 1,
        "parent_rss_first_kib": rss_values[0], "parent_rss_last_kib": rss_values[-1],
        "parent_rss_max_kib": max(rss_values), "parent_vm_hwm_kib": parent_peak,
        "sampled_tree_peak_rss_kib": tree_peak, "sampled_peak_children": child_peak,
        "sampled_cgroup_memory_peak_bytes": cgroup_peak or None,
        "unit_memory_before": before["MemoryCurrent"], "unit_memory_after": after["MemoryCurrent"],
        "unit_memory_peak_lifetime": after.get("MemoryPeak"),
        "unit_cpu_ms": cpu_ns / 1e6, "unit_cpu_percent_of_one_core": cpu_ns / (elapsed * 1e7),
        "parent_cpu_ms": last["parent_cpu_ms"] - first["parent_cpu_ms"],
        "waited_children_cpu_ms": last["children_cpu_ms"] - first["children_cpu_ms"],
        "host_min_mem_available_kib": min(available), "host_vmstat_delta": counters,
        "notes": ["unit CPU accounting includes children, sampled process-tree RSS is a lower bound",
                  "VmHWM and unit MemoryPeak include process/unit lifetime before this observation",
                  "cgroup charged memory includes file cache and kernel allocations",
                  "observation_label declares the concurrent workload; busy time is not idle evidence"],
    }, indent=2))
    return 0


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    sub = parser.add_subparsers(dest="action", required=True)
    generate = sub.add_parser("fixture", help="create a new synthetic F1/F2 directory")
    generate.add_argument("size", choices=["F1", "F2"])
    generate.add_argument("directory", help="new directory; existing paths are never overwritten")
    bench = sub.add_parser("run", help="measure the selected binary; never captures command contents")
    bench.add_argument("--binary", required=True)
    bench.add_argument("--suite", choices=["startup", "local", "all"], default="all")
    bench.add_argument("--commands", help="JSON file mapping safe report labels to argv arrays")
    bench.add_argument("--fixture", help="Linux: bind the generated fixture in a private mount namespace")
    bench.add_argument("--samples", type=int, default=100)
    bench.add_argument("--warmups", type=int, default=5)
    bench.add_argument("--timeout", type=float, default=10)
    bench.add_argument("--interval-ms", type=float, default=2)
    bench.add_argument("--parent-mount-ns", help=argparse.SUPPRESS)
    watch = sub.add_parser("watchdog", help="observe an existing systemd watchdog without changing it")
    watch.add_argument("--unit", default="proxy-watchdog.service")
    watch.add_argument("--seconds", type=float, default=600)
    watch.add_argument("--label", required=True, help="describe concurrent workload honestly")
    args = parser.parse_args()
    if args.action == "run" and (args.samples < 1 or args.warmups < 0 or args.timeout <= 0 or args.interval_ms <= 0
                                 or not math.isfinite(args.timeout) or not math.isfinite(args.interval_ms)):
        parser.error("samples, timeout and interval must be positive; warmups must be nonnegative")
    if args.action == "watchdog" and (not math.isfinite(args.seconds) or args.seconds <= 0):
        parser.error("seconds must be positive")
    try:
        if args.action == "run":
            return run(args)
        if args.action == "watchdog":
            return watchdog(args)
        return fixture(args)
    except (OSError, ValueError, subprocess.CalledProcessError) as error:
        print(f"benchmark error: {type(error).__name__}", file=sys.stderr)
        return 2


if __name__ == "__main__":
    sys.exit(main())
