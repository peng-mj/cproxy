#!/usr/bin/env python3
"""
Test script for scproxy caching functionality with diverse file types
Tests caching with various file sizes, types, and sources
"""

import os
import sys
import subprocess
import time
import signal
import shutil
from pathlib import Path
import urllib.request
import urllib.error


class Colors:
    """ANSI color codes for terminal output"""
    BLUE = '\033[0;34m'
    GREEN = '\033[0;32m'
    YELLOW = '\033[1;33m'
    RED = '\033[0;31m'
    CYAN = '\033[0;36m'
    MAGENTA = '\033[0;35m'
    BOLD = '\033[1m'
    NC = '\033[0m'  # No Color


class DiverseCacheTester:
    """Tests scproxy cache functionality with diverse file types"""

    def __init__(self):
        self.script_dir = Path(__file__).parent.parent

        # Use PRXY_BIN_PATH from environment if set (for automated testing)
        prxy_bin_path = os.environ.get('PRXY_BIN_PATH')
        if prxy_bin_path:
            self.prxy_bin = Path(prxy_bin_path)
        else:
            self.prxy_bin = self.script_dir / "scproxy"

        self.target_url = "https://mirrors.aliyun.com"
        self.github_url = "https://github.com"
        self.proxy_port = 9092  # Different port to avoid conflicts

        # Use TEST_CACHE_DIR from environment if set (for automated testing)
        test_cache_dir = os.environ.get('TEST_CACHE_DIR')
        if test_cache_dir:
            self.test_dir = Path(test_cache_dir) / "prxy_diverse_test"
        else:
            self.test_dir = Path("/tmp/prxy_diverse_test")

        self.cache_dir = self.test_dir / "cache"
        self.prxy_pid = None

        # Diverse test cases
        self.test_cases = [
            {
                "name": "Small Ubuntu package description",
                "url": "ubuntu/pool/main/liba/libaal/libaal_1.0.5-6.dsc",
                "source": "aliyun",
                "type": "dsc",
                "expected_size": 1352,
                "category": "tiny"
            },
            {
                "name": "Medium text file (Debian constitution)",
                "url": "debian/doc/constitution.txt",
                "source": "aliyun",
                "type": "txt",
                "expected_size": 31955,
                "category": "medium"
            },
            {
                "name": "Small text file (GitHub checksums)",
                "url": "Madh93/scproxy/releases/download/v0.1.0/checksums.txt",
                "source": "github",
                "type": "txt",
                "expected_size": 0,  # Unknown, will measure
                "category": "small"
            },
            {
                "name": "Large binary archive (dufs release)",
                "url": "sigoden/dufs/releases/download/v0.45.0/dufs-v0.45.0-x86_64-unknown-linux-musl.tar.gz",
                "source": "github",
                "type": "tar.gz",
                "expected_size": 0,  # Unknown, will measure
                "category": "large"
            }
        ]

        # Log file
        self.log_file = self.test_dir / "scproxy.log"

        # Results tracking
        self.results = []

    def log_info(self, message):
        """Print info message"""
        print(f"{Colors.BLUE}[INFO]{Colors.NC} {message}")

    def log_success(self, message):
        """Print success message"""
        print(f"{Colors.GREEN}[SUCCESS]{Colors.NC} {message}")

    def log_error(self, message):
        """Print error message"""
        print(f"{Colors.RED}[ERROR]{Colors.NC} {message}")

    def log_warning(self, message):
        """Print warning message"""
        print(f"{Colors.YELLOW}[WARNING]{Colors.NC} {message}")

    def log_test(self, message):
        """Print test message"""
        print(f"{Colors.MAGENTA}[TEST]{Colors.NC} {message}")

    def cleanup(self):
        """Clean up test environment"""
        self.log_info("Stopping scproxy server...")
        if self.prxy_pid:
            try:
                os.kill(self.prxy_pid, signal.SIGTERM)
                os.waitpid(self.prxy_pid, 0)
            except (ProcessLookupError, ChildProcessError):
                pass

        self.log_info("Cleaning up test directory...")
        if self.test_dir.exists():
            shutil.rmtree(self.test_dir)

    def setup(self):
        """Setup test environment"""
        # Check if scproxy binary exists
        if not self.prxy_bin.exists():
            self.log_error(f"scproxy binary not found at {self.prxy_bin}")
            self.log_info("Please build scproxy first: go build -o scproxy .")
            sys.exit(1)

        # Create test directory
        self.test_dir.mkdir(parents=True, exist_ok=True)
        os.chdir(self.test_dir)

        # Create cache directory
        self.cache_dir.mkdir(parents=True, exist_ok=True)

    def start_prxy_server(self):
        """Start scproxy server with cache enabled"""
        self.log_info("Starting scproxy server for diverse file testing...")
        self.log_info(f"Port: {self.proxy_port}")
        self.log_info(f"Cache directory: {self.cache_dir}")

        config_file = self.test_dir / "config.json"

        # Start scproxy server
        cmd = [
            str(self.prxy_bin),
            "--target", self.target_url,
            "--port", str(self.proxy_port),
            "--cache",
            "--config", str(config_file),
            "--log-level", "info"
        ]

        with open(self.log_file, 'w') as log_f:
            self.prxy_pid = subprocess.Popen(
                cmd,
                stdout=log_f,
                stderr=log_f
            ).pid

        # Wait for server to start
        time.sleep(2)

        # Check if server is running
        try:
            os.kill(self.prxy_pid, 0)
            self.log_success(f"scproxy server started (PID: {self.prxy_pid})")
        except ProcessLookupError:
            self.log_error("Failed to start scproxy server")
            with open(self.log_file, 'r') as f:
                print(f.read()[-2000:])
            sys.exit(1)

        print()

    def download_file(self, full_url, output_file):
        """Download a file from URL"""
        try:
            urllib.request.urlretrieve(full_url, output_file)
            return True, os.path.getsize(output_file)
        except urllib.error.URLError as e:
            return False, str(e)

    def test_file(self, test_case, attempt=1):
        """Test downloading a specific file"""
        name = test_case["name"]
        url_path = test_case["url"]
        source = test_case["source"]
        file_type = test_case["type"]
        category = test_case["category"]

        # Determine base URL based on source
        if source == "github":
            base_url = f"{self.github_url}/{url_path}"
        else:
            base_url = f"{self.target_url}/{url_path}"

        full_url = f"http://127.0.0.1:{self.proxy_port}/{url_path}"

        self.log_test(f"Test: {name}")
        self.log_info(f"Category: {category.upper()} | Type: {file_type} | Source: {source}")
        self.log_info(f"URL: {full_url}")

        output_file = self.test_dir / f"test_{category}_{attempt}.{file_type.replace('.', '_')}"

        start_time = time.time()
        success, result = self.download_file(full_url, output_file)
        download_time = time.time() - start_time

        if success:
            file_size = result
            size_kb = file_size / 1024

            self.log_success(f"Downloaded successfully")
            self.log_info(f"Size: {file_size:,} bytes ({size_kb:.2f} KB)")
            self.log_info(f"Time: {download_time:.2f}s")

            result_data = {
                "name": name,
                "attempt": attempt,
                "success": True,
                "size": file_size,
                "time": download_time,
                "category": category
            }
            self.results.append(result_data)
        else:
            self.log_error(f"Download failed: {result}")
            result_data = {
                "name": name,
                "attempt": attempt,
                "success": False,
                "error": str(result),
                "category": category
            }
            self.results.append(result_data)

        print()
        return result_data

    def test_cache_miss_hit(self, test_case):
        """Test cache MISS then HIT for a file"""
        name = test_case["name"]
        category = test_case["category"]

        print(f"{Colors.CYAN}{Colors.BOLD}--- Testing Cache MISS/HIT: {name} ---{Colors.NC}")
        print()

        # First download (should be MISS)
        self.log_info("Attempt 1: Cache MISS expected")
        result1 = self.test_file(test_case, attempt=1)

        if not result1["success"]:
            self.log_warning(f"Skipping cache HIT test for {name}")
            return

        time.sleep(1)

        # Second download (should be HIT)
        self.log_info("Attempt 2: Cache HIT expected")
        result2 = self.test_file(test_case, attempt=2)

        # Compare results
        if result1["success"] and result2["success"]:
            speedup = result1["time"] / result2["time"] if result2["time"] > 0 else 0
            self.log_info(f"Speedup: {speedup:.1f}x faster")

            if speedup > 1.5:
                self.log_success(f"Cache HIT confirmed (significant speedup)")
            elif speedup > 1.0:
                self.log_success(f"Cache HIT likely (moderate speedup)")
            else:
                self.log_warning(f"Cache HIT not clearly detected")

        print()

    def display_summary(self):
        """Display test summary"""
        print("=" * 60)
        print("Diverse Cache Test Summary")
        print("=" * 60)
        print()

        # Group results by category
        categories = {}
        for result in self.results:
            cat = result["category"]
            if cat not in categories:
                categories[cat] = []
            categories[cat].append(result)

        # Display by category
        for category in ["tiny", "small", "medium", "large"]:
            if category not in categories:
                continue

            print(f"{Colors.BOLD}{category.upper()} Files:{Colors.NC}")

            for result in categories[category]:
                status = f"{Colors.GREEN}✓{Colors.NC}" if result["success"] else f"{Colors.RED}✗{Colors.NC}"
                name = result["name"]
                attempt = result["attempt"]

                if result["success"]:
                    size = result["size"]
                    time_taken = result["time"]
                    print(f"  {status} {name} (attempt {attempt}): {size:,} bytes, {time_taken:.2f}s")
                else:
                    print(f"  {status} {name} (attempt {attempt}): FAILED")

            print()

        # Overall statistics
        total = len(self.results)
        successful = sum(1 for r in self.results if r["success"])
        failed = total - successful

        print(f"{Colors.BOLD}Overall Results:{Colors.NC}")
        print(f"  Total downloads: {total}")
        print(f"  {Colors.GREEN}Successful: {successful}{Colors.NC}")
        print(f"  {Colors.RED}Failed: {failed}{Colors.NC}")
        print()

        # Cache statistics
        if self.cache_dir.exists():
            cache_files = list(self.cache_dir.rglob("*"))
            cache_files = [f for f in cache_files if f.is_file()]
            total_size = sum(f.stat().st_size for f in cache_files)

            print(f"{Colors.BOLD}Cache Statistics:{Colors.NC}")
            print(f"  Files cached: {len(cache_files)}")

            # Display size in appropriate unit
            if total_size < 1024:
                print(f"  Total size: {total_size} bytes")
            elif total_size < 1024 * 1024:
                print(f"  Total size: {total_size / 1024:.2f} KB")
            else:
                print(f"  Total size: {total_size / (1024*1024):.2f} MB")

            print()

    def run_tests(self):
        """Run all diverse tests"""
        print("=" * 60)
        print("scproxy Diverse Cache Functionality Test")
        print("=" * 60)
        print()
        print("Testing cache with diverse file types and sources:")
        print("  • Tiny files (< 2KB)")
        print("  • Small files (< 10KB)")
        print("  • Medium files (< 100KB)")
        print("  • Large files (> 100KB)")
        print("  • Different sources (Aliyun, GitHub)")
        print("  • Different file types (.dsc, .txt, .tar.gz)")
        print()

        # Setup
        self.setup()

        try:
            # Start server
            self.start_prxy_server()

            # Test each file type
            for i, test_case in enumerate(self.test_cases, 1):
                print(f"{Colors.CYAN}{Colors.BOLD}=== Test Group {i}/{len(self.test_cases)} ==={Colors.NC}")
                self.test_cache_miss_hit(test_case)

            # Display summary
            self.display_summary()

            print(f"{Colors.GREEN}{Colors.BOLD}✓ All diverse cache tests completed!{Colors.NC}")
            print()

        finally:
            # Cleanup
            self.cleanup()


def main():
    """Main entry point"""
    tester = DiverseCacheTester()

    # Set up cleanup on exit
    import atexit
    atexit.register(tester.cleanup)

    # Run tests
    tester.run_tests()


if __name__ == "__main__":
    main()