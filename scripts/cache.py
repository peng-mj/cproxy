#!/usr/bin/env python3
"""
Cache management script for scproxy
"""

import json
import os
import sys
import subprocess
from pathlib import Path


class CacheManager:
    """Manages scproxy cache configuration and operations"""

    def __init__(self):
        self.config_file = Path.home() / ".scproxy" / "config.json"

    def load_config(self):
        """Load configuration from file"""
        if not self.config_file.exists():
            print(f"Configuration file not found: {self.config_file}")
            return None

        with open(self.config_file, 'r') as f:
            return json.load(f)

    def save_config(self, config):
        """Save configuration to file"""
        with open(self.config_file, 'w') as f:
            json.dump(config, f, indent=2)

    def enable(self):
        """Enable cache mode"""
        print("Enabling cache mode...")

        config = self.load_config()
        if config is None:
            return

        if 'cache' in config:
            config['cache']['enabled'] = True
            print('Cache enabled')
        else:
            print('No cache section in config file')
            return

        self.save_config(config)
        print("")
        print("Configuration updated, cache is now enabled")
        print("Start scproxy to use cache functionality")

    def disable(self):
        """Disable cache mode"""
        print("Disabling cache mode...")

        config = self.load_config()
        if config is None:
            return

        if 'cache' in config:
            config['cache']['enabled'] = False
            print('Cache disabled')
        else:
            print('No cache section in config file')
            return

        self.save_config(config)
        print("")
        print("Configuration updated, cache is now disabled")
        print("Restart scproxy to stop using cache")

    def status(self):
        """Display current cache status"""
        print("Current cache status:")
        print("")

        config = self.load_config()
        if config is None:
            return

        if 'cache' not in config:
            print('No cache section in config file')
            return

        cache_config = config['cache']
        status = 'Enabled' if cache_config['enabled'] else 'Disabled'
        print(f'Cache status: {status}')
        print(f'Cache directory: {cache_config["directory"]}')
        print(f'Maximum size: {cache_config["maxSizeMB"]} MB')
        print(f'Minimum file size: {cache_config["minSizeKB"]} KB')
        print(f'Maximum file size: {cache_config["maxSizeKB"]} KB')
        print(f'Cache authentication: {"Enabled" if cache_config["cacheAuth"] else "Disabled"}')
        print(f'Excluded extensions: "{", ".join(cache_config["excludeExtensions"])}"')
        print('')
        print('Cache statistics:')

        cache_dir = cache_config['directory']
        if os.path.exists(cache_dir):
            total_size = 0
            file_count = 0

            for root, dirs, files in os.walk(cache_dir):
                for file in files:
                    file_path = os.path.join(root, file)
                    if os.path.isfile(file_path):
                        file_count += 1
                        total_size += os.path.getsize(file_path)

            print(f'Cached files: {file_count}')
            print(f'Total size: {total_size / (1024*1024):.2f} MB')
        else:
            print('Cache directory does not exist (no files cached yet)')

    def clear(self):
        """Clear cache data"""
        print("Clearing cache data...")

        # Check if scproxy binary exists in parent directory
        prxy_bin = Path(__file__).parent.parent / "scproxy"
        if not prxy_bin.exists():
            print("scproxy binary not found, please build first")
            print("   Run: make build")
            return

        subprocess.run([str(prxy_bin), "--clear-cache"])

    def show_help(self):
        """Display help message"""
        print("scproxy Cache Management Tool")
        print("")
        print("Usage: python cache.py <command>")
        print("")
        print("Commands:")
        print("  enable   - Enable cache mode")
        print("  disable  - Disable cache mode")
        print("  status   - View cache status")
        print("  clear    - Clear cache data")
        print("  help     - Show this help message")
        print("")
        print("Examples:")
        print("  python cache.py enable   # Enable cache")
        print("  python cache.py disable  # Disable cache")
        print("  python cache.py status   # View status")
        print("  python cache.py clear    # Clear cache")
        print("")
        print(f"Configuration file location: {self.config_file}")


def main():
    """Main entry point"""
    manager = CacheManager()

    if len(sys.argv) < 2:
        manager.show_help()
        return

    command = sys.argv[1].lower()

    commands = {
        'enable': manager.enable,
        'disable': manager.disable,
        'status': manager.status,
        'clear': manager.clear,
        'help': manager.show_help,
    }

    if command in commands:
        commands[command]()
    else:
        print(f"Unknown command: {command}")
        manager.show_help()
        sys.exit(1)


if __name__ == "__main__":
    main()