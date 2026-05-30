#!/usr/bin/env python3
"""
Test script for GitHub releases proxy functionality
This script tests the optimized GitHub releases download feature
"""

import os
import sys
import subprocess
import time
import signal
from pathlib import Path
import urllib.request
import urllib.error


class Colors:
    """ANSI color codes for terminal output"""
    GREEN = '\033[0;32m'
    RED = '\033[0;31m'
    YELLOW = '\033[1;33m'
    NC = '\033[0m'  # No Color


class GitHubReleasesTester:
    """Tests GitHub releases proxy functionality"""

    def __init__(self):
        self.script_dir = Path(__file__).parent.parent

        # Use PRXY_BIN_PATH from environment if set (for automated testing)
        prxy_bin_path = os.environ.get('PRXY_BIN_PATH')
        if prxy_bin_path:
            self.prxy_bin = Path(prxy_bin_path)
        else:
            self.prxy_bin = self.script_dir / "prxy"

        self.target_url = "https://github.com"
        self.proxy_url = ""  # Empty for direct connection
        self.port = "8888"
        self.test_file = "/Madh93/prxy/releases/download/v0.1.0/checksums.txt"
        self.test_url = f"http://localhost:{self.port}{self.test_file}"
        self.prxy_pid = None
        self.log_file = self.script_dir / "prxy_test.log"

    def cleanup(self):
        """Clean up test environment"""
        print(f"{Colors.YELLOW}Cleaning up...{Colors.NC}")
        if self.prxy_pid:
            try:
                os.kill(self.prxy_pid, signal.SIGTERM)
                os.waitpid(self.prxy_pid, 0)
            except (ProcessLookupError, ChildProcessError):
                pass

        # Remove test output files
        for f in ["test_output.txt", "test_direct.txt"]:
            file_path = self.script_dir / f
            if file_path.exists():
                file_path.unlink()

        print("Cleanup complete")

    def download_file(self, url, output_file, headers_only=False):
        """Download file from URL"""
        try:
            if headers_only:
                # Download with headers
                import http.client
                conn = http.client.HTTPConnection("localhost", int(self.port))
                conn.request("GET", self.test_file)
                response = conn.getresponse()
                headers = dict(response.getheaders())
                conn.close()
                return headers, response.read()
            else:
                urllib.request.urlretrieve(url, output_file)
                return True, os.path.getsize(output_file)
        except urllib.error.URLError as e:
            return False, str(e)

    def start_prxy_server(self):
        """Start prxy server"""
        print(f"{Colors.GREEN}1. Starting prxy server...{Colors.NC}")

        cmd = [str(self.prxy_bin)]

        # Add target
        cmd.extend(["--target", self.target_url])

        # Add proxy if specified
        if self.proxy_url:
            cmd.extend(["--proxy", self.proxy_url])

        # Add port and cache
        cmd.extend(["--port", self.port, "--cache"])

        # Start server
        with open(self.log_file, 'w') as log_f:
            self.prxy_pid = subprocess.Popen(
                cmd,
                stdout=log_f,
                stderr=log_f,
                cwd=self.script_dir
            ).pid

        # Wait for server to start
        print("Waiting for server to start...")
        time.sleep(2)

        # Check if server is running
        try:
            os.kill(self.prxy_pid, 0)  # Check if process exists
            print(f"{Colors.GREEN}Server started successfully (PID: {self.prxy_pid}){Colors.NC}")
            print()
            return True
        except ProcessLookupError:
            print(f"{Colors.RED}Error: prxy server failed to start. Check {self.log_file} for details.{Colors.NC}")
            with open(self.log_file, 'r') as f:
                print(f.read())
            return False

    def test_first_download(self):
        """Test first download (should be cache MISS)"""
        print(f"{Colors.GREEN}2. Testing first download (should be cache MISS)...{Colors.NC}")

        output_file = self.script_dir / "test_output.txt"
        success, result = self.download_file(self.test_url, output_file)

        if success:
            file_size = result
            print(f"Response code: 200")
            print(f"Downloaded file size: {file_size} bytes")

            if file_size > 0:
                print(f"{Colors.GREEN}✓ File downloaded successfully{Colors.NC}")
            else:
                print(f"{Colors.RED}✗ Downloaded file is empty{Colors.NC}")
                sys.exit(1)
        else:
            print(f"{Colors.RED}✗ Failed to download file{Colors.NC}")
            print(f"Error: {result}")
            sys.exit(1)

        print()
        return True

    def test_second_download(self):
        """Test second download (should be cache HIT)"""
        print(f"{Colors.GREEN}3. Testing second download (should be cache HIT)...{Colors.NC}")

        # Try to get headers
        try:
            import http.client
            conn = http.client.HTTPConnection("localhost", int(self.port))
            conn.request("GET", self.test_file)
            response = conn.getresponse()

            headers = dict(response.getheaders())
            cache_status = headers.get("X-Cache", "UNKNOWN")

            print(f"Response headers: {cache_status}")

            if "HIT" in cache_status:
                print(f"{Colors.GREEN}✓ Cache HIT confirmed{Colors.NC}")
            else:
                print(f"{Colors.YELLOW}⚠ Cache HIT not detected (might need longer delay){Colors.NC}")

            conn.close()
        except Exception as e:
            print(f"Error checking headers: {e}")

        print()

    def test_file_integrity(self):
        """Verify file integrity"""
        print(f"{Colors.GREEN}4. Verifying file integrity...{Colors.NC}")

        direct_url = "https://github.com/Madh93/prxy/releases/download/v0.1.0/checksums.txt"
        proxy_file = self.script_dir / "test_output.txt"
        direct_file = self.script_dir / "test_direct.txt"

        # Download direct file
        try:
            urllib.request.urlretrieve(direct_url, direct_file)
        except urllib.error.URLError as e:
            print(f"{Colors.RED}✗ Failed to download direct file: {e}{Colors.NC}")
            print()

        # Compare files
        if proxy_file.exists() and direct_file.exists():
            with open(proxy_file, 'rb') as f1, open(direct_file, 'rb') as f2:
                content1 = f1.read()
                content2 = f2.read()

                if content1 == content2:
                    print(f"{Colors.GREEN}✓ Files are identical{Colors.NC}")
                else:
                    print(f"{Colors.RED}✗ Files differ{Colors.NC}")
                    print(f"Proxy file size: {len(content1)} bytes")
                    print(f"Direct file size: {len(content2)} bytes")
        else:
            print(f"{Colors.RED}✗ Files not available for comparison{Colors.NC}")

        print()

    def test_cache_directory(self):
        """Check cache directory"""
        print(f"{Colors.GREEN}5. Checking cache directory...{Colors.NC}")

        cache_dir = Path.home() / ".prxy" / "cache" / "data"

        if cache_dir.exists():
            cached_files = list(cache_dir.rglob("checksums.txt"))
            if cached_files:
                print(f"{Colors.GREEN}✓ Found cached files:{Colors.NC}")
                for f in cached_files:
                    print(f"  {f.relative_to(cache_dir.parent)}")
            else:
                print(f"{Colors.YELLOW}⚠ No cached files found in {cache_dir}{Colors.NC}")
        else:
            print(f"{Colors.YELLOW}⚠ Cache directory not found: {cache_dir}{Colors.NC}")

        print()

    def test_github_handler_usage(self):
        """Check for GitHub handler usage in logs"""
        print(f"{Colors.GREEN}6. Checking for GitHub handler usage in logs...{Colors.NC}")

        if self.log_file.exists():
            with open(self.log_file, 'r') as f:
                log_content = f.read()

                if "GitHub releases file detected" in log_content:
                    print(f"{Colors.GREEN}✓ GitHub releases handler was used{Colors.NC}")
                    # Print first 5 lines mentioning GitHub
                    for line in log_content.split('\n'):
                        if "GitHub releases" in line:
                            print(f"  {line}")
                            # Only show first 5 matches
                            break
                else:
                    print(f"{Colors.YELLOW}⚠ GitHub releases handler usage not detected in logs{Colors.NC}")
        else:
            print(f"{Colors.YELLOW}⚠ Log file not found{Colors.NC}")

        print()

    def run_tests(self):
        """Run all tests"""
        print("=== GitHub Releases Proxy Test ===")
        print()

        try:
            # Test 1: Start server
            if not self.start_prxy_server():
                sys.exit(1)

            # Test 2: First download
            self.test_first_download()

            # Test 3: Second download
            self.test_second_download()

            # Test 4: File integrity
            self.test_file_integrity()

            # Test 5: Cache directory
            self.test_cache_directory()

            # Test 6: GitHub handler usage
            self.test_github_handler_usage()

            # Final message
            print(f"{Colors.GREEN}=== Test completed ==={Colors.NC}")
            print(f"Check {self.log_file} for detailed logs")

        finally:
            # Cleanup
            self.cleanup()


def main():
    """Main entry point"""
    # Use PRXY_BIN_PATH from environment if set (for automated testing)
    prxy_bin_path = os.environ.get('PRXY_BIN_PATH')
    if prxy_bin_path:
        prxy_bin = Path(prxy_bin_path)
    else:
        script_dir = Path(__file__).parent.parent
        prxy_bin = script_dir / "prxy"

    if not prxy_bin.exists():
        print(f"{Colors.RED}Error: prxy binary not found at {prxy_bin}.{Colors.NC}")
        if not os.environ.get('PRXY_BIN_PATH'):
            print(f"{Colors.RED}Please run 'make build' first.{Colors.NC}")
        sys.exit(1)

    tester = GitHubReleasesTester()

    # Set up cleanup on exit
    import atexit
    atexit.register(tester.cleanup)

    # Run tests
    tester.run_tests()


if __name__ == "__main__":
    main()