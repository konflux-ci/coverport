package snapshot

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseSnapshot(t *testing.T) {
	tests := []struct {
		name        string
		json        string
		wantCount   int
		wantError   bool
		wantCompName string
	}{
		{
			name:         "valid single component",
			json:         `{"components":[{"name":"app","containerImage":"quay.io/user/app@sha256:abc123"}]}`,
			wantCount:    1,
			wantCompName: "app",
		},
		{
			name:      "valid multiple components",
			json:      `{"components":[{"name":"frontend","containerImage":"quay.io/user/fe:v1"},{"name":"backend","containerImage":"quay.io/user/be:v1"}]}`,
			wantCount: 2,
		},
		{
			name:      "empty components",
			json:      `{"components":[]}`,
			wantCount: 0,
		},
		{
			name:      "invalid JSON",
			json:      `{invalid}`,
			wantError: true,
		},
		{
			name:      "empty string",
			json:      ``,
			wantError: true,
		},
		{
			name:      "with source info",
			json:      `{"components":[{"name":"app","containerImage":"quay.io/user/app:v1","source":{"git":{"url":"https://github.com/org/repo","revision":"abc123"}}}]}`,
			wantCount: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			snap, err := ParseSnapshot(tt.json)
			if tt.wantError {
				if err == nil {
					t.Error("expected error but got none")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(snap.Components) != tt.wantCount {
				t.Errorf("got %d components, want %d", len(snap.Components), tt.wantCount)
			}
			if tt.wantCompName != "" && len(snap.Components) > 0 {
				if snap.Components[0].Name != tt.wantCompName {
					t.Errorf("got component name %q, want %q", snap.Components[0].Name, tt.wantCompName)
				}
			}
		})
	}
}

func TestParseSnapshotFromFile(t *testing.T) {
	t.Run("valid file", func(t *testing.T) {
		tmpDir := t.TempDir()
		filePath := filepath.Join(tmpDir, "snapshot.json")
		content := `{"components":[{"name":"test-app","containerImage":"quay.io/user/app:v1"}]}`
		if err := os.WriteFile(filePath, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}

		snap, err := ParseSnapshotFromFile(filePath)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(snap.Components) != 1 {
			t.Errorf("got %d components, want 1", len(snap.Components))
		}
		if snap.Components[0].Name != "test-app" {
			t.Errorf("got component name %q, want %q", snap.Components[0].Name, "test-app")
		}
	})

	t.Run("nonexistent file", func(t *testing.T) {
		_, err := ParseSnapshotFromFile("/nonexistent/path/snapshot.json")
		if err == nil {
			t.Error("expected error for nonexistent file")
		}
	})
}

func TestGetImages(t *testing.T) {
	snap := &Snapshot{
		Components: []Component{
			{Name: "fe", ContainerImage: "quay.io/user/fe:v1"},
			{Name: "be", ContainerImage: "quay.io/user/be:v2"},
			{Name: "db", ContainerImage: "quay.io/user/db@sha256:abc"},
		},
	}

	images := snap.GetImages()
	if len(images) != 3 {
		t.Fatalf("got %d images, want 3", len(images))
	}

	expected := []string{
		"quay.io/user/fe:v1",
		"quay.io/user/be:v2",
		"quay.io/user/db@sha256:abc",
	}
	for i, img := range images {
		if img != expected[i] {
			t.Errorf("images[%d] = %q, want %q", i, img, expected[i])
		}
	}
}

func TestGetComponentByImage(t *testing.T) {
	snap := &Snapshot{
		Components: []Component{
			{Name: "frontend", ContainerImage: "quay.io/user/fe:v1"},
			{Name: "backend", ContainerImage: "quay.io/user/be:v2"},
		},
	}

	t.Run("found", func(t *testing.T) {
		comp := snap.GetComponentByImage("quay.io/user/be:v2")
		if comp == nil {
			t.Fatal("expected component but got nil")
		}
		if comp.Name != "backend" {
			t.Errorf("got component %q, want %q", comp.Name, "backend")
		}
	})

	t.Run("not found", func(t *testing.T) {
		comp := snap.GetComponentByImage("quay.io/user/nonexistent:v1")
		if comp != nil {
			t.Errorf("expected nil but got component %q", comp.Name)
		}
	})
}
