#!/usr/bin/env python3
"""
Coverage HTTP Server Wrapper (Multiprocess Edition)

A wrapper that enables coverage collection across all processes (including
Gunicorn workers) and exposes combined coverage data via HTTP.

Compatible with openshift-eng/art-tools coverage protocol.

Based on recommendations from coverage.py documentation and Gemini research:
- Uses sitecustomize.py for global process instrumentation
- Uses Gunicorn worker_exit hooks for reliable data saving
- Stores data in /dev/shm (writable even with readOnlyRootFilesystem)
- Combines all process coverage files on /coverage request

Multiple containers in a pod may each start a coverage server. The server
tries ports starting at COVERAGE_PORT (default 53700) and increments up to
MAX_RETRIES times until it finds a free port.

Clients can identify a coverage server by checking the response headers:
    X-Art-Coverage-Server:         1
    X-Art-Coverage-Pid:            <pid>
    X-Art-Coverage-Binary:         <binary-name>
    X-Art-Coverage-Source-Commit:  <commit>  (if SOURCE_GIT_COMMIT is set)
    X-Art-Coverage-Source-Url:     <url>     (if SOURCE_GIT_URL is set)

Usage:
    python coverage_server.py -m gunicorn -c gunicorn_coverage.py app:app
    python coverage_server.py app.py

Environment Variables:
    COVERAGE_PORT - Starting port for coverage HTTP server (default: 53700)
    COVERAGE_PROCESS_START - Path to .coveragerc (set automatically)
    COVERAGE_DATA_DIR - Directory for coverage files (default: /dev/shm)
"""

import os
import sys
import runpy
import json
import base64
import glob
import urllib.parse
from datetime import datetime, timezone
from threading import Thread, Event
from http.server import HTTPServer, BaseHTTPRequestHandler
from socketserver import ThreadingMixIn
import coverage

# Configuration
DEFAULT_PORT = 53700
MAX_RETRIES = 50
COVERAGE_PORT = int(os.getenv("COVERAGE_PORT", str(DEFAULT_PORT)))
# Default to /dev/shm (Linux containers) or /tmp/coverage-test (macOS)
_DEFAULT_DIR = "/dev/shm" if os.path.exists("/dev/shm") else "/tmp/coverage-test"
COVERAGE_DATA_DIR = os.getenv("COVERAGE_DATA_DIR", _DEFAULT_DIR)
PRINT_PREFIX = "[coverage-wrapper]"

# Path to the .coveragerc file (relative to this script)
SCRIPT_DIR = os.path.dirname(os.path.abspath(__file__))
DEFAULT_COVERAGERC = os.path.join(SCRIPT_DIR, ".coveragerc")


def _build_identity_headers():
    """Build identity headers once at import time."""
    headers = {
        "X-Art-Coverage-Server": "1",
        "X-Art-Coverage-Pid": str(os.getpid()),
        "X-Art-Coverage-Binary": os.path.basename(sys.executable),
    }
    for header, env_var in [
        ("X-Art-Coverage-Source-Commit", "SOURCE_GIT_COMMIT"),
        ("X-Art-Coverage-Source-Url", "SOURCE_GIT_URL"),
    ]:
        val = os.environ.get(env_var, "")
        if val:
            headers[header] = val

    software_group = os.environ.get("SOFTWARE_GROUP", "") or os.environ.get("__doozer_group", "")
    if software_group:
        headers["X-Art-Coverage-Software-Group"] = software_group
    software_key = os.environ.get("SOFTWARE_KEY", "") or os.environ.get("__doozer_key", "")
    if software_key:
        headers["X-Art-Coverage-Software-Key"] = software_key

    return headers


_IDENTITY_HEADERS = _build_identity_headers()


class CoverageHandler(BaseHTTPRequestHandler):
    """HTTP handler for coverage endpoints."""

    def log_message(self, format, *args):
        """Suppress default request logging"""
        pass

    def end_headers(self):
        for header, value in _IDENTITY_HEADERS.items():
            self.send_header(header, value)
        super().end_headers()

    def do_HEAD(self):
        """Respond to HEAD requests with identity headers."""
        self.send_response(200)
        self.end_headers()

    def do_GET(self):
        parsed = urllib.parse.urlparse(self.path)
        path = parsed.path
        query = urllib.parse.parse_qs(parsed.query)
        label = query.get("name", ["session"])[0]

        if path == "/coverage":
            self._handle_coverage(label)
        elif path == "/health":
            self._handle_health()
        elif path == "/coverage/save":
            self._handle_save()
        elif path == "/coverage/reset":
            self._handle_reset()
        elif path == "/coverage/files":
            self._handle_list_files()
        else:
            self.send_response(404)
            self.send_header("Content-Type", "text/plain")
            self.end_headers()
            self.wfile.write(b"Not found")

    def _handle_coverage(self, label):
        """Combine all coverage files and return as JSON."""
        print(f"{PRINT_PREFIX} Coverage dump requested (label={label})", flush=True)

        try:
            # Find all coverage files in the data directory
            pattern = os.path.join(COVERAGE_DATA_DIR, ".coverage*")
            coverage_files = sorted(glob.glob(pattern))

            print(f"{PRINT_PREFIX} Found {len(coverage_files)} coverage file(s)", flush=True)

            if not coverage_files:
                # Return empty coverage data
                payload = {
                    "label": label,
                    "timestamp": datetime.now(timezone.utc).isoformat(),
                    "coverage_data": "",
                    "files_combined": 0,
                    "message": "No coverage files found"
                }
            else:
                # Create a combined coverage data object (in-memory, no writes)
                combined = coverage.CoverageData(no_disk=True)

                for cov_file in coverage_files:
                    try:
                        # Read each coverage file from disk (no_disk=False required for reading!)
                        file_data = coverage.CoverageData(basename=cov_file)
                        file_data.read()
                        combined.update(file_data)
                        measured = list(file_data.measured_files())
                        print(f"{PRINT_PREFIX} Combined: {os.path.basename(cov_file)} ({len(measured)} files)", flush=True)
                    except Exception as e:
                        print(f"{PRINT_PREFIX} Error reading {cov_file}: {e}", flush=True)

                # Serialize to binary and encode
                json_bytes = combined.dumps()
                json_b64 = base64.b64encode(json_bytes).decode('ascii')

                payload = {
                    "label": label,
                    "timestamp": datetime.now(timezone.utc).isoformat(),
                    "coverage_data": json_b64,
                    "files_combined": len(coverage_files),
                }

            body = json.dumps(payload).encode()
            self.send_response(200)
            self.send_header("Content-Type", "application/json")
            self.send_header("Content-Length", str(len(body)))
            self.end_headers()
            self.wfile.write(body)

        except Exception as e:
            print(f"{PRINT_PREFIX} Error collecting coverage: {e}", flush=True)
            self.send_response(500)
            self.send_header("Content-Type", "text/plain")
            self.end_headers()
            self.wfile.write(f"Error: {e}".encode())

    def _handle_health(self):
        """Return health status."""
        print(f"{PRINT_PREFIX} Health check requested", flush=True)

        # Count coverage files
        pattern = os.path.join(COVERAGE_DATA_DIR, ".coverage*")
        file_count = len(glob.glob(pattern))

        payload = {
            "status": "ok",
            "coverage_enabled": True,
            "data_dir": COVERAGE_DATA_DIR,
            "coverage_files": file_count,
        }
        body = json.dumps(payload).encode()

        self.send_response(200)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)

    def _handle_save(self):
        """Trigger coverage save by sending SIGHUP to PID 1 (Gunicorn master).

        This restarts Gunicorn workers, which triggers the worker_exit hook
        in gunicorn_coverage.py, saving each worker's coverage data to /dev/shm.
        """
        import signal
        import time

        print(f"{PRINT_PREFIX} Coverage save triggered via /coverage/save", flush=True)

        try:
            os.kill(1, signal.SIGHUP)
            time.sleep(3)

            pattern = os.path.join(COVERAGE_DATA_DIR, ".coverage*")
            file_count = len(glob.glob(pattern))
            print(f"{PRINT_PREFIX} After save: {file_count} coverage file(s) in {COVERAGE_DATA_DIR}", flush=True)

            payload = {
                "status": "ok",
                "message": "Coverage save triggered (SIGHUP sent to Gunicorn master)",
                "coverage_files": file_count,
            }
            self.send_response(200)
        except Exception as e:
            print(f"{PRINT_PREFIX} Error triggering save: {e}", flush=True)
            payload = {"status": "error", "message": str(e), "coverage_files": 0}
            self.send_response(500)

        body = json.dumps(payload).encode()
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)

    def _handle_reset(self):
        """Delete all coverage files."""
        print(f"{PRINT_PREFIX} Coverage reset requested", flush=True)

        pattern = os.path.join(COVERAGE_DATA_DIR, ".coverage*")
        files = glob.glob(pattern)
        deleted = 0

        for f in files:
            try:
                os.remove(f)
                deleted += 1
            except Exception as e:
                print(f"{PRINT_PREFIX} Error deleting {f}: {e}", flush=True)

        self.send_response(200)
        self.send_header("Content-Type", "text/plain")
        self.end_headers()
        self.wfile.write(f"Deleted {deleted} coverage files".encode())

    def _handle_list_files(self):
        """List all coverage files (for debugging)."""
        pattern = os.path.join(COVERAGE_DATA_DIR, ".coverage*")
        files = sorted(glob.glob(pattern))

        file_info = []
        for f in files:
            try:
                stat = os.stat(f)
                file_info.append({
                    "name": os.path.basename(f),
                    "size": stat.st_size,
                    "mtime": datetime.fromtimestamp(stat.st_mtime, timezone.utc).isoformat()
                })
            except Exception:
                file_info.append({"name": os.path.basename(f), "error": "stat failed"})

        payload = {
            "data_dir": COVERAGE_DATA_DIR,
            "files": file_info
        }
        body = json.dumps(payload, indent=2).encode()

        self.send_response(200)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)


def run_server(ready_event=None):
    """Run the coverage HTTP server.

    Tries successive ports starting from COVERAGE_PORT until one is available
    or MAX_RETRIES attempts are exhausted. If a ready_event is provided, it is
    set once the server is listening so callers can wait for readiness.
    """
    class ThreadedHTTPServer(ThreadingMixIn, HTTPServer):
        daemon_threads = True
        allow_reuse_address = True

    for attempt in range(MAX_RETRIES):
        port = COVERAGE_PORT + attempt
        try:
            server = ThreadedHTTPServer(("0.0.0.0", port), CoverageHandler)
            print(f"{PRINT_PREFIX} HTTP server listening on port {port} (pid {os.getpid()})", flush=True)
            print(f"{PRINT_PREFIX} Endpoints: GET :{port}/coverage, GET :{port}/health, HEAD :{port}/*", flush=True)
            if ready_event:
                ready_event.set()
            server.serve_forever()
            return
        except OSError as e:
            print(f"{PRINT_PREFIX} Port {port} unavailable: {e}; trying next", flush=True)

    print(f"{PRINT_PREFIX} ERROR: Could not bind any port in range {COVERAGE_PORT}–{COVERAGE_PORT + MAX_RETRIES - 1}", flush=True)


def setup_environment():
    """Set up environment variables for coverage collection."""
    # Set COVERAGE_PROCESS_START if not already set
    if not os.environ.get('COVERAGE_PROCESS_START'):
        if os.path.exists(DEFAULT_COVERAGERC):
            os.environ['COVERAGE_PROCESS_START'] = DEFAULT_COVERAGERC
            print(f"{PRINT_PREFIX} Set COVERAGE_PROCESS_START={DEFAULT_COVERAGERC}", flush=True)
        else:
            print(f"{PRINT_PREFIX} WARNING: No .coveragerc found at {DEFAULT_COVERAGERC}", flush=True)
    else:
        print(f"{PRINT_PREFIX} Using COVERAGE_PROCESS_START={os.environ['COVERAGE_PROCESS_START']}", flush=True)

    # Add the sitecustomize.py directory to PYTHONPATH
    sitecustomize_dir = SCRIPT_DIR
    pythonpath = os.environ.get('PYTHONPATH', '')
    if sitecustomize_dir not in pythonpath:
        os.environ['PYTHONPATH'] = f"{sitecustomize_dir}:{pythonpath}" if pythonpath else sitecustomize_dir
        print(f"{PRINT_PREFIX} Added {sitecustomize_dir} to PYTHONPATH", flush=True)

    # Ensure coverage data directory exists and is writable
    if not os.path.exists(COVERAGE_DATA_DIR):
        print(f"{PRINT_PREFIX} WARNING: Coverage data dir {COVERAGE_DATA_DIR} does not exist", flush=True)
    elif not os.access(COVERAGE_DATA_DIR, os.W_OK):
        print(f"{PRINT_PREFIX} WARNING: Coverage data dir {COVERAGE_DATA_DIR} is not writable", flush=True)
    else:
        print(f"{PRINT_PREFIX} Coverage data dir: {COVERAGE_DATA_DIR}", flush=True)


def main():
    """Main entry point."""
    if len(sys.argv) < 2:
        print(f"Usage: python {sys.argv[0]} <script.py> [args...]")
        print(f"       python {sys.argv[0]} -m module [args...]")
        print()
        print("This wrapper:")
        print("  1. Sets up coverage.process_startup() via sitecustomize.py")
        print("  2. Starts an HTTP server for collecting coverage data")
        print("  3. Runs your application")
        print()
        print("For Gunicorn, use with the coverage hooks:")
        print(f"  python {sys.argv[0]} -m gunicorn -c gunicorn_coverage.py app:app")
        sys.exit(1)

    # Set up environment for coverage collection
    setup_environment()

    # Start HTTP server in background thread with readiness handshake
    server_ready = Event()
    server_thread = Thread(target=run_server, args=(server_ready,), daemon=True)
    server_thread.start()

    if not server_ready.wait(timeout=10):
        print(f"{PRINT_PREFIX} ERROR: Coverage server failed to start within 10s", flush=True)
        sys.exit(1)

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
        sys.exit(0)
    except Exception as e:
        print(f"{PRINT_PREFIX} Error: {e}", flush=True)
        raise
