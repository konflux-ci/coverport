package coverageclient

import "testing"

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
