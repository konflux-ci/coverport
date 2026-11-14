package coverageclient

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestSanitizeFilename verifies that server-provided filenames cannot be used
// to escape the target directory via path traversal. See issue #96.
func TestSanitizeFilename(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{name: "plain filename", input: "coverage.profraw", want: "coverage.profraw"},
		{name: "dotted filename", input: "meta.data.json", want: "meta.data.json"},
		{name: "relative traversal", input: "../../etc/cron.d/backdoor", want: "backdoor"},
		{name: "absolute path", input: "/etc/passwd", want: "passwd"},
		{name: "nested path separators", input: "foo/bar/baz.txt", want: "baz.txt"},
		{name: "trailing separator", input: "report/", want: "report"},
		{name: "hidden file is preserved", input: ".bashrc", want: ".bashrc"},
		{name: "leading dots are not traversal", input: "..data.json", want: "..data.json"},
		{name: "single traversal", input: "..", wantErr: true},
		{name: "traversal with trailing separator", input: "../", wantErr: true},
		{name: "deep traversal", input: "../../..", wantErr: true},
		{name: "current dir", input: ".", wantErr: true},
		{name: "separator only", input: "/", wantErr: true},
		{name: "repeated separators only", input: "///", wantErr: true},
		{name: "empty string", input: "", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := sanitizeFilename(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("sanitizeFilename(%q) = %q, want error", tt.input, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("sanitizeFilename(%q) unexpected error: %v", tt.input, err)
			}
			if got != tt.want {
				t.Errorf("sanitizeFilename(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

// serveJSON starts a test server that answers every request with v as JSON.
func serveJSON(t *testing.T, v any) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(v)
	}))
	t.Cleanup(server.Close)
	return server
}

// TestCollectCoverage_RejectsUnusableFilenames verifies that a server-provided
// filename which does not reduce to a usable path element aborts collection.
func TestCollectCoverage_RejectsUnusableFilenames(t *testing.T) {
	data := base64.StdEncoding.EncodeToString([]byte("data"))

	tests := []struct {
		name     string
		response any
		wantErr  string
	}{
		{
			name: "rust profraw",
			response: RustCoverageResponse{
				ProfrawFilename: "..",
				ProfrawData:     data,
				CoverageEnabled: true,
			},
			wantErr: "sanitize profraw filename",
		},
		{
			name: "go metadata",
			response: CoverageResponse{
				MetaFilename:     "..",
				MetaData:         data,
				CountersFilename: "covcounters.test",
				CountersData:     data,
			},
			wantErr: "sanitize metadata filename",
		},
		{
			name: "go counters",
			response: CoverageResponse{
				MetaFilename:     "covmeta.test",
				MetaData:         data,
				CountersFilename: "/",
				CountersData:     data,
			},
			wantErr: "sanitize counters filename",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := serveJSON(t, tt.response)
			client := &CoverageClient{
				outputDir:  t.TempDir(),
				httpClient: &http.Client{Timeout: 10 * time.Second},
			}

			err := client.CollectCoverageFromURL(server.URL, "sec-test")
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("expected %q error, got: %v", tt.wantErr, err)
			}
		})
	}
}

// TestCollectCoverage_TraversalStaysInTestDir verifies that a traversal name
// from the server is written inside the test directory, not outside it.
func TestCollectCoverage_TraversalStaysInTestDir(t *testing.T) {
	data := base64.StdEncoding.EncodeToString([]byte("data"))
	server := serveJSON(t, CoverageResponse{
		MetaFilename:     "../../covmeta.test",
		MetaData:         data,
		CountersFilename: "../covcounters.test",
		CountersData:     data,
	})

	outputDir := t.TempDir()
	client := &CoverageClient{
		outputDir:  outputDir,
		httpClient: &http.Client{Timeout: 10 * time.Second},
	}

	if err := client.CollectCoverageFromURL(server.URL, "sec-test"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	for _, name := range []string{"covmeta.test", "covcounters.test"} {
		if _, err := os.Stat(filepath.Join(outputDir, "sec-test", name)); err != nil {
			t.Errorf("%s not written inside the test directory: %v", name, err)
		}
		if _, err := os.Stat(filepath.Join(outputDir, name)); err == nil {
			t.Errorf("%s escaped the test directory", name)
		}
	}
}
