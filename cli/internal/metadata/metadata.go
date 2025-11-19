package metadata

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
)

// GitMetadata contains git information extracted from container image attestation
type GitMetadata struct {
	RepoURL   string
	CommitSHA string
	Branch    string
	Tag       string
}

// ImageMetadataExtractor handles extracting metadata from container images
type ImageMetadataExtractor struct {
	cosignPath string
}

// NewImageMetadataExtractor creates a new metadata extractor
func NewImageMetadataExtractor() (*ImageMetadataExtractor, error) {
	// Check if cosign is available
	cosignPath, err := exec.LookPath("cosign")
	if err != nil {
		return nil, fmt.Errorf("cosign not found in PATH (required for extracting git metadata): %w", err)
	}

	return &ImageMetadataExtractor{
		cosignPath: cosignPath,
	}, nil
}

// ExtractGitMetadata extracts git metadata from a container image using cosign
func (e *ImageMetadataExtractor) ExtractGitMetadata(ctx context.Context, image string) (*GitMetadata, error) {
	fmt.Printf("🔍 Extracting git metadata from image: %s\n", image)

	// Download attestation using cosign
	cmd := exec.CommandContext(ctx, e.cosignPath, "download", "attestation", image)
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to download attestation: %w (output: %s)", err, output)
	}

	// Parse the attestation JSON - handle both single object and array
	var attestation map[string]interface{}
	
	// Try to parse as array first
	var attestations []map[string]interface{}
	if err := json.Unmarshal(output, &attestations); err == nil && len(attestations) > 0 {
		// It's an array, use the first one
		attestation = attestations[0]
	} else {
		// Try to parse as single object
		if err := json.Unmarshal(output, &attestation); err != nil {
			return nil, fmt.Errorf("failed to parse attestation JSON (tried both array and object): %w", err)
		}
	}

	if attestation == nil || len(attestation) == 0 {
		return nil, fmt.Errorf("no attestation data found for image")
	}
	payloadStr, ok := attestation["payload"].(string)
	if !ok {
		return nil, fmt.Errorf("attestation payload not found or invalid")
	}

	// Parse the payload
	var payload map[string]interface{}
	if err := json.Unmarshal([]byte(payloadStr), &payload); err != nil {
		// Try base64 decoding first
		cmd := exec.Command("base64", "-d")
		cmd.Stdin = strings.NewReader(payloadStr)
		decoded, err := cmd.Output()
		if err != nil {
			return nil, fmt.Errorf("failed to decode payload: %w", err)
		}
		if err := json.Unmarshal(decoded, &payload); err != nil {
			return nil, fmt.Errorf("failed to parse decoded payload: %w", err)
		}
	}

	// Navigate to the annotations in the Konflux attestation structure
	// Path: predicate.buildConfig.tasks[0].invocation.environment.annotations
	predicate, ok := payload["predicate"].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("predicate not found in attestation")
	}

	buildConfig, ok := predicate["buildConfig"].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("buildConfig not found in predicate")
	}

	tasks, ok := buildConfig["tasks"].([]interface{})
	if !ok || len(tasks) == 0 {
		return nil, fmt.Errorf("tasks not found or empty in buildConfig")
	}

	firstTask, ok := tasks[0].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("first task is invalid")
	}

	invocation, ok := firstTask["invocation"].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("invocation not found in task")
	}

	environment, ok := invocation["environment"].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("environment not found in invocation")
	}

	annotations, ok := environment["annotations"].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("annotations not found in environment")
	}

	// Extract git information from Konflux annotations
	metadata := &GitMetadata{}

	// Repository URL
	if repoURL, ok := annotations["pipelinesascode.tekton.dev/repo-url"].(string); ok {
		metadata.RepoURL = repoURL
	} else {
		return nil, fmt.Errorf("repo-url not found in annotations")
	}

	// Commit SHA
	if commitSHA, ok := annotations["build.appstudio.redhat.com/commit_sha"].(string); ok {
		metadata.CommitSHA = commitSHA
	} else {
		return nil, fmt.Errorf("commit_sha not found in annotations")
	}

	// Optional: Branch and Tag
	if branch, ok := annotations["pipelinesascode.tekton.dev/source_branch"].(string); ok {
		metadata.Branch = branch
	}
	if tag, ok := annotations["pipelinesascode.tekton.dev/tag"].(string); ok {
		metadata.Tag = tag
	}

	fmt.Printf("✅ Extracted git metadata:\n")
	fmt.Printf("   Repository: %s\n", metadata.RepoURL)
	fmt.Printf("   Commit: %s\n", metadata.CommitSHA)
	if metadata.Branch != "" {
		fmt.Printf("   Branch: %s\n", metadata.Branch)
	}
	if metadata.Tag != "" {
		fmt.Printf("   Tag: %s\n", metadata.Tag)
	}

	return metadata, nil
}

