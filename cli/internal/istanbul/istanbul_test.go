package istanbul

import (
	"strings"
	"testing"
)

func TestDecode(t *testing.T) {
	const validLocation = `{"start":{"line":1,"column":0},"end":{"line":1,"column":1}}`

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
			name: "valid nested coverage",
			data: `{"/app/app.js":{` +
				`"statementMap":{"0":` + validLocation + `},` +
				`"fnMap":{"0":{"name":"main","decl":` + validLocation + `,"loc":` + validLocation + `,"line":1}},` +
				`"branchMap":{"0":{"type":"if","locations":[` + validLocation + `],"line":1}}` +
				`}}`,
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
		{
			name:      "statement start is null",
			data:      `{"/app/app.js":{"statementMap":{"0":{"start":null,"end":{"line":1,"column":1}}}}}`,
			wantError: "start position is null or missing",
		},
		{
			name:      "statement end is null",
			data:      `{"/app/app.js":{"statementMap":{"0":{"start":{"line":1,"column":0},"end":null}}}}`,
			wantError: "end position is null or missing",
		},
		{
			name:      "statement line is zero",
			data:      `{"/app/app.js":{"statementMap":{"0":{"start":{"line":0,"column":0},"end":{"line":1,"column":1}}}}}`,
			wantError: "line numbers must be positive",
		},
		{
			name:      "statement column is negative",
			data:      `{"/app/app.js":{"statementMap":{"0":{"start":{"line":1,"column":-1},"end":{"line":1,"column":1}}}}}`,
			wantError: "columns must be non-negative",
		},
		{
			name: "function declaration is null",
			data: `{"/app/app.js":{"fnMap":{"0":{` +
				`"name":"main","decl":null,"loc":{"start":{"line":1,"column":0},"end":{"line":1,"column":1}},"line":1` +
				`}}}}`,
			wantError: "function declaration location is null or missing",
		},
		{
			name: "function location is null",
			data: `{"/app/app.js":{"fnMap":{"0":{` +
				`"name":"main","decl":{"start":{"line":1,"column":0},"end":{"line":1,"column":1}},"loc":null,"line":1` +
				`}}}}`,
			wantError: "function location is null or missing",
		},
		{
			name: "function line is zero",
			data: `{"/app/app.js":{"fnMap":{"0":{` +
				`"name":"main","decl":` + validLocation + `,"loc":` + validLocation + `,"line":0` +
				`}}}}`,
			wantError: "function line number must be positive",
		},
		{
			name:      "branch locations are null",
			data:      `{"/app/app.js":{"branchMap":{"0":{"type":"if","locations":null,"line":1}}}}`,
			wantError: "branch locations are null or missing",
		},
		{
			name: "branch location is null",
			data: `{"/app/app.js":{"branchMap":{"0":{` +
				`"type":"if","locations":[null],"line":1` +
				`}}}}`,
			wantError: "branch location 0 is null",
		},
		{
			name:      "branch line is zero",
			data:      `{"/app/app.js":{"branchMap":{"0":{"type":"if","locations":[` + validLocation + `],"line":0}}}}`,
			wantError: "branch line number must be positive",
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
