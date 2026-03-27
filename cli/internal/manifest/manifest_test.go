package manifest

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNewCollectionManifest(t *testing.T) {
	params := CollectionParameters{
		CoveragePort: 9095,
		Filters:      []string{"coverage_server.go"},
		Format:       "go",
		Namespace:    "test-ns",
	}

	m := NewCollectionManifest("my-test", params)

	if m.Version != Version {
		t.Errorf("got version %q, want %q", m.Version, Version)
	}
	if m.TestName != "my-test" {
		t.Errorf("got test name %q, want %q", m.TestName, "my-test")
	}
	if m.CollectedAt == "" {
		t.Error("CollectedAt should not be empty")
	}
	if len(m.Components) != 0 {
		t.Errorf("got %d components, want 0", len(m.Components))
	}
	if m.CollectionParams.CoveragePort != 9095 {
		t.Errorf("got port %d, want 9095", m.CollectionParams.CoveragePort)
	}
}

func TestAddComponent(t *testing.T) {
	m := NewCollectionManifest("test", CollectionParameters{})

	m.AddComponent(ComponentInfo{Name: "comp1", Image: "img1", CollectedAt: "now"})
	m.AddComponent(ComponentInfo{Name: "comp2", Image: "img2", CollectedAt: "now"})

	if len(m.Components) != 2 {
		t.Fatalf("got %d components, want 2", len(m.Components))
	}
	if m.Components[0].Name != "comp1" {
		t.Errorf("first component name = %q, want %q", m.Components[0].Name, "comp1")
	}
	if m.Components[1].Name != "comp2" {
		t.Errorf("second component name = %q, want %q", m.Components[1].Name, "comp2")
	}
}

func TestSaveAndLoad(t *testing.T) {
	tmpDir := t.TempDir()

	original := NewCollectionManifest("round-trip-test", CollectionParameters{
		CoveragePort: 8080,
		Filters:      []string{"test.go", "mock.go"},
		Format:       "go",
		Namespace:    "my-ns",
	})
	original.AddComponent(ComponentInfo{
		Name:          "frontend",
		Image:         "quay.io/org/fe:v1",
		CoverageDir:   "frontend/coverage-test",
		Namespace:     "my-ns",
		PodName:       "fe-pod-abc",
		ContainerName: "frontend",
		CollectedAt:   "2025-01-01T00:00:00Z",
	})
	original.AddComponent(ComponentInfo{
		Name:          "backend",
		Image:         "quay.io/org/be:v2",
		CoverageDir:   "backend/coverage-test",
		Namespace:     "my-ns",
		PodName:       "be-pod-xyz",
		ContainerName: "backend",
		CollectedAt:   "2025-01-01T00:00:01Z",
	})

	if err := original.Save(tmpDir); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	// Verify file exists
	manifestPath := filepath.Join(tmpDir, "metadata.json")
	if _, err := os.Stat(manifestPath); os.IsNotExist(err) {
		t.Fatal("metadata.json was not created")
	}

	// Load and verify
	loaded, err := Load(tmpDir)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if loaded.Version != original.Version {
		t.Errorf("version: got %q, want %q", loaded.Version, original.Version)
	}
	if loaded.TestName != original.TestName {
		t.Errorf("test name: got %q, want %q", loaded.TestName, original.TestName)
	}
	if len(loaded.Components) != len(original.Components) {
		t.Fatalf("components count: got %d, want %d", len(loaded.Components), len(original.Components))
	}
	if loaded.CollectionParams.CoveragePort != 8080 {
		t.Errorf("coverage port: got %d, want 8080", loaded.CollectionParams.CoveragePort)
	}
	if loaded.CollectionParams.Namespace != "my-ns" {
		t.Errorf("namespace: got %q, want %q", loaded.CollectionParams.Namespace, "my-ns")
	}

	for i, comp := range loaded.Components {
		orig := original.Components[i]
		if comp.Name != orig.Name {
			t.Errorf("component[%d] name: got %q, want %q", i, comp.Name, orig.Name)
		}
		if comp.Image != orig.Image {
			t.Errorf("component[%d] image: got %q, want %q", i, comp.Image, orig.Image)
		}
		if comp.CoverageDir != orig.CoverageDir {
			t.Errorf("component[%d] coverage dir: got %q, want %q", i, comp.CoverageDir, orig.CoverageDir)
		}
		if comp.PodName != orig.PodName {
			t.Errorf("component[%d] pod name: got %q, want %q", i, comp.PodName, orig.PodName)
		}
	}
}

func TestLoad_InvalidFile(t *testing.T) {
	tmpDir := t.TempDir()

	// Write invalid JSON
	manifestPath := filepath.Join(tmpDir, "metadata.json")
	if err := os.WriteFile(manifestPath, []byte("{invalid json}"), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := Load(tmpDir)
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestLoad_MissingFile(t *testing.T) {
	tmpDir := t.TempDir()

	_, err := Load(tmpDir)
	if err == nil {
		t.Error("expected error for missing file")
	}
}

func TestExists(t *testing.T) {
	tmpDir := t.TempDir()

	if Exists(tmpDir) {
		t.Error("should return false when no metadata.json exists")
	}

	manifestPath := filepath.Join(tmpDir, "metadata.json")
	if err := os.WriteFile(manifestPath, []byte("{}"), 0644); err != nil {
		t.Fatal(err)
	}

	if !Exists(tmpDir) {
		t.Error("should return true when metadata.json exists")
	}
}
