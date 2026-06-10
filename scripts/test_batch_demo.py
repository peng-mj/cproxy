#!/usr/bin/env python3
"""
Batch proxy functionality demo script
"""

import os
import sys
import subprocess
import time
import signal
from pathlib import Path
import urllib.request
import urllib.error


def download_url(url, timeout=5):
    """Download content from URL and return it as string"""
    try:
        with urllib.request.urlopen(url, timeout=timeout) as response:
            return response.read().decode('utf-8')
    except urllib.error.URLError as e:
        return None


def main():
    """Main demo function"""
    print("=== scproxy Batch Proxy Functionality Demo ===")
    print()

    script_dir = Path(__file__).parent.parent

    # Use PRXY_BIN_PATH from environment if set (for automated testing)
    prxy_bin_path = os.environ.get('PRXY_BIN_PATH')
    if prxy_bin_path:
        prxy_bin = Path(prxy_bin_path)
    else:
        prxy_bin = script_dir / "scproxy"

    test_config = script_dir / "test-config.json"

    # Check if config exists
    if not test_config.exists():
        print(f"Error: Test configuration file not found: {test_config}")
        print("Please create test-config.json with batch routes configuration")
        sys.exit(1)

    # Check if scproxy binary exists
    if not prxy_bin.exists():
        print(f"Error: scproxy binary not found: {prxy_bin}")
        print("Please build scproxy first: make build")
        sys.exit(1)

    print("Configuration file has 2 routes:")
    print("  - Port 18081 -> https://httpbin.org")
    print("  - Port 18082 -> https://www.example.com")
    print()

    print("CLI parameter adds 1 additional route:")
    print("  - Port 18083 -> https://www.baidu.com")
    print()

    print("Starting service...")

    # Start scproxy with test configuration
    cmd = [
        str(prxy_bin),
        "--config", str(test_config),
        "--target", "https://www.baidu.com",
        "--port", "18083"
    ]

    process = subprocess.Popen(cmd)
    prxy_pid = process.pid

    # Wait for server to start
    time.sleep(3)

    try:
        print()
        print("=== Testing port 18081 (httpbin.org) ===")

        # Test httpbin.org - it returns JSON
        content = download_url("http://localhost:18081/ip")
        if content:
            # Print first few lines
            lines = content.split('\n')[:3]
            for line in lines:
                print(line)
        else:
            print("Failed to connect to httpbin.org")

        print()

        print("=== Testing port 18082 (example.com) ===")

        # Test example.com - extract title
        content = download_url("http://localhost:18082/")
        if content and "<title>" in content:
            # Extract title
            start = content.find("<title>") + 7
            end = content.find("</title>")
            title = content[start:end]
            print(f"<title>{title}</title>")
        else:
            print("Failed to connect to example.com")

        print()

        print("=== Testing port 18083 (baidu.com - from CLI) ===")

        # Test baidu.com - extract title
        content = download_url("http://localhost:18083/")
        if content and "<title>" in content:
            # Extract title
            start = content.find("<title>") + 7
            end = content.find("</title>")
            title = content[start:end]
            print(f"<title>{title}</title>")
        else:
            print("Failed to connect to baidu.com")

        print()

    finally:
        print("=== Cleanup ===")
        try:
            os.kill(prxy_pid, signal.SIGTERM)
            process.wait(timeout=5)
        except (ProcessLookupError, ChildProcessError):
            pass

        print()
        print("Demo completed!")


if __name__ == "__main__":
    main()