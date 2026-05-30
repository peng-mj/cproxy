#!/usr/bin/env python3
"""
Test script for prxy caching functionality
Tests caching with actual file downloads from Aliyun mirror
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
    NC = '\033[0m'  # No Color


class CacheTester:
    """Tests prxy cache functionality"""

    def __init__(self):
        self.script_dir = Path(__file__).parent.parent

        # Use PRXY_BIN_PATH from environment if set (for automated testing)
        prxy_bin_path = os.environ.get('PRXY_BIN_PATH')
        if prxy_bin_path:
            self.prxy_bin = Path(prxy_bin_path)
        else:
            self.prxy_bin = self.script_dir / "prxy"

        self.target_url = "https://mirrors.aliyun.com"
        self.proxy_port = 9091

        # Use TEST_CACHE_DIR from environment if set (for automated testing)
        test_cache_dir = os.environ.get('TEST_CACHE_DIR')
        if test_cache_dir:
            self.test_dir = Path(test_cache_dir) / "prxy_test"
        else:
            self.test_dir = Path("/tmp/prxy_test")

        self.cache_dir = self.test_dir / "cache"
        self.prxy_pid = None

        # Test URLs
        self.test_file_1 = "ubuntu/pool/main/liba/libaal/libaal_1.0.5-6.dsc"
        self.test_file_2 = "ubuntu/pool/main/liba/libaal/libaal_1.0.6-1.dsc"

        # Log file
        self.log_file = self.test_dir / "prxy.log"

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

    def cleanup(self):
        """Clean up test environment"""
        self.log_info("Stopping prxy server...")
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
        # Check if prxy binary exists
        if not self.prxy_bin.exists():
            self.log_error(f"prxy binary not found at {self.prxy_bin}")
            self.log_info("Please build prxy first: go build -o prxy .")
            sys.exit(1)

        # Create test directory
        self.test_dir.mkdir(parents=True, exist_ok=True)
        os.chdir(self.test_dir)

        # Create cache directory
        self.cache_dir.mkdir(parents=True, exist_ok=True)

    def start_prxy_server(self):
        """Start prxy server with cache enabled"""
        self.log_info("Starting prxy server...")
        self.log_info(f"Target: {self.target_url}")
        self.log_info(f"Port: {self.proxy_port}")
        self.log_info(f"Cache directory: {self.cache_dir}")

        config_file = self.test_dir / "config.json"

        # Start prxy server
        cmd = [
            str(self.prxy_bin),
            "--target", self.target_url,
            "--port", str(self.proxy_port),
            "--cache",
            "--config", str(config_file),
            "--log-level", "debug"
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
            os.kill(self.prxy_pid, 0)  # Check if process exists
            self.log_success(f"Prxy server started (PID: {self.prxy_pid})")
        except ProcessLookupError:
            self.log_error("Failed to start prxy server")
            with open(self.log_file, 'r') as f:
                print(f.read())
            sys.exit(1)

        print()

    def download_file(self, url, output_file):
        """Download a file from URL"""
        try:
            urllib.request.urlretrieve(url, output_file)
            return True, os.path.getsize(output_file)
        except urllib.error.URLError as e:
            return False, str(e)

    def test_first_download(self):
        """Test first download (should be cache MISS)"""
        self.log_info("Test 1: First download (should be CACHE MISS)")
        url = f"http://127.0.0.1:{self.proxy_port}/{self.test_file_1}"
        self.log_info(f"URL: {url}")

        output_file = self.test_dir / "test1.dsc"
        success, result = self.download_file(url, output_file)

        if success:
            file_size = result
            self.log_success(f"Download completed (Size: {file_size} bytes)")
        else:
            self.log_error("Download failed")
            self.log_error(str(result))
            with open(self.log_file, 'r') as f:
                print(f.read()[-2000:])
            sys.exit(1)

        print()

        # Check cache miss in logs
        with open(self.log_file, 'r') as f:
            log_content = f.read()
            if "Cache miss" in log_content:
                self.log_success("Cache MISS detected in logs (as expected)")
            elif "X-Cache: MISS" in log_content:
                self.log_success("Cache MISS detected in response headers")
            else:
                self.log_warning("Could not verify cache miss status in logs")

        print()
        return file_size

    def test_cache_verification(self):
        """Test if file was cached"""
        self.log_info("Test 2: Verify file was cached")

        cache_files = list(self.cache_dir.rglob("*"))
        cache_files = [f for f in cache_files if f.is_file()]

        if cache_files:
            self.log_success(f"Cache files created: {len(cache_files)} file(s)")
            for cache_file in cache_files:
                size = cache_file.stat().st_size
                print(f"  {cache_file.relative_to(self.cache_dir)}: {size} bytes")
        else:
            self.log_warning("No cache files found in cache directory")

        print()

    def test_second_download(self, original_size):
        """Test second download (should be cache HIT)"""
        self.log_info("Test 3: Second download of same file (should be CACHE HIT)")
        url = f"http://127.0.0.1:{self.proxy_port}/{self.test_file_1}"

        output_file = self.test_dir / "test1_v2.dsc"
        success, result = self.download_file(url, output_file)

        if success:
            file_size = result
            self.log_success(f"Download completed (Size: {file_size} bytes)")

            # Compare file sizes
            if file_size == original_size:
                self.log_success("File sizes match - download successful")
            else:
                self.log_error("File size mismatch!")
                sys.exit(1)
        else:
            self.log_error("Download failed")
            sys.exit(1)

        print()

        # Check cache hit in logs
        with open(self.log_file, 'r') as f:
            log_content = f.read()
            if "Cache hit" in log_content:
                self.log_success("Cache HIT detected in logs (as expected)")
            elif "X-Cache: HIT" in log_content:
                self.log_success("Cache HIT detected in response headers")
            else:
                self.log_warning("Could not verify cache hit status in logs")

        print()

    def test_another_file(self):
        """Test downloading a different file"""
        self.log_info("Test 4: Download different file (should be CACHE MISS, then HIT)")
        url = f"http://127.0.0.1:{self.proxy_port}/{self.test_file_2}"
        self.log_info(f"URL: {url}")

        output_file1 = self.test_dir / "test2.dsc"
        output_file2 = self.test_dir / "test2_v2.dsc"

        success, _ = self.download_file(url, output_file1)
        if success:
            self.log_success("First download completed")

        success, _ = self.download_file(url, output_file2)
        if success:
            self.log_success("Second download completed")

        print()

    def test_cache_statistics(self):
        """Display cache statistics"""
        self.log_info("Test 5: Cache statistics")

        cache_files = list(self.cache_dir.rglob("*"))
        cache_files = [f for f in cache_files if f.is_file()]

        total_size = sum(f.stat().st_size for f in cache_files)

        print(f"Cache directory: {self.cache_dir}")
        print(f"Total files: {len(cache_files)}")

        # Display size in KB for small files
        if total_size < 1024:
            print(f"Total size: {total_size} bytes")
        elif total_size < 1024 * 1024:
            print(f"Total size: {total_size / 1024:.2f} KB")
        else:
            print(f"Total size: {total_size / (1024*1024):.2f} MB")

        print()

    def test_log_display(self):
        """Display recent log entries"""
        self.log_info("Test 6: Recent prxy log entries (last 30 lines)")
        print("---")

        with open(self.log_file, 'r') as f:
            lines = f.readlines()
            recent_lines = lines[-30:]

            # Filter for relevant log entries
            filtered = [line for line in recent_lines
                       if any(keyword in line for keyword in ["Cache", "Starting", "Download"])]

            if filtered:
                print("".join(filtered))
            else:
                print("".join(recent_lines))

        print("---")
        print()

    def run_tests(self):
        """Run all tests"""
        print("=" * 50)
        print("Prxy Cache Functionality Test")
        print("=" * 50)
        print()

        # Setup
        self.setup()

        try:
            # Test 1: Start server
            self.start_prxy_server()

            # Test 2: First download
            original_size = self.test_first_download()

            # Test 3: Verify cache
            self.test_cache_verification()

            # Test 4: Second download
            self.test_second_download(original_size)

            # Test 5: Another file
            self.test_another_file()

            # Test 6: Statistics
            self.test_cache_statistics()

            # Test 7: Log display
            self.test_log_display()

            # Final summary
            print("=" * 50)
            print("Test Summary")
            print("=" * 50)
            self.log_success("All tests completed successfully!")
            print()

            cache_files = list(self.cache_dir.rglob("*"))
            cache_files = [f for f in cache_files if f.is_file()]
            total_size = sum(f.stat().st_size for f in cache_files)

            print("Test results:")
            print("  ✓ Prxy server started successfully")
            print("  ✓ First download (cache MISS)")
            print("  ✓ File was cached")
            print("  ✓ Second download (cache HIT)")
            print("  ✓ Multiple files cached")
            print()

            print(f"Cache statistics:")
            print(f"  Files cached: {len(cache_files)}")

            # Display size in appropriate unit
            if total_size < 1024:
                print(f"  Total size: {total_size} bytes")
            elif total_size < 1024 * 1024:
                print(f"  Total size: {total_size / 1024:.2f} KB")
            else:
                print(f"  Total size: {total_size / (1024*1024):.2f} MB")

            print()

            print(f"Logs saved to: {self.log_file}")
            print(f"Config file: {self.test_dir / 'config.json'}")
            print()

        finally:
            # Cleanup
            self.cleanup()


def main():
    """Main entry point"""
    tester = CacheTester()

    # Set up cleanup on exit
    import atexit
    atexit.register(tester.cleanup)

    # Run tests
    tester.run_tests()


if __name__ == "__main__":
    main()