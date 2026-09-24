package istanbul

import (
	"strings"
	"testing"
)

func TestDecode(t *testing.T) {
	tests := []struct {
		name      string
		data      string
		wantError string
	}{
		{
			name: "valid coverage",
			data: `{` +
				`"/app/app.js":{"path":"/app/app.js","statementMap":{},"fnMap":{},"branchMap":{},"s":{},"f":{},"b":{}}` +
				`}`,
		},
		{
			name:      "statement entry is array",
			data:      `{"/app/app.js":{"statementMap":{"0":[]}}}`,
			wantError: "cannot unmarshal array",
		},
		{
			name:      "function entry is scalar",
			data:      `{"/app/app.js":{"fnMap":{"0":42}}}`,
			wantError: "cannot unmarshal number",
		},
		{
			name:      "branch entry is null",
			data:      `{"/app/app.js":{"branchMap":{"0":null}}}`,
			wantError: `branchMap entry "0"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			coverage, err := Decode([]byte(tt.data))
			if tt.wantError != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantError) {
					t.Fatalf("expected error containing %q, got %v", tt.wantError, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(coverage) != 1 {
				t.Fatalf("got %d files, want 1", len(coverage))
			}
		})
	}
}
