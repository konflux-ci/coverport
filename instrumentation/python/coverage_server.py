#!/usr/bin/env python3
"""
Coverage HTTP Server Wrapper

A pure wrapper that runs any Python script with coverage collection and
exposes coverage data via HTTP. Completely application-agnostic.

Usage:
    python coverage_server.py app.py
    python coverage_server.py -m flask run
    python coverage_server.py path/to/script.py

Environment Variables:
    COVERAGE_PORT - Port for coverage HTTP server (default: 9095)
"""

import os
import sys
import runpy
import json
import base64
import urllib.parse
from datetime import datetime, timezone
from threading import Thread
from http.server import HTTPServer, BaseHTTPRequestHandler
from socketserver import ThreadingMixIn
import coverage

# Configuration
COVERAGE_PORT = int(os.getenv("COVERAGE_PORT", "9095"))
PRINT_PREFIX = "[coverage-wrapper]"

# Global coverage instance
cov = None


class CoverageHandler(BaseHTTPRequestHandler):
    """HTTP handler for coverage endpoints."""
    
    def log_message(self, format, *args):
        """Suppress default request logging"""
        pass

    def do_GET(self):
        parsed = urllib.parse.urlparse(self.path)
        path = parsed.path
        query = urllib.parse.parse_qs(parsed.query)
        label = query.get("name", ["session"])[0]

        if path == "/coverage":
            print(f"{PRINT_PREFIX} Coverage dump requested (label={label})", flush=True)
            cov.stop()
            cov.save()

            # Dump in-memory coverage data as base64-encoded JSON
            data = cov.get_data()
            json_bytes = data.dumps()
            json_b64 = base64.b64encode(json_bytes).decode('ascii')
            
            payload = {
                "label": label,
                "timestamp": datetime.now(timezone.utc).isoformat(),
                "coverage_data": json_b64,
            }
            body = json.dumps(payload).encode()

            self.send_response(200)
            self.send_header("Content-Type", "application/json")
            self.send_header("Content-Length", str(len(body)))
            self.end_headers()
            self.wfile.write(body)

            cov.start()  # Resume tracing
            return

        elif path == "/health":
            print(f"{PRINT_PREFIX} Health check requested", flush=True)
            payload = {
                "status": "ok",
                "coverage_enabled": True,
            }
            body = json.dumps(payload).encode()
            
            self.send_response(200)
            self.send_header("Content-Type", "application/json")
            self.send_header("Content-Length", str(len(body)))
            self.end_headers()
            self.wfile.write(body)
            return

        elif path == "/coverage/reset":
            print(f"{PRINT_PREFIX} Coverage reset requested", flush=True)
            cov.stop()
            cov.erase()
            cov.start()
            
            self.send_response(200)
            self.send_header("Content-Type", "text/plain")
            self.end_headers()
            self.wfile.write(b"Coverage reset")
            return

        else:
            self.send_response(404)
            self.send_header("Content-Type", "text/plain")
            self.end_headers()
            self.wfile.write(b"Not found")


def run_server():
    """Run the coverage HTTP server."""
    class ThreadedHTTPServer(ThreadingMixIn, HTTPServer):
        daemon_threads = True

    server = ThreadedHTTPServer(("0.0.0.0", COVERAGE_PORT), CoverageHandler)
    print(f"{PRINT_PREFIX} HTTP server listening on port {COVERAGE_PORT}", flush=True)
    server.serve_forever()


def main():
    """Main entry point."""
    global cov
    
    if len(sys.argv) < 2:
        print(f"Usage: python {sys.argv[0]} <script.py> [args...]")
        print(f"       python {sys.argv[0]} -m module [args...]")
        sys.exit(1)

    # Start coverage collection (in-memory, no filesystem writes)
    # Note: With read-only filesystem + Gunicorn, coverage is limited to:
    # - Startup code (imports, config loading)
    # - Code executed in master process before forking
    # Request handlers in worker processes won't be captured without /tmp access
    cov = coverage.Coverage(
        data_file=None,  # In-memory only (no filesystem writes)
        auto_data=False,
        omit=[
            '*/coverage_server.py',  # Exclude this wrapper
            '*/site-packages/*',      # Exclude installed packages
        ]
    )
    cov.start()
    print(f"{PRINT_PREFIX} Coverage collection started (in-memory, limited to master process)", flush=True)

    # Start HTTP server in background thread
    server_thread = Thread(target=run_server, daemon=True)
    server_thread.start()

    # Prepare to run the target script
    script_args = sys.argv[1:]
    
    # Handle -m module syntax
    if script_args[0] == '-m' and len(script_args) > 1:
        module_name = script_args[1]
        sys.argv = [module_name] + script_args[2:]
        print(f"{PRINT_PREFIX} Running module: {module_name}", flush=True)
        runpy.run_module(module_name, run_name="__main__", alter_sys=True)
    else:
        # Run as script
        script_path = script_args[0]
        sys.argv = script_args
        print(f"{PRINT_PREFIX} Running script: {script_path}", flush=True)
        
        # Add script directory to path
        script_dir = os.path.dirname(os.path.abspath(script_path))
        if script_dir not in sys.path:
            sys.path.insert(0, script_dir)
        
        # Run the script with __name__ == '__main__'
        runpy.run_path(script_path, run_name="__main__")


if __name__ == '__main__':
    try:
        main()
    except KeyboardInterrupt:
        print(f"\n{PRINT_PREFIX} Shutting down...", flush=True)
        if cov:
            cov.stop()
        sys.exit(0)
    except Exception as e:
        print(f"{PRINT_PREFIX} Error: {e}", flush=True)
        if cov:
            cov.stop()
        raise

