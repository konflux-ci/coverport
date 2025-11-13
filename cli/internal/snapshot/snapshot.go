package snapshot

import (
	"encoding/json"
	"fmt"
	"os"
)

// Snapshot represents a Konflux/Tekton snapshot containing component information
type Snapshot struct {
	Components []Component `json:"components"`
}

// Component represents a component in the snapshot
type Component struct {
	Name           string `json:"name"`
	ContainerImage string `json:"containerImage"`
	Source         Source `json:"source,omitempty"`
}

// Source represents the source information for a component
type Source struct {
	Git GitSource `json:"git,omitempty"`
}

// GitSource represents git source information
type GitSource struct {
	URL      string `json:"url,omitempty"`
	Revision string `json:"revision,omitempty"`
}

// ParseSnapshot parses a snapshot from JSON string
func ParseSnapshot(snapshotJSON string) (*Snapshot, error) {
	var snapshot Snapshot
	if err := json.Unmarshal([]byte(snapshotJSON), &snapshot); err != nil {
		return nil, fmt.Errorf("parse snapshot JSON: %w", err)
	}
	return &snapshot, nil
}

// ParseSnapshotFromFile parses a snapshot from a file
func ParseSnapshotFromFile(filepath string) (*Snapshot, error) {
	data, err := os.ReadFile(filepath)
	if err != nil {
		return nil, fmt.Errorf("read snapshot file: %w", err)
	}
	return ParseSnapshot(string(data))
}

// GetImages returns all container images from the snapshot
func (s *Snapshot) GetImages() []string {
	images := make([]string, len(s.Components))
	for i, comp := range s.Components {
		images[i] = comp.ContainerImage
	}
	return images
}

// GetComponentByImage finds a component by its container image
func (s *Snapshot) GetComponentByImage(image string) *Component {
	for i := range s.Components {
		if s.Components[i].ContainerImage == image {
			return &s.Components[i]
		}
	}
	return nil
}

