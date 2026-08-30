#!/usr/bin/env python3
"""
Test script for scproxy trailing-slash path handling.

Verifies that a request path ending with "/" is treated as its index
document ("/foo/" -> "/foo/index.html") consistently across:
  - upstream fetch path rewriting,
  - cache exclusion rules (excludeLastPfx),
  - cache key equivalence ("/foo/" hits the entry cached by "/foo/index.html").

This test is self-contained: it runs a local upstream HTTP server and does
not require internet access.
"""

import json
import os
import shutil
import signal
import socket
import subprocess
import sys
import threading
import time
import urllib.request
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from pathlib import Path


class Colors:
    """ANSI color codes for terminal output"""
    BLUE = '\033[0;34m'
    GREEN = '\033[0;32m'
    YELLOW = '\033[1;33m'
    RED = '\033[0;31m'
    NC = '\033[0m'  # No Color


class UpstreamHandler(BaseHTTPRequestHandler):
    """Upstream server that records requested paths and returns deterministic content"""

    def do_GET(self):
        self._serve()

    def do_POST(self):
        self._serve()

    def _serve(self):
        self.server.request_paths.append(self.path)
        body = f"content-for-{self.path}".encode()
        self.send_response(200)
        self.send_header("Content-Type", "text/plain")
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)

    def log_message(self, format, *args):
        pass  # silence request logging


class TrailingSlashTester:
    """Tests scproxy trailing-slash path handling"""

    def __init__(self):
        self.script_dir = Path(__file__).parent.parent

        bin_path = (os.environ.get('SCPROXY_BIN_PATH')
                    or os.environ.get('PRXY_BIN_PATH'))
        if bin_path:
            self.prxy_bin = Path(bin_path)
        else:
            self.prxy_bin = self.script_dir / "scproxy"

        test_dir_env = os.environ.get('TEST_CACHE_DIR')
        base = Path(test_dir_env) if test_dir_env else Path("/tmp")
        self.test_dir = base / "scproxy_trailing_slash_test"

        self.upstream_port = self.find_free_port()
        self.proxy_port = self.find_free_port()
        self.upstream = None
        self.prxy_pid = None
        self.prxy_log = None

        self.failures = []

    @staticmethod
    def find_free_port():
        with socket.socket(socket.AF_INET, socket.SOCK_STREAM) as s:
            s.bind(("127.0.0.1", 0))
            return s.getsockname()[1]

    def log_info(self, message):
        print(f"{Colors.BLUE}[INFO]{Colors.NC} {message}")

    def log_success(self, message):
        print(f"{Colors.GREEN}[SUCCESS]{Colors.NC} {message}")

    def log_error(self, message):
        print(f"{Colors.RED}[ERROR]{Colors.NC} {message}")

    def check(self, name, condition, detail=""):
        if condition:
            self.log_success(f"{name}")
        else:
            self.log_error(f"{name}" + (f" - {detail}" if detail else ""))
            self.failures.append(name)

    # ------------------------------------------------------------------ setup

    def cleanup(self):
        if self.prxy_pid:
            try:
                os.kill(self.prxy_pid, signal.SIGTERM)
                os.waitpid(self.prxy_pid, 0)
            except (ProcessLookupError, ChildProcessError):
                pass
            self.prxy_pid = None
        if self.upstream:
            self.upstream.shutdown()
            self.upstream.server_close()
            self.upstream = None

    def reset_test_dir(self):
        if self.test_dir.exists():
            shutil.rmtree(self.test_dir)
        self.test_dir.mkdir(parents=True)

    def start_upstream(self):
        self.upstream = ThreadingHTTPServer(
            ("127.0.0.1", self.upstream_port), UpstreamHandler)
        self.upstream.request_paths = []
        threading.Thread(target=self.upstream.serve_forever,
                         daemon=True).start()
        self.log_info(f"Upstream server started on 127.0.0.1:{self.upstream_port}")

    def write_config(self, exclude_last_pfx):
        cache_dir = self.test_dir / "cache"
        cache_dir.mkdir(parents=True, exist_ok=True)
        config = {
            "host": "127.0.0.1",
            "cache": {
                "enabled": True,
                "directory": str(cache_dir),
                "excludeLastPfx": exclude_last_pfx,
            },
            "logging": {"level": "debug", "output": "stdout"},
            "dns": {"enabled": False},
            "vhost": {"enabled": False},
            "tls": {"enabled": False},
            "routes": [
                {"target": f"http://127.0.0.1:{self.upstream_port}",
                 "port": self.proxy_port}
            ],
        }
        config_file = self.test_dir / "config.json"
        config_file.write_text(json.dumps(config, indent=2))
        return config_file

    def wait_for_port(self, port, timeout=10):
        deadline = time.time() + timeout
        while time.time() < deadline:
            try:
                with socket.create_connection(("127.0.0.1", port), timeout=0.5):
                    return True
            except OSError:
                time.sleep(0.1)
        return False

    def start_proxy(self, exclude_last_pfx):
        self.write_config(exclude_last_pfx)

        self.prxy_log = self.test_dir / "scproxy.log"
        with open(self.prxy_log, 'w') as log_f:
            self.prxy_pid = subprocess.Popen(
                [str(self.prxy_bin),
                 "--config", str(self.test_dir / "config.json"),
                 "--log-level", "debug"],
                stdout=log_f,
                stderr=log_f
            ).pid

        if not self.wait_for_port(self.proxy_port):
            self.dump_log()
            raise RuntimeError("scproxy failed to start")

        self.log_info(f"scproxy started on 127.0.0.1:{self.proxy_port} "
                      f"(excludeLastPfx={exclude_last_pfx})")

    def stop_proxy(self):
        if self.prxy_pid:
            try:
                os.kill(self.prxy_pid, signal.SIGTERM)
                os.waitpid(self.prxy_pid, 0)
            except (ProcessLookupError, ChildProcessError):
                pass
            self.prxy_pid = None

    def dump_log(self):
        if self.prxy_log and self.prxy_log.exists():
            print("--- scproxy log (last 30 lines) ---")
            with open(self.prxy_log, 'r') as f:
                print("".join(f.readlines()[-30:]))
            print("---")

    # --------------------------------------------------------------- requests

    def proxy_get(self, path):
        url = f"http://127.0.0.1:{self.proxy_port}{path}"
        with urllib.request.urlopen(url, timeout=10) as resp:
            return {
                "status": resp.status,
                "x_cache": resp.headers.get("X-Cache", ""),
                "body": resp.read().decode(),
            }

    def proxy_post(self, path):
        url = f"http://127.0.0.1:{self.proxy_port}{path}"
        req = urllib.request.Request(url, data=b"payload", method="POST")
        with urllib.request.urlopen(req, timeout=10) as resp:
            return {
                "status": resp.status,
                "x_cache": resp.headers.get("X-Cache", ""),
                "body": resp.read().decode(),
            }

    def upstream_count(self, path):
        return self.upstream.request_paths.count(path)

    # ----------------------------------------------------------------- tests

    def run_scenario_default_exclusion(self):
        """With the default exclusion (index.html), '/foo/' must BYPASS
        exactly like '/foo/index.html', and the upstream must receive the
        rewritten '/foo/index.html' path."""
        self.log_info("Scenario 1: default exclusion ['index.html']")
        self.reset_test_dir()
        self.start_proxy(["index.html"])

        r = self.proxy_get("/foo/")
        self.check("GET /foo/ returns 200", r["status"] == 200, f"got {r['status']}")
        self.check("GET /foo/ is BYPASS (excluded like /foo/index.html)",
                   r["x_cache"] == "BYPASS", f"X-Cache: {r['x_cache']}")
        self.check("upstream received rewritten /foo/index.html",
                   self.upstream_count("/foo/index.html") == 1,
                   f"upstream paths: {self.upstream.request_paths}")

        r = self.proxy_get("/")
        self.check("GET / returns 200", r["status"] == 200, f"got {r['status']}")
        self.check("GET / is BYPASS (resolves to /index.html)",
                   r["x_cache"] == "BYPASS", f"X-Cache: {r['x_cache']}")
        self.check("upstream received rewritten /index.html",
                   self.upstream_count("/index.html") == 1,
                   f"upstream paths: {self.upstream.request_paths}")

        r = self.proxy_get("/foo/")
        self.check("repeated GET /foo/ still BYPASS (never cached)",
                   r["x_cache"] == "BYPASS", f"X-Cache: {r['x_cache']}")
        self.check("upstream fetched again (no cache entry)",
                   self.upstream_count("/foo/index.html") == 2,
                   f"upstream paths: {self.upstream.request_paths}")

        cache_files = [f for f in (self.test_dir / "cache").rglob("*") if f.is_file()]
        index_like = [f for f in cache_files if f.name.endswith("index.html")]
        self.check("no index.html content cached under default exclusion",
                   not index_like, f"cached files: {[str(f) for f in cache_files]}")

        # Scope of the rewrite: only GET/HEAD directory paths are resolved;
        # other methods and interior "//" sequences are forwarded verbatim.
        r = self.proxy_post("/api/upload/")
        self.check("POST /api/upload/ returns 200", r["status"] == 200, f"got {r['status']}")
        self.check("POST directory path NOT rewritten (API endpoints)",
                   self.upstream_count("/api/upload/") == 1,
                   f"upstream paths: {self.upstream.request_paths}")

        r = self.proxy_get("/a//b")
        self.check("GET /a//b returns 200", r["status"] == 200, f"got {r['status']}")
        self.check("interior double slashes forwarded verbatim",
                   self.upstream_count("/a//b") == 1,
                   f"upstream paths: {self.upstream.request_paths}")

        self.stop_proxy()

    def run_scenario_no_exclusion(self):
        """Without exclusions, '/foo/' and '/foo/index.html' must share one
        cache entry: caching either one makes the other a HIT, and the
        upstream is fetched exactly once."""
        self.log_info("Scenario 2: no exclusion []")
        self.reset_test_dir()
        self.start_proxy([])
        self.upstream.request_paths = []

        r = self.proxy_get("/foo/index.html")
        self.check("GET /foo/index.html is MISS on first request",
                   r["x_cache"] in ("MISS", ""), f"X-Cache: {r['x_cache']}")
        expected_body = r["body"]
        self.check("upstream fetched /foo/index.html once",
                   self.upstream_count("/foo/index.html") == 1,
                   f"upstream paths: {self.upstream.request_paths}")

        r = self.proxy_get("/foo/")
        self.check("GET /foo/ is HIT (same cache key as /foo/index.html)",
                   r["x_cache"] == "HIT", f"X-Cache: {r['x_cache']}")
        self.check("GET /foo/ returns cached body",
                   r["body"] == expected_body, f"body: {r['body']!r}")
        self.check("upstream still fetched /foo/index.html only once",
                   self.upstream_count("/foo/index.html") == 1,
                   f"upstream paths: {self.upstream.request_paths}")

        r = self.proxy_get("/")
        self.check("GET / is MISS on first request (distinct key)",
                   r["x_cache"] in ("MISS", ""), f"X-Cache: {r['x_cache']}")
        root_body = r["body"]

        r = self.proxy_get("//")
        self.check("GET // is HIT (collapses to /index.html key)",
                   r["x_cache"] == "HIT", f"X-Cache: {r['x_cache']}")
        self.check("GET // returns same body as /",
                   r["body"] == root_body, f"body: {r['body']!r}")

        cache_files = [f for f in (self.test_dir / "cache").rglob("*") if f.is_file()]
        data_files = [f for f in cache_files if f.parent.name != ""]
        index_files = [f for f in cache_files
                       if f.relative_to(self.test_dir / "cache").as_posix().endswith("index.html")]
        self.check("exactly one cached index.html file for /foo/ + /foo/index.html",
                   len([f for f in index_files
                        if "foo" in f.relative_to(self.test_dir / "cache").as_posix()]) == 1,
                   f"cache files: {[str(f) for f in data_files]}")

        self.stop_proxy()

    # ------------------------------------------------------------------ main

    def run_tests(self):
        print("=" * 60)
        print("scproxy Trailing-Slash Path Handling Test")
        print("=" * 60)
        print()

        if not self.prxy_bin.exists():
            self.log_error(f"scproxy binary not found at {self.prxy_bin}")
            self.log_info("Please build scproxy first: make build")
            sys.exit(1)

        try:
            self.start_upstream()

            self.run_scenario_default_exclusion()
            print()
            self.run_scenario_no_exclusion()

            print()
            print("=" * 60)
            if self.failures:
                print(f"{Colors.RED}FAILED checks ({len(self.failures)}):{Colors.NC}")
                for name in self.failures:
                    print(f"  x {name}")
                sys.exit(1)
            else:
                print(f"{Colors.GREEN}All trailing-slash checks passed!{Colors.NC}")
        finally:
            self.cleanup()
            if self.test_dir.exists():
                shutil.rmtree(self.test_dir)


def main():
    tester = TrailingSlashTester()
    import atexit
    atexit.register(tester.cleanup)
    tester.run_tests()


if __name__ == "__main__":
    main()
