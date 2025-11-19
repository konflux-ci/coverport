package manifest

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// Version of the manifest format
const Version = "1.0"

// CollectionManifest represents the top-level metadata for collected coverage
type CollectionManifest struct {
	Version          string               `json:"version"`
	TestName         string               `json:"test_name"`
	CollectedAt      string               `json:"collected_at"`
	CollectionParams CollectionParameters `json:"collection_params"`
	Components       []ComponentInfo      `json:"components"`
}

// CollectionParameters stores the parameters used during collection
type CollectionParameters struct {
	CoveragePort int      `json:"coverage_port,omitempty"`
	Filters      []string `json:"filters,omitempty"`
	Format       string   `json:"format,omitempty"`
	Namespace    string   `json:"namespace,omitempty"`
}

// ComponentInfo represents metadata for a single component
type ComponentInfo struct {
	Name          string `json:"name"`
	Image         string `json:"image"`
	CoverageDir   string `json:"coverage_dir"`
	Namespace     string `json:"namespace,omitempty"`
	PodName       string `json:"pod_name,omitempty"`
	ContainerName string `json:"container_name,omitempty"`
	CollectedAt   string `json:"collected_at"`
}

// NewCollectionManifest creates a new collection manifest
func NewCollectionManifest(testName string, params CollectionParameters) *CollectionManifest {
	return &CollectionManifest{
		Version:          Version,
		TestName:         testName,
		CollectedAt:      time.Now().Format(time.RFC3339),
		CollectionParams: params,
		Components:       []ComponentInfo{},
	}
}

// AddComponent adds a component to the manifest
func (m *CollectionManifest) AddComponent(component ComponentInfo) {
	m.Components = append(m.Components, component)
}

// Save writes the manifest to a file
func (m *CollectionManifest) Save(outputDir string) error {
	manifestPath := filepath.Join(outputDir, "metadata.json")
	
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal manifest: %w", err)
	}
	
	if err := os.WriteFile(manifestPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write manifest: %w", err)
	}
	
	fmt.Printf("📄 Collection manifest saved: %s\n", manifestPath)
	return nil
}

// Load reads a manifest from a file
func Load(coverageDir string) (*CollectionManifest, error) {
	manifestPath := filepath.Join(coverageDir, "metadata.json")
	
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read manifest: %w", err)
	}
	
	var manifest CollectionManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return nil, fmt.Errorf("failed to parse manifest: %w", err)
	}
	
	return &manifest, nil
}

// Exists checks if a manifest file exists in the given directory
func Exists(coverageDir string) bool {
	manifestPath := filepath.Join(coverageDir, "metadata.json")
	_, err := os.Stat(manifestPath)
	return err == nil
}

