// Package istanbul defines the coverage data exchanged between the Node.js
// collector and the NYC processor.
package istanbul

import (
	"encoding/json"
	"fmt"
)

// FileCoverage represents Istanbul coverage data for a single file.
type FileCoverage struct {
	Path           string                   `json:"path"`
	StatementMap   map[string]*Location     `json:"statementMap"`
	FnMap          map[string]*FunctionInfo `json:"fnMap"`
	BranchMap      map[string]*BranchInfo   `json:"branchMap"`
	S              map[string]int           `json:"s"`
	F              map[string]int           `json:"f"`
	B              map[string][]int         `json:"b"`
	InputSourceMap json.RawMessage          `json:"inputSourceMap,omitempty"`
}

// Location represents an Istanbul source location.
type Location struct {
	Start Position `json:"start"`
	End   Position `json:"end"`
}

// Position represents a line and column in a source file.
type Position struct {
	Line   int `json:"line"`
	Column int `json:"column"`
}

// FunctionInfo represents Istanbul function coverage metadata.
type FunctionInfo struct {
	Name string   `json:"name"`
	Decl Location `json:"decl"`
	Loc  Location `json:"loc"`
	Line int      `json:"line"`
}

// BranchInfo represents Istanbul branch coverage metadata.
type BranchInfo struct {
	Type      string     `json:"type"`
	Locations []Location `json:"locations"`
	Line      int        `json:"line"`
}

// CoverageData maps source file paths to their coverage data.
type CoverageData map[string]*FileCoverage

// Decode parses Istanbul coverage data and rejects null file or nested map
// entries that encoding/json otherwise accepts for pointer values.
func Decode(data []byte) (CoverageData, error) {
	var coverageData CoverageData
	if err := json.Unmarshal(data, &coverageData); err != nil {
		return nil, err
	}

	for filePath, fileCoverage := range coverageData {
		if fileCoverage == nil {
			return nil, fmt.Errorf("file entry %q is null", filePath)
		}
		for entryID, location := range fileCoverage.StatementMap {
			if location == nil {
				return nil, fmt.Errorf("statementMap entry %q for %q is null", entryID, filePath)
			}
		}
		for entryID, function := range fileCoverage.FnMap {
			if function == nil {
				return nil, fmt.Errorf("fnMap entry %q for %q is null", entryID, filePath)
			}
		}
		for entryID, branch := range fileCoverage.BranchMap {
			if branch == nil {
				return nil, fmt.Errorf("branchMap entry %q for %q is null", entryID, filePath)
			}
		}
	}

	return coverageData, nil
}
