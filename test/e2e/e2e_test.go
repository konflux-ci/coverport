package e2e

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// syncBuffer is a bytes.Buffer guarded by a mutex so a subprocess can Write
// concurrently with String() reads from the test goroutine.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

var coverportBin string

func TestMain(m *testing.M) {
	bin := os.Getenv("COVERPORT_BIN")
	if bin == "" {
		bin = "coverport"
	}
	var err error
	coverportBin, err = filepath.Abs(bin)
	if err != nil {
		fmt.Fprintf(os.Stderr, "cannot resolve coverport binary path %q: %v\n", bin, err)
		os.Exit(1)
	}
	if _, err := os.Stat(coverportBin); err != nil {
		fmt.Fprintf(os.Stderr, "coverport binary not found at %q: %v\n", coverportBin, err)
		os.Exit(1)
	}
	os.Exit(m.Run())
}

type langFixture struct {
	image  string
	format string // coverage format for `coverport process`
}

const defaultImagePrefix = "quay.io/konflux-ci/konflux-devprod/coverport-testapp"

func fixtureImage(lang string) string {
	if prefix := os.Getenv("TESTAPP_IMAGE_PREFIX"); prefix != "" {
		return prefix + "-" + lang + ":latest"
	}
	return defaultImagePrefix + "-" + lang + ":latest"
}

// Kind/HTTP coverport targets (container instrumentation).
// Python: Pattern D (pytest-cov) — see TestPythonPytestCov.
// Node.js: Pattern C (NYC filesystem process) — see TestProcessNodejsFilesystem
// (collect does not support Node HTTP; format collides with Python).
var (
	goFixture     = langFixture{image: fixtureImage("go"), format: "go"}
	rustFixture   = langFixture{image: fixtureImage("rust"), format: "rust"}
	nodejsFixture = langFixture{image: fixtureImage("nodejs"), format: "nyc"}
)

func fixtureNamespace(testPrefix, lang string) string {
	return "e2e-" + testPrefix + "-" + lang
}

func podManifest(name, namespace, image string, labels map[string]string) string {
	labelYAML := ""
	for k, v := range labels {
		labelYAML += fmt.Sprintf("    %s: %q\n", k, v)
	}

	return fmt.Sprintf(`apiVersion: v1
kind: Pod
metadata:
  name: %s
  namespace: %s
  labels:
%s
spec:
  containers:
  - name: app
    image: %s
    imagePullPolicy: Never
    ports:
    - containerPort: 8080
      name: http
    - containerPort: 53700
      name: coverage
    readinessProbe:
      httpGet:
        path: /hello
        port: 8080
      initialDelaySeconds: 2
      periodSeconds: 3
`, name, namespace, labelYAML, image)
}

func runCmd(t *testing.T, name string, args ...string) (string, string) {
	t.Helper()
	cmd := exec.Command(name, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	cmd.Env = os.Environ()
	err := cmd.Run()
	if err != nil {
		t.Fatalf("command %q %v failed: %v\nstdout: %s\nstderr: %s",
			name, args, err, stdout.String(), stderr.String())
	}
	return stdout.String(), stderr.String()
}

func runCoverport(t *testing.T, args ...string) (string, string) {
	t.Helper()
	cmd := exec.Command(coverportBin, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	cmd.Env = os.Environ()
	err := cmd.Run()
	if err != nil {
		t.Fatalf("coverport %v failed: %v\nstdout: %s\nstderr: %s",
			args, err, stdout.String(), stderr.String())
	}
	return stdout.String(), stderr.String()
}

// runCoverportExpectFail runs coverport and requires a non-zero exit.
// If wantSubstr is non-empty, stdout or stderr must contain it.
func runCoverportExpectFail(t *testing.T, wantSubstr string, args ...string) (string, string) {
	t.Helper()
	cmd := exec.Command(coverportBin, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	cmd.Env = os.Environ()
	err := cmd.Run()
	if err == nil {
		t.Fatalf("expected coverport %v to fail, but it succeeded\nstdout: %s\nstderr: %s",
			args, stdout.String(), stderr.String())
	}
	combined := stdout.String() + stderr.String()
	if wantSubstr != "" && !strings.Contains(combined, wantSubstr) {
		t.Fatalf("expected coverport output to contain %q\nstdout: %s\nstderr: %s",
			wantSubstr, stdout.String(), stderr.String())
	}
	return stdout.String(), stderr.String()
}


func kubectl(t *testing.T, args ...string) string {
	t.Helper()
	out, _ := runCmd(t, "kubectl", args...)
	return out
}

func createNamespace(t *testing.T, ns string) {
	t.Helper()
	cmd := exec.Command("kubectl", "create", "namespace", ns)
	cmd.Run() // ignore error if already exists
}

func deleteNamespace(t *testing.T, ns string) {
	t.Helper()
	cmd := exec.Command("kubectl", "delete", "namespace", ns, "--ignore-not-found", "--wait=false")
	cmd.Run()
}

func deployPod(t *testing.T, lang string, fixture langFixture, namespace string) {
	t.Helper()
	labels := map[string]string{
		"app":                    fmt.Sprintf("testapp-%s", lang),
		"coverport.dev/language": lang,
	}
	manifest := podManifest(
		fmt.Sprintf("testapp-%s", lang),
		namespace,
		fixture.image,
		labels,
	)
	cmd := exec.Command("kubectl", "apply", "-f", "-")
	cmd.Stdin = strings.NewReader(manifest)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("failed to deploy pod for %s: %v\nstderr: %s", lang, err, stderr.String())
	}
}

func waitForPodReady(t *testing.T, namespace, podName string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		cmd := exec.Command("kubectl", "get", "pod", podName,
			"-n", namespace,
			"-o", "jsonpath={.status.conditions[?(@.type=='Ready')].status}")
		out, err := cmd.Output()
		if err == nil && strings.TrimSpace(string(out)) == "True" {
			return
		}
		time.Sleep(3 * time.Second)
	}
	// Print pod status for debugging
	cmd := exec.Command("kubectl", "describe", "pod", podName, "-n", namespace)
	out, _ := cmd.Output()
	t.Fatalf("pod %s/%s not ready within %v\n%s", namespace, podName, timeout, string(out))
}

func waitForPortForward(t *testing.T, pfOut *syncBuffer) string {
	t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		output := pfOut.String()
		if idx := strings.Index(output, "Forwarding from 127.0.0.1:"); idx >= 0 {
			rest := output[idx+len("Forwarding from 127.0.0.1:"):]
			if end := strings.Index(rest, " "); end > 0 {
				return rest[:end]
			}
		}
		time.Sleep(500 * time.Millisecond)
	}
	t.Fatalf("port-forward did not become ready within 15s, output: %s", pfOut.String())
	return ""
}

// portForwardTo starts a port-forward to the given pod/port and returns the
// local "host:port" string plus a cleanup function that kills the process.
func portForwardTo(t *testing.T, namespace, podName string, remotePort int) (string, func()) {
	t.Helper()
	pfCmd := exec.Command("kubectl", "port-forward",
		"-n", namespace, podName, fmt.Sprintf("0:%d", remotePort))
	var pfOut syncBuffer
	pfCmd.Stdout = &pfOut
	pfCmd.Stderr = &pfOut
	if err := pfCmd.Start(); err != nil {
		t.Fatalf("failed to start port-forward to %s/%s:%d: %v", namespace, podName, remotePort, err)
	}
	stop := func() { pfCmd.Process.Kill(); pfCmd.Wait() }
	t.Cleanup(stop)
	localPort := waitForPortForward(t, &pfOut)
	return localPort, stop
}

func exerciseApp(t *testing.T, namespace, podName string) {
	t.Helper()
	localPort, stop := portForwardTo(t, namespace, podName, 8080)
	defer stop()

	baseURL := fmt.Sprintf("http://localhost:%s", localPort)
	for _, path := range []string{"/hello", "/hello?name=test", "/hello?name=coverport"} {
		resp, err := http.Get(baseURL + path)
		if err != nil {
			t.Fatalf("failed to GET %s: %v", path, err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("GET %s returned %d, expected 200", path, resp.StatusCode)
		}
	}
}

// containerRuntime returns "docker" or "podman", preferring docker (CI/Kind).
func containerRuntime() (string, error) {
	for _, rt := range []string{"docker", "podman"} {
		if _, err := exec.LookPath(rt); err == nil {
			return rt, nil
		}
	}
	return "", fmt.Errorf("neither docker nor podman found in PATH")
}

// extractImageBinary copies containerPath from image to a temp file (create+cp+rm).
// Used for Rust process: llvm-cov needs the instrumented binary that matches the profraw.
func extractImageBinary(t *testing.T, image, containerPath string) string {
	t.Helper()
	rt, err := containerRuntime()
	if err != nil {
		t.Fatal(err)
	}

	createOut, err := exec.Command(rt, "create", image).CombinedOutput()
	if err != nil {
		t.Fatalf("%s create %s failed: %v\n%s", rt, image, err, createOut)
	}
	cid := strings.TrimSpace(string(createOut))
	defer func() {
		_ = exec.Command(rt, "rm", cid).Run()
	}()

	dest := filepath.Join(t.TempDir(), "coverage-binary")
	cpOut, err := exec.Command(rt, "cp", cid+":"+containerPath, dest).CombinedOutput()
	if err != nil {
		t.Fatalf("%s cp %s:%s failed: %v\n%s", rt, cid, containerPath, err, cpOut)
	}
	if err := os.Chmod(dest, 0755); err != nil {
		t.Fatalf("chmod extracted binary: %v", err)
	}

	info, err := os.Stat(dest)
	if err != nil || info.Size() == 0 {
		t.Fatalf("extracted binary missing or empty at %s: %v", dest, err)
	}
	t.Logf("extracted %s:%s -> %s (%d bytes) via %s", image, containerPath, dest, info.Size(), rt)
	return dest
}

func TestCollectGo(t *testing.T) {
	collectFromLanguage(t, "go", goFixture)
}

func TestCollectRust(t *testing.T) {
	collectFromLanguage(t, "rust", rustFixture)
}

func collectFromLanguage(t *testing.T, lang string, fixture langFixture) {
	t.Helper()
	ns := fixtureNamespace("collect", lang)
	createNamespace(t, ns)
	t.Cleanup(func() { deleteNamespace(t, ns) })

	deployPod(t, lang, fixture, ns)
	podName := fmt.Sprintf("testapp-%s", lang)
	waitForPodReady(t, ns, podName, 3*time.Minute)
	exerciseApp(t, ns, podName)

	outputDir := t.TempDir()
	runCoverport(t, "collect",
		"--namespace", ns,
		"--pods", podName,
		"--output", outputDir,
		"--test-name", fmt.Sprintf("e2e-%s", lang),
		"--auto-process=false",
	)

	entries, err := os.ReadDir(outputDir)
	if err != nil {
		t.Fatalf("failed to read output dir: %v", err)
	}
	if len(entries) == 0 {
		t.Fatal("output directory is empty — no coverage collected")
	}

	hasNonZeroFile := false
	for _, e := range entries {
		info, _ := e.Info()
		if info != nil {
			t.Logf("  %s (%d bytes)", e.Name(), info.Size())
			if info.Size() > 0 {
				hasNonZeroFile = true
			}
		}
	}
	if !hasNonZeroFile {
		t.Fatal("all collected files are empty")
	}
}

func TestDiscover(t *testing.T) {
	ns := fixtureNamespace("discover", "go")
	createNamespace(t, ns)
	t.Cleanup(func() { deleteNamespace(t, ns) })

	deployPod(t, "go", goFixture, ns)
	podName := "testapp-go"
	waitForPodReady(t, ns, podName, 3*time.Minute)

	stdout, _ := runCoverport(t, "discover",
		"--namespace", ns,
		"--pods", podName,
		"--verbose",
	)

	if !strings.Contains(stdout, podName) {
		t.Errorf("discover output should mention pod %q, got:\n%s", podName, stdout)
	}
}

func TestDiscoverByLabelSelector(t *testing.T) {
	ns := fixtureNamespace("label", "go")
	createNamespace(t, ns)
	t.Cleanup(func() { deleteNamespace(t, ns) })

	deployPod(t, "go", goFixture, ns)
	podName := "testapp-go"
	waitForPodReady(t, ns, podName, 3*time.Minute)

	stdout, _ := runCoverport(t, "discover",
		"--namespace", ns,
		"--label-selector", "app=testapp-go",
		"--verbose",
	)

	if !strings.Contains(stdout, podName) {
		t.Errorf("discover by label should find pod %q, got:\n%s", podName, stdout)
	}
}

func TestCollectMultipleLanguages(t *testing.T) {
	ns := "e2e-multi"
	createNamespace(t, ns)
	t.Cleanup(func() { deleteNamespace(t, ns) })

	deployPod(t, "go", goFixture, ns)
	deployPod(t, "rust", rustFixture, ns)

	waitForPodReady(t, ns, "testapp-go", 3*time.Minute)
	waitForPodReady(t, ns, "testapp-rust", 3*time.Minute)

	exerciseApp(t, ns, "testapp-rust")

	outputDir := t.TempDir()

	runCoverport(t, "collect",
		"--namespace", ns,
		"--pods", "testapp-go,testapp-rust",
		"--output", outputDir,
		"--test-name", "e2e-multi",
		"--auto-process=false",
	)

	metadataPath := filepath.Join(outputDir, "metadata.json")
	data, err := os.ReadFile(metadataPath)
	if err != nil {
		entries, _ := os.ReadDir(outputDir)
		for _, e := range entries {
			t.Logf("  %s (dir=%v)", e.Name(), e.IsDir())
		}
		t.Fatalf("metadata.json not found at %s: %v", metadataPath, err)
	}

	var metadata map[string]interface{}
	if err := json.Unmarshal(data, &metadata); err != nil {
		t.Fatalf("invalid metadata.json: %v", err)
	}

	components, ok := metadata["components"].([]interface{})
	if !ok || len(components) < 2 {
		t.Fatalf("expected at least 2 components in metadata.json, got: %s", string(data))
	}
	t.Logf("metadata.json: %d components", len(components))
}

func TestProcessGo(t *testing.T) {
	processLanguage(t, "go", goFixture, nil)
}

func TestProcessRust(t *testing.T) {
	processLanguage(t, "rust", rustFixture, func(t *testing.T, cmd *exec.Cmd, fixture langFixture) {
		binary := extractImageBinary(t, fixture.image, "/testapp")
		cmd.Env = append(cmd.Env, "COVERAGE_BINARY="+binary)
	})
}

func processLanguage(t *testing.T, lang string, fixture langFixture, prepareEnv func(*testing.T, *exec.Cmd, langFixture)) {
	t.Helper()
	ns := fixtureNamespace("process", lang)
	createNamespace(t, ns)
	t.Cleanup(func() { deleteNamespace(t, ns) })

	deployPod(t, lang, fixture, ns)
	podName := fmt.Sprintf("testapp-%s", lang)
	waitForPodReady(t, ns, podName, 3*time.Minute)
	exerciseApp(t, ns, podName)

	collectDir := t.TempDir()
	runCoverport(t, "collect",
		"--namespace", ns,
		"--pods", podName,
		"--output", collectDir,
		"--test-name", fmt.Sprintf("e2e-process-%s", lang),
		"--auto-process=false",
	)

	workspace := t.TempDir()
	componentRepoDir := filepath.Join(workspace, podName, "repo")
	if err := os.MkdirAll(componentRepoDir, 0755); err != nil {
		t.Fatalf("failed to create component repo dir: %v", err)
	}

	cmd := exec.Command(coverportBin, "process",
		"--coverage-dir", collectDir,
		"--format", fixture.format,
		"--upload=false",
		"--skip-clone",
		"--keep-workspace",
		"--workspace", workspace,
		"--repo-url", "https://github.com/konflux-ci/coverport",
		"--commit-sha", "abc123def456789",
	)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	cmd.Env = os.Environ()
	if prepareEnv != nil {
		prepareEnv(t, cmd, fixture)
	}
	err := cmd.Run()

	t.Logf("process stdout:\n%s", stdout.String())
	if stderr.Len() > 0 {
		t.Logf("process stderr:\n%s", stderr.String())
	}
	if err != nil {
		t.Fatalf("%s processing failed: %v", lang, err)
	}

	entries, _ := os.ReadDir(workspace)
	t.Logf("workspace after process (%s):", lang)
	for _, e := range entries {
		info, _ := e.Info()
		if info != nil {
			t.Logf("  %s (%d bytes, dir=%v)", e.Name(), info.Size(), e.IsDir())
		}
	}

	if lang == "rust" {
		if !strings.Contains(stdout.String(), "LCOV report:") &&
			!strings.Contains(stdout.String(), "Rust coverage processed successfully") {
			t.Fatal("rust process succeeded but stdout missing LCOV success markers")
		}
	}
}

// TestPythonPytestCov exercises Pattern D from coverport-integration: run pytest
// directly against source with --cov and produce coverage XML. No Kind pod,
// no coverport collect/process — matches the skill's intended Python path.
func TestPythonPytestCov(t *testing.T) {
	fixtureDir, err := filepath.Abs("../fixtures/python")
	if err != nil {
		t.Fatalf("resolve python fixture dir: %v", err)
	}
	if _, err := os.Stat(filepath.Join(fixtureDir, "app.py")); err != nil {
		t.Fatalf("python fixture missing at %s: %v", fixtureDir, err)
	}

	outDir := t.TempDir()
	coverageXML := filepath.Join(outDir, "coverage-e2e.xml")

	cmd := exec.Command("python3", "-m", "pytest",
		".",
		"-vv", "--tb=short",
		"--cov=app",
		"--cov-report=xml:"+coverageXML,
		"--cov-report=term",
		"--cov-branch",
	)
	cmd.Dir = fixtureDir
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	cmd.Env = os.Environ()
	if err := cmd.Run(); err != nil {
		t.Fatalf("pytest --cov failed: %v\nstdout:\n%s\nstderr:\n%s",
			err, stdout.String(), stderr.String())
	}
	t.Logf("pytest stdout:\n%s", stdout.String())

	info, err := os.Stat(coverageXML)
	if err != nil {
		t.Fatalf("coverage XML not written to %s: %v", coverageXML, err)
	}
	if info.Size() == 0 {
		t.Fatal("coverage XML is empty")
	}

	data, err := os.ReadFile(coverageXML)
	if err != nil {
		t.Fatalf("read coverage XML: %v", err)
	}
	if !strings.Contains(string(data), "app.py") {
		t.Fatalf("coverage XML missing app.py; got:\n%s", string(data))
	}
	t.Logf("coverage XML: %s (%d bytes)", coverageXML, info.Size())
}

// Is only documented for Go not sure if should work for other languages..
func TestCollectWithAutoProcess(t *testing.T) {
	ns := fixtureNamespace("auto", "go")
	createNamespace(t, ns)
	t.Cleanup(func() { deleteNamespace(t, ns) })

	podName := "testapp-go"
	deployPod(t, "go", goFixture, ns)
	waitForPodReady(t, ns, podName, 3*time.Minute)
	exerciseApp(t, ns, podName)

	outputDir := t.TempDir()

	runCoverport(t, "collect",
		"--namespace", ns,
		"--pods", podName,
		"--output", outputDir,
		"--test-name", "e2e-autoprocess-go",
	)

	foundReport := false
	filepath.WalkDir(outputDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.Name() == "coverage.out" {
			info, _ := d.Info()
			if info != nil && info.Size() > 0 {
				foundReport = true
				t.Logf("processed report: %s (%d bytes)", path, info.Size())
			}
		}
		return nil
	})

	if !foundReport {
		t.Fatalf("auto-process produced no coverage report (coverage.out missing or empty)")
	}
}

// Failure cases kept here are integration edges (Kind / live HTTP / k8s API).
// Pure CLI validation belongs in unit tests under cli/cmd.
func TestCollectErrorCases(t *testing.T) {
	t.Run("nonexistent_namespace", func(t *testing.T) {
		runCoverportExpectFail(t, "",
			"collect",
			"--namespace", "e2e-does-not-exist",
			"--pods", "nonexistent-pod",
			"--output", t.TempDir(),
			"--test-name", "e2e-error",
		)
	})

	t.Run("no_matching_pods", func(t *testing.T) {
		ns := "e2e-collect-empty"
		createNamespace(t, ns)
		t.Cleanup(func() { deleteNamespace(t, ns) })

		runCoverportExpectFail(t, "No running pods found matching the criteria",
			"collect",
			"--namespace", ns,
			"--label-selector", "app=definitely-does-not-exist",
			"--output", t.TempDir(),
			"--test-name", "e2e-error",
		)
	})

	t.Run("unreachable_url", func(t *testing.T) {
		runCoverportExpectFail(t, "Failed to collect coverage from URL",
			"collect",
			"--url", "http://localhost:19999",
			"--output", t.TempDir(),
			"--test-name", "e2e-error",
			"--timeout", "5",
		)
	})

	t.Run("wrong_coverage_port", func(t *testing.T) {
		ns := fixtureNamespace("badport", "go")
		createNamespace(t, ns)
		t.Cleanup(func() { deleteNamespace(t, ns) })

		deployPod(t, "go", goFixture, ns)
		podName := "testapp-go"
		waitForPodReady(t, ns, podName, 3*time.Minute)

		runCoverportExpectFail(t, "Failed to collect coverage from any pods",
			"collect",
			"--namespace", ns,
			"--pods", podName,
			"--port", "19999",
			"--output", t.TempDir(),
			"--test-name", "e2e-error-badport",
			"--auto-process=false",
		)
	})
}

func TestDiscoverErrorCases(t *testing.T) {
	t.Run("nonexistent_namespace", func(t *testing.T) {
		runCoverportExpectFail(t, "Pod discovery failed",
			"discover",
			"--namespace", "e2e-does-not-exist",
			"--pods", "nonexistent-pod",
		)
	})
}

// TestProcessRustMissingBinary collects real Rust profraw then process without
// COVERAGE_BINARY — the skill-required failure mode for Rust LCOV export.
func TestProcessRustMissingBinary(t *testing.T) {
	ns := fixtureNamespace("nobin", "rust")
	createNamespace(t, ns)
	t.Cleanup(func() { deleteNamespace(t, ns) })

	deployPod(t, "rust", rustFixture, ns)
	podName := "testapp-rust"
	waitForPodReady(t, ns, podName, 3*time.Minute)
	exerciseApp(t, ns, podName)

	collectDir := t.TempDir()
	runCoverport(t, "collect",
		"--namespace", ns,
		"--pods", podName,
		"--output", collectDir,
		"--test-name", "e2e-process-nobin-rust",
		"--auto-process=false",
	)

	workspace := t.TempDir()
	if err := os.MkdirAll(filepath.Join(workspace, podName, "repo"), 0755); err != nil {
		t.Fatal(err)
	}

	runCoverportExpectFail(t, "no instrumented binary found",
		"process",
		"--coverage-dir", collectDir,
		"--format", "rust",
		"--upload=false",
		"--skip-clone",
		"--keep-workspace",
		"--workspace", workspace,
		"--repo-url", "https://github.com/konflux-ci/coverport",
		"--commit-sha", "abc123def456789",
	)
}

func TestCoverageServerEndpointsGo(t *testing.T) {
	coverageServerEndpoints(t, "go", goFixture)
}

func TestCoverageServerEndpointsRust(t *testing.T) {
	coverageServerEndpoints(t, "rust", rustFixture)
}

func coverageServerEndpoints(t *testing.T, lang string, fixture langFixture) {
	t.Helper()
	ns := fixtureNamespace("endpoints", lang)
	createNamespace(t, ns)
	t.Cleanup(func() { deleteNamespace(t, ns) })

	deployPod(t, lang, fixture, ns)
	podName := fmt.Sprintf("testapp-%s", lang)
	waitForPodReady(t, ns, podName, 3*time.Minute)

	localPort, stop := portForwardTo(t, ns, podName, 53700)
	defer stop()

	t.Run("health", func(t *testing.T) {
		resp, err := http.Get(fmt.Sprintf("http://localhost:%s/health", localPort))
		if err != nil {
			t.Fatalf("failed to GET /health: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("expected 200 from /health, got %d", resp.StatusCode)
		}
		body, err := io.ReadAll(resp.Body)
			if err != nil {
				t.Fatalf("failed to read /health response: %v", err)
			}
			if !strings.Contains(string(body), "healthy") {
				t.Errorf("/health response does not contain 'healthy': %s", string(body))
			}
	})

	t.Run("coverage", func(t *testing.T) {
		resp, err := http.Get(fmt.Sprintf("http://localhost:%s/coverage", localPort))
		if err != nil {
			t.Fatalf("failed to GET /coverage: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("expected 200, got %d", resp.StatusCode)
		}
		if resp.Header.Get("X-Art-Coverage-Server") != "1" {
			t.Error("expected X-Art-Coverage-Server header")
		}
		var body map[string]interface{}
		if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
			t.Fatalf("failed to decode coverage response: %v", err)
		}
		keys := make([]string, 0, len(body))
		for k := range body {
			keys = append(keys, k)
		}
		t.Logf("coverage response keys: %v", keys)
	})
}

// TestProcessNodejsFilesystem verifies Pattern C: fetch Istanbul JSON, write
// coverage-final.json, and run `coverport process --format nyc`.
func TestProcessNodejsFilesystem(t *testing.T) {
	ns := fixtureNamespace("nycfs", "nodejs")
	createNamespace(t, ns)
	t.Cleanup(func() { deleteNamespace(t, ns) })

	podName := "testapp-nodejs"
	deployPod(t, "nodejs", nodejsFixture, ns)
	waitForPodReady(t, ns, podName, 3*time.Minute)
	exerciseApp(t, ns, podName)

	// Port-forward to the coverage server and fetch Istanbul JSON.
	localPort, stop := portForwardTo(t, ns, podName, 53700)
	defer stop()

	resp, err := http.Get(fmt.Sprintf("http://localhost:%s/coverage?name=e2e-nycfs", localPort))
	if err != nil {
		t.Fatalf("GET /coverage failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /coverage returned %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("failed to read coverage response body: %v", err)
	}

	// The envelope is {"label":"...","timestamp":"...","coverage_data":"<base64>"}
	var envelope struct {
		CoverageData string `json:"coverage_data"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		t.Fatalf("failed to decode coverage envelope: %v", err)
	}
	if envelope.CoverageData == "" {
		t.Fatal("coverage_data field is empty")
	}

	istanbulJSON, err := base64.StdEncoding.DecodeString(envelope.CoverageData)
	if err != nil {
		t.Fatalf("failed to base64-decode coverage_data: %v", err)
	}
	t.Logf("decoded Istanbul JSON: %d bytes", len(istanbulJSON))

	// Write the Istanbul JSON as coverage-final.json so the NYC processor
	// can find it via findNYCCoverageFile.
	coverageDir := t.TempDir()
	coveragePath := filepath.Join(coverageDir, "coverage-final.json")
	if err := os.WriteFile(coveragePath, istanbulJSON, 0644); err != nil {
		t.Fatalf("failed to write coverage-final.json: %v", err)
	}

	// The legacy (single-component) process path expects workspace/repo.
	workspace := t.TempDir()
	repoDir := filepath.Join(workspace, "repo")
	if err := os.MkdirAll(repoDir, 0755); err != nil {
		t.Fatalf("failed to create repo dir: %v", err)
	}

	cmd := exec.Command(coverportBin, "process",
		"--coverage-dir", coverageDir,
		"--format", "nyc",
		"--upload=false",
		"--skip-clone",
		"--keep-workspace",
		"--workspace", workspace,
		"--repo-url", "https://github.com/konflux-ci/coverport",
		"--commit-sha", "abc123def456789",
	)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	cmd.Env = os.Environ()
	if err := cmd.Run(); err != nil {
		t.Fatalf("coverport process --format nyc failed: %v\nstdout: %s\nstderr: %s",
			err, stdout.String(), stderr.String())
	}
	t.Logf("process stdout:\n%s", stdout.String())

	lcovPath := filepath.Join(workspace, "coverage.lcov")
	info, err := os.Stat(lcovPath)
	if err != nil || info.Size() == 0 {
		t.Fatal("coverage.lcov missing or empty")
	}
	t.Logf("output: %s (%d bytes)", lcovPath, info.Size())
}
