"""
Python Coverage HTTP Client
Collect coverage data from Python applications running in Kubernetes pods.
"""

import os
import json
import base64
import time
import socket
import select
import threading
from pathlib import Path
from typing import Optional
import requests
from coverage import Coverage


class CoverageClient:
    """Client for collecting coverage data from remote Python applications."""
    
    def __init__(self, namespace: str = "default", output_dir: str = "./coverage-output"):
        """
        Initialize coverage client.
        
        Args:
            namespace: Kubernetes namespace
            output_dir: Directory to store coverage data
        """
        self.namespace = namespace
        self.output_dir = Path(output_dir)
        self.output_dir.mkdir(parents=True, exist_ok=True)
    
    @staticmethod
    def get_pod_name(namespace: str, label_selector: str = "app=coverage-demo") -> str:
        """
        Get pod name dynamically using Kubernetes label selector.
        
        Args:
            namespace: Kubernetes namespace
            label_selector: Label selector to find the pod (e.g., "app=my-app")
            
        Returns:
            Name of the first running pod matching the selector
            
        Raises:
            RuntimeError: If no running pod is found or kubernetes package not available
            
        Example:
            >>> pod_name = CoverageClient.get_pod_name("default", "app=my-app")
            >>> client.collect_coverage_from_pod(pod_name, "test1")
        """
        try:
            from kubernetes import client, config
        except ImportError:
            raise RuntimeError(
                "kubernetes package required for pod discovery. "
                "Install with: pip install kubernetes"
            )
        
        try:
            # Load kubeconfig
            try:
                config.load_incluster_config()
            except:
                config.load_kube_config()
            
            v1 = client.CoreV1Api()
            
            # List pods with label selector
            pods = v1.list_namespaced_pod(namespace=namespace, label_selector=label_selector)
            
            # Find first running pod
            for pod in pods.items:
                if pod.status.phase == "Running":
                    return pod.metadata.name
            
            raise RuntimeError(
                f"No running pod found with label '{label_selector}' in namespace '{namespace}'"
            )
            
        except Exception as e:
            if "No running pod found" in str(e):
                raise
            raise RuntimeError(f"Failed to get pod name: {e}")
        
    def reset_coverage(
        self,
        pod_name: str,
        coverage_port: int = 9095,
        timeout: int = 30,
        use_kubectl: bool = True
    ) -> bool:
        """
        Reset coverage counters in the pod.
        
        Args:
            pod_name: Name of the pod
            coverage_port: Port where coverage server is running
            timeout: Timeout in seconds
            use_kubectl: If True, use kubectl binary; if False, use native Python
            
        Returns:
            True if reset was successful, False otherwise
        """
        print(f"[coverage-client] Resetting coverage counters in pod {pod_name}")
        
        if use_kubectl:
            return self._reset_with_kubectl(pod_name, coverage_port, timeout)
        else:
            return self._reset_with_native_portforward(pod_name, coverage_port, timeout)
    
    def collect_coverage_from_pod(
        self,
        pod_name: str,
        test_name: str,
        coverage_port: int = 9095,
        timeout: int = 30,
        use_kubectl: bool = True
    ) -> Optional[str]:
        """
        Collect coverage data from a Kubernetes pod via port-forwarding.
        
        Args:
            pod_name: Name of the pod
            test_name: Name of the test (used for labeling)
            coverage_port: Port where coverage server is running
            timeout: Timeout in seconds
            use_kubectl: If True, use kubectl binary (default, reliable); 
                        if False, use native Python (no kubectl binary required)
            
        Returns:
            Path to the saved coverage file, or None if failed
        """
        print(f"[coverage-client] Collecting coverage from pod {pod_name} (test: {test_name})")
        
        if use_kubectl:
            return self._collect_with_kubectl(pod_name, test_name, coverage_port, timeout)
        else:
            return self._collect_with_native_portforward(pod_name, test_name, coverage_port, timeout)
    
    def _collect_with_kubectl(
        self,
        pod_name: str,
        test_name: str,
        coverage_port: int,
        timeout: int
    ) -> Optional[str]:
        """Collect coverage using kubectl binary (legacy method)."""
        import subprocess
        
        local_port = self._find_free_port()
        print(f"[coverage-client] Using kubectl port-forward")
        
        # Start port-forwarding
        pf_process = subprocess.Popen(
            [
                "kubectl", "port-forward",
                "-n", self.namespace,
                pod_name,
                f"{local_port}:{coverage_port}"
            ],
            stdout=subprocess.PIPE,
            stderr=subprocess.PIPE
        )
        
        try:
            # Wait for port-forward to be ready
            time.sleep(2)
            
            # Check if port-forward is working
            health_url = f"http://localhost:{local_port}/health"
            for _ in range(5):
                try:
                    response = requests.get(health_url, timeout=2)
                    if response.status_code == 200:
                        print(f"[coverage-client] Port-forward ready on port {local_port}")
                        break
                except requests.exceptions.RequestException:
                    time.sleep(1)
            else:
                print("[coverage-client] Failed to establish port-forward connection")
                return None
            
            return self._fetch_coverage_data(local_port, test_name, timeout)
            
        finally:
            pf_process.terminate()
            pf_process.wait(timeout=5)
    
    def _collect_with_native_portforward(
        self,
        pod_name: str,
        test_name: str,
        coverage_port: int,
        timeout: int
    ) -> Optional[str]:
        """Collect coverage using native Python Kubernetes client."""
        try:
            from kubernetes import client, config
            from kubernetes.stream import portforward
        except ImportError:
            print("[coverage-client] kubernetes package not installed")
            print("  Install with: pip install kubernetes")
            print("  Or use use_kubectl=True to use kubectl binary")
            return None
        
        print(f"[coverage-client] Using native Python port-forward")
        
        try:
            # Load kubeconfig
            try:
                config.load_incluster_config()
                print("[coverage-client] Using in-cluster config")
            except:
                config.load_kube_config()
                print("[coverage-client] Using kubeconfig")
            
            # Create API client
            v1 = client.CoreV1Api()
            
            # Create port-forward connection
            pf = portforward(
                v1.connect_get_namespaced_pod_portforward,
                pod_name,
                self.namespace,
                ports=str(coverage_port),
            )
            
            print(f"[coverage-client] Port-forward established")
            
            # Fetch coverage data through the port-forward socket
            return self._fetch_coverage_via_socket(pf, coverage_port, test_name, timeout)
            
        except Exception as e:
            print(f"[coverage-client] Error with native port-forward: {e}")
            import traceback
            traceback.print_exc()
            return None
    
    def _fetch_coverage_via_socket(self, pf, remote_port: int, test_name: str, timeout: int) -> Optional[str]:
        """Fetch coverage data through a PortForward socket."""
        try:
            import json
            import base64
            
            # Get the socket for the remote port
            sock = pf.socket(remote_port)
            
            # Build HTTP GET request
            http_request = f"GET /coverage?name={test_name} HTTP/1.1\r\nHost: localhost\r\nConnection: close\r\n\r\n"
            sock.sendall(http_request.encode())
            
            # Read HTTP response
            response_data = b""
            while True:
                chunk = sock.recv(4096)
                if not chunk:
                    break
                response_data += chunk
            
            # Parse HTTP response
            response_str = response_data.decode('utf-8')
            
            # Split headers and body
            header_end = response_str.find('\r\n\r\n')
            if header_end == -1:
                print("[coverage-client] Invalid HTTP response")
                return None
            
            headers = response_str[:header_end]
            body = response_str[header_end + 4:]
            
            # Check status code
            if '200 OK' not in headers:
                print(f"[coverage-client] HTTP error: {headers.split()[1]}")
                return None
            
            # Parse JSON response
            coverage_response = json.loads(body)
            coverage_b64 = coverage_response.get("coverage_data")
            
            if not coverage_b64:
                print("[coverage-client] No coverage data in response")
                return None
            
            # Decode coverage data and convert to SQLite format
            from coverage.data import CoverageData
            import tempfile
            import shutil
            
            coverage_json = base64.b64decode(coverage_b64)
            
            # Convert serialized format to SQLite database
            tmp_db = tempfile.mktemp(suffix='.db')
            cov_data = CoverageData(basename=tmp_db)
            cov_data.loads(coverage_json)
            cov_data.write()
            
            # Save as SQLite database
            coverage_file = self.output_dir / f".coverage_{test_name}"
            shutil.copy2(tmp_db, coverage_file)
            
            print(f"[coverage-client] Coverage data saved to {coverage_file}")
            return str(coverage_file)
            
        except Exception as e:
            print(f"[coverage-client] Error fetching coverage via socket: {e}")
            import traceback
            traceback.print_exc()
            return None
    
    def _fetch_coverage_data(self, local_port: int, test_name: str, timeout: int) -> Optional[str]:
        """Fetch coverage data from the port-forwarded endpoint."""
        try:
            # Collect coverage data
            coverage_url = f"http://localhost:{local_port}/coverage?name={test_name}"
            response = requests.get(coverage_url, timeout=timeout)
            
            if response.status_code != 200:
                print(f"[coverage-client] Failed to collect coverage: HTTP {response.status_code}")
                return None
            
            data = response.json()
            coverage_b64 = data.get("coverage_data")
            if not coverage_b64:
                print("[coverage-client] No coverage data in response")
                return None
            
            # Decode coverage data and convert to SQLite format
            from coverage.data import CoverageData
            import tempfile
            import shutil
            
            coverage_json = base64.b64decode(coverage_b64)
            
            # Convert serialized format to SQLite database
            tmp_db = tempfile.mktemp(suffix='.db')
            cov_data = CoverageData(basename=tmp_db)
            cov_data.loads(coverage_json)
            cov_data.write()
            
            # Save as SQLite database
            coverage_file = self.output_dir / f".coverage_{test_name}"
            shutil.copy2(tmp_db, coverage_file)
            
            print(f"[coverage-client] Coverage data saved to {coverage_file}")
            return str(coverage_file)
            
        except Exception as e:
            print(f"[coverage-client] Error fetching coverage data: {e}")
            return None
    
    def _reset_with_kubectl(
        self,
        pod_name: str,
        coverage_port: int,
        timeout: int
    ) -> bool:
        """Reset coverage using kubectl binary."""
        import subprocess
        
        local_port = self._find_free_port()
        
        # Start port-forwarding
        pf_process = subprocess.Popen(
            [
                "kubectl", "port-forward",
                "-n", self.namespace,
                pod_name,
                f"{local_port}:{coverage_port}"
            ],
            stdout=subprocess.PIPE,
            stderr=subprocess.PIPE
        )
        
        try:
            # Wait for port-forward to be ready
            time.sleep(2)
            
            # Reset coverage
            reset_url = f"http://localhost:{local_port}/coverage/reset"
            response = requests.get(reset_url, timeout=timeout)
            
            if response.status_code == 200:
                print(f"[coverage-client] ✓ Coverage counters reset")
                return True
            else:
                print(f"[coverage-client] Failed to reset coverage: HTTP {response.status_code}")
                return False
                
        except Exception as e:
            print(f"[coverage-client] Error resetting coverage: {e}")
            return False
        finally:
            pf_process.terminate()
            pf_process.wait(timeout=5)
    
    def _reset_with_native_portforward(
        self,
        pod_name: str,
        coverage_port: int,
        timeout: int
    ) -> bool:
        """Reset coverage using native Python Kubernetes client."""
        try:
            from kubernetes import client, config
            from kubernetes.stream import portforward
        except ImportError:
            print("[coverage-client] kubernetes package not installed")
            return False
        
        try:
            # Load kubeconfig
            try:
                config.load_incluster_config()
            except:
                config.load_kube_config()
            
            # Create API client
            v1 = client.CoreV1Api()
            
            # Create port-forward connection
            pf = portforward(
                v1.connect_get_namespaced_pod_portforward,
                pod_name,
                self.namespace,
                ports=str(coverage_port),
            )
            
            # Get the socket for the remote port
            sock = pf.socket(coverage_port)
            
            # Build HTTP GET request
            http_request = f"GET /coverage/reset HTTP/1.1\r\nHost: localhost\r\nConnection: close\r\n\r\n"
            sock.sendall(http_request.encode())
            
            # Read HTTP response
            response_data = b""
            while True:
                chunk = sock.recv(4096)
                if not chunk:
                    break
                response_data += chunk
            
            # Check if successful
            if b'200 OK' in response_data:
                print(f"[coverage-client] ✓ Coverage counters reset")
                return True
            else:
                print(f"[coverage-client] Failed to reset coverage")
                return False
            
        except Exception as e:
            print(f"[coverage-client] Error resetting coverage: {e}")
            return False
    
    def generate_coverage_report(self, test_name: str, source_dir: str = ".", remap_paths: bool = True) -> None:
        """
        Generate text coverage report from collected data.
        
        Args:
            test_name: Name of the test
            source_dir: Source directory for coverage analysis
            remap_paths: If True, remap container paths (/app/) to local paths
        """
        from coverage.data import CoverageData
        import os
        import tempfile
        
        coverage_file = self.output_dir / f".coverage_{test_name}"
        if not coverage_file.exists():
            print(f"[coverage-client] Coverage file not found: {coverage_file}")
            return
        
        # Load coverage data (always SQLite format)
        import shutil
        tmp_db_path = tempfile.mktemp(suffix='.db')
        shutil.copy2(coverage_file, tmp_db_path)
        cov_data = CoverageData(basename=tmp_db_path)
        cov_data.read()
        
        # Apply path remapping if needed
        if remap_paths:
            # Auto-detect container paths and remap to source_dir
            remapped_bytes = self._remap_coverage_paths(cov_data.dumps(), source_dir=source_dir)
            new_db_path = tempfile.mktemp(suffix='.db')
            cov_data = CoverageData(basename=new_db_path)
            cov_data.loads(remapped_bytes)
        
        cov = Coverage(data_file=None)
        cov._data = cov_data
        
        # Generate text report
        report_file = self.output_dir / f"report_{test_name}.txt"
        with report_file.open('w') as f:
            cov.report(file=f)
        
        print(f"[coverage-client] Text report saved to {report_file}")
        
        # Also print to console
        cov.report()
    
    def generate_html_report(self, test_name: str, source_dir: str = ".", remap_paths: bool = True) -> None:
        """
        Generate HTML coverage report from collected data.
        
        Args:
            test_name: Name of the test
            source_dir: Source directory for coverage analysis
            remap_paths: If True, remap container paths (/app/) to local paths
        """
        from coverage.data import CoverageData
        import os
        import tempfile
        
        coverage_file = self.output_dir / f".coverage_{test_name}"
        if not coverage_file.exists():
            print(f"[coverage-client] Coverage file not found: {coverage_file}")
            return
        
        # Load coverage data (always SQLite format)
        import shutil
        tmp_db_path = tempfile.mktemp(suffix='.db')
        shutil.copy2(coverage_file, tmp_db_path)
        cov_data = CoverageData(basename=tmp_db_path)
        cov_data.read()
        
        # Apply path remapping if needed
        if remap_paths:
            # Auto-detect container paths and remap to source_dir
            remapped_bytes = self._remap_coverage_paths(cov_data.dumps(), source_dir=source_dir)
            new_db_path = tempfile.mktemp(suffix='.db')
            cov_data = CoverageData(basename=new_db_path)
            cov_data.loads(remapped_bytes)
        
        cov = Coverage(data_file=None)
        cov._data = cov_data
        
        # Generate HTML report
        html_dir = self.output_dir / f"html_{test_name}"
        cov.html_report(directory=str(html_dir))
        
        print(f"[coverage-client] HTML report saved to {html_dir}/index.html")
    
    def generate_xml_report(self, test_name: str, source_dir: str = ".", remap_paths: bool = True) -> None:
        """
        Generate XML coverage report (for Codecov, etc.).
        
        Args:
            test_name: Name of the test
            source_dir: Source directory for coverage analysis
            remap_paths: If True, remap container paths (/app/) to local paths
                        Note: Should be True so coverage.py can find source files
        """
        from coverage.data import CoverageData
        import os
        import tempfile
        
        coverage_file = self.output_dir / f".coverage_{test_name}"
        if not coverage_file.exists():
            print(f"[coverage-client] Coverage file not found: {coverage_file}")
            return
        
        # Load coverage data (always SQLite format)
        import shutil
        tmp_db_path = tempfile.mktemp(suffix='.db')
        shutil.copy2(coverage_file, tmp_db_path)
        cov_data = CoverageData(basename=tmp_db_path)
        cov_data.read()
        
        # Apply path remapping if needed
        if remap_paths:
            # Auto-detect container paths and remap to source_dir
            remapped_bytes = self._remap_coverage_paths(cov_data.dumps(), source_dir=source_dir)
            new_db_path = tempfile.mktemp(suffix='.db')
            cov_data = CoverageData(basename=new_db_path)
            cov_data.loads(remapped_bytes)
        
        cov = Coverage(data_file=None)
        cov._data = cov_data
        
        # Generate XML report (ignore errors for missing source files)
        xml_file = self.output_dir / "coverage.xml"
        cov.xml_report(outfile=str(xml_file), ignore_errors=True)
        
        print(f"[coverage-client] XML report saved to {xml_file}")
    
    def merge_coverage_files(self, test_names: list[str], merged_name: str = "merged") -> None:
        """
        Merge multiple coverage files into one.
        
        Args:
            test_names: List of test names to merge
            merged_name: Name for the merged coverage file
        """
        from coverage.data import CoverageData
        import tempfile
        
        # Create merged data with unique temp database
        merged_db_path = tempfile.mktemp(suffix='.db')
        merged_data = CoverageData(basename=merged_db_path)
        
        for test_name in test_names:
            coverage_file = self.output_dir / f".coverage_{test_name}"
            if coverage_file.exists():
                # Load SQLite database
                import shutil
                load_db_path = tempfile.mktemp(suffix='.db')
                shutil.copy2(coverage_file, load_db_path)
                cov_data = CoverageData(basename=load_db_path)
                cov_data.read()
                merged_data.update(cov_data)
                print(f"[coverage-client] Merged coverage from {test_name}")
            else:
                print(f"[coverage-client] Warning: Coverage file not found for {test_name}")
        
        # Save merged data as SQLite database
        merged_file = self.output_dir / f".coverage_{merged_name}"
        merged_data.write()
        import shutil
        shutil.copy2(merged_db_path, merged_file)
        
        print(f"[coverage-client] Merged coverage saved to {merged_file}")
    
    def _detect_container_paths(self, coverage_data, source_dir: str = ".") -> dict:
        """
        Auto-detect container path mappings by analyzing coverage data.
        Uses intelligent matching based on relative path structure.
        
        Args:
            coverage_data: CoverageData object
            source_dir: Local source directory to search for matching files
        
        Returns a dictionary mapping container paths to local paths.
        For example: {'/app/': '/Users/user/project/'}
        """
        import os
        from pathlib import Path
        from collections import defaultdict
        
        measured_files = sorted(coverage_data.measured_files())  # Sort for determinism
        container_files = []
        
        # Find files that don't exist locally (these are container paths)
        for file_path in measured_files:
            if not os.path.exists(file_path):
                container_files.append(file_path)
        
        if not container_files:
            # No container paths detected - all files already have local paths
            return {}
        
        # Build a map of relative paths to local file paths
        # Key: relative path parts (tuple), Value: full local path
        source_path = Path(source_dir).resolve()
        local_files_by_relpath = {}
        
        # Collect all local Python files with their relative path structure
        for local_file in sorted(source_path.rglob("*.py")):  # Sort for determinism
            rel_path = local_file.relative_to(source_path)
            # Store using path parts for better matching
            path_parts = rel_path.parts
            local_files_by_relpath[path_parts] = str(local_file)
        
        # Try to find the best common container root by matching directory structures
        # Group container files by potential root paths
        potential_mappings = defaultdict(list)  # {container_root: [(container_file, local_file)]}
        
        for container_file in container_files:
            container_path = Path(container_file)
            container_parts = container_path.parts
            filename = container_path.name
            
            # Try to find matching local file based on path structure
            best_match = None
            best_match_score = 0
            
            for local_parts, local_path in local_files_by_relpath.items():
                # Files must have same name
                if local_parts[-1] != filename:
                    continue
                
                # Count matching suffix parts (from filename backwards)
                match_score = 0
                for i in range(1, min(len(container_parts), len(local_parts)) + 1):
                    if container_parts[-i] == local_parts[-i]:
                        match_score = i
                    else:
                        break
                
                # Prefer longer matches (more specific paths)
                if match_score > best_match_score:
                    best_match_score = match_score
                    best_match = (local_parts, local_path)
            
            if best_match:
                local_parts, local_path = best_match
                
                # Extract the container root by removing the matched suffix
                # For /app/hello/page/views.py matching to .../hello/page/views.py
                # Container root is /app/ 
                container_root_parts = container_parts[:-best_match_score]
                if container_root_parts:
                    # Build container root with proper path joining
                    container_root = os.path.join('/', *container_root_parts) + '/'
                    
                    # Build local root with proper path joining
                    local_root_parts = Path(local_path).parts[:-best_match_score]
                    if local_root_parts:
                        local_root = os.path.join(*local_root_parts) + '/'
                        potential_mappings[container_root].append((container_file, local_path))
        
        # Select the most common container root (the one with the most matches)
        # This gives us the primary mapping like /app/ -> /Users/.../project/
        path_mappings = {}
        
        if potential_mappings:
            # Sort by number of matches (descending), then alphabetically for determinism
            sorted_roots = sorted(
                potential_mappings.items(),
                key=lambda x: (-len(x[1]), x[0])  # More matches first, then alphabetically
            )
            
            # Use the root with the most matches
            best_root, matches = sorted_roots[0]
            
            # Find the local root by analyzing the matches
            if matches:
                container_file, first_local = matches[0]
                
                # Calculate how many path parts match
                container_parts = Path(container_file).parts
                local_parts = Path(first_local).parts
                
                # Find matching suffix length
                match_len = 0
                for i in range(1, min(len(container_parts), len(local_parts)) + 1):
                    if container_parts[-i] == local_parts[-i]:
                        match_len = i
                    else:
                        break
                
                # Calculate local root: remove the matching suffix from local path
                local_root_parts = local_parts[:-match_len]
                if local_root_parts:
                    # Use os.path.join for proper path construction
                    local_root = os.path.join(*local_root_parts)
                    if not local_root.startswith('/'):
                        local_root = '/' + local_root
                    if not local_root.endswith('/'):
                        local_root += '/'
                    path_mappings[best_root] = local_root
        
        return path_mappings
    
    def _remap_coverage_paths(self, coverage_bytes: bytes, source_dir: str = "."):
        """
        Remap coverage paths from container to local filesystem.
        Automatically detects container paths by analyzing the coverage data.
        
        Args:
            coverage_bytes: Coverage data in binary format
            source_dir: Local source directory to search for matching files
        
        Returns:
            New coverage data with remapped paths (as bytes)
        """
        from coverage.data import CoverageData
        import os
        import tempfile
        
        # Load original coverage data using a temp database
        tmp_db_path = tempfile.mktemp(suffix='.db')
        original_data = CoverageData(basename=tmp_db_path)
        original_data.loads(coverage_bytes)
        
        # Auto-detect container paths
        path_mappings = self._detect_container_paths(original_data, source_dir)
        
        if path_mappings:
            print(f"[coverage-client] Auto-detected path mappings: {path_mappings}")
        else:
            print(f"[coverage-client] No container paths detected, using paths as-is")
        
        # Create new coverage data with remapped paths using a different temp database
        new_db_path = tempfile.mktemp(suffix='.db')
        new_data = CoverageData(basename=new_db_path)
        
        # Remap each measured file
        for old_file in original_data.measured_files():
            # Skip coverage server and other instrumentation files
            if 'coverage_server.py' in old_file or '/site-packages/' in old_file:
                continue
            
            new_file = old_file
            
            # Try each path mapping
            for container_prefix, local_prefix in path_mappings.items():
                if old_file.startswith(container_prefix):
                    new_file = old_file.replace(container_prefix, local_prefix, 1)
                    break
            
            # Only include if local file exists
            if os.path.exists(new_file):
                # Get coverage data
                lines = original_data.lines(old_file)
                if lines:
                    new_data.add_lines({new_file: lines})
                
                arcs = original_data.arcs(old_file)
                if arcs:
                    new_data.add_arcs({new_file: list(arcs)})
        
        # Serialize and return
        return new_data.dumps()
    
    def _find_free_port(self) -> int:
        """Find a free local port for port-forwarding."""
        import socket
        with socket.socket(socket.AF_INET, socket.SOCK_STREAM) as s:
            s.bind(('', 0))
            s.listen(1)
            port = s.getsockname()[1]
        return port

