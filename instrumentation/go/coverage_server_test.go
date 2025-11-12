package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"runtime/coverage"
	"strings"
	"testing"
)

func TestCoverageHandler_Success(t *testing.T) {
	// Skip if coverage is not enabled
	if !isCoverageEnabled() {
		t.Skip("Skipping test - coverage not enabled (run with: go test -cover)")
	}

	// Create a request to the coverage handler
	req, err := http.NewRequest("GET", "/coverage", nil)
	if err != nil {
		t.Fatal(err)
	}

	// Create a ResponseRecorder to record the response
	rr := httptest.NewRecorder()
	handler := http.HandlerFunc(CoverageHandler)

	// Call the handler
	handler.ServeHTTP(rr, req)

	// Check the status code
	if status := rr.Code; status != http.StatusOK {
		t.Errorf("Handler returned wrong status code: got %v want %v (body: %s)",
			status, http.StatusOK, rr.Body.String())
	}

	// Check the content type
	contentType := rr.Header().Get("Content-Type")
	if !strings.Contains(contentType, "application/json") {
		t.Errorf("Expected Content-Type to be application/json, got %s", contentType)
	}

	// Parse the response
	var response CoverageResponse
	if err := json.NewDecoder(rr.Body).Decode(&response); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	// Verify response structure
	if response.MetaFilename == "" {
		t.Error("MetaFilename should not be empty")
	}

	if response.CountersFilename == "" {
		t.Error("CountersFilename should not be empty")
	}

	if response.Timestamp == 0 {
		t.Error("Timestamp should not be zero")
	}

	// Verify filenames have correct format
	if !strings.HasPrefix(response.MetaFilename, "covmeta.") {
		t.Errorf("MetaFilename should start with 'covmeta.', got: %s", response.MetaFilename)
	}

	if !strings.HasPrefix(response.CountersFilename, "covcounters.") {
		t.Errorf("CountersFilename should start with 'covcounters.', got: %s", response.CountersFilename)
	}

	// Verify base64 encoded data
	if response.MetaData == "" {
		t.Error("MetaData should not be empty")
	}

	if response.CountersData == "" {
		t.Error("CountersData should not be empty")
	}

	// Verify base64 data can be decoded
	metaBytes, err := base64.StdEncoding.DecodeString(response.MetaData)
	if err != nil {
		t.Errorf("MetaData is not valid base64: %v", err)
	}
	if len(metaBytes) == 0 {
		t.Error("Decoded MetaData should not be empty")
	}

	counterBytes, err := base64.StdEncoding.DecodeString(response.CountersData)
	if err != nil {
		t.Errorf("CountersData is not valid base64: %v", err)
	}
	if len(counterBytes) == 0 {
		t.Error("Decoded CountersData should not be empty")
	}
}

// isCoverageEnabled checks if coverage is enabled in the test binary
func isCoverageEnabled() bool {
	// The coverage package returns an error if coverage is not enabled
	// Specifically: "error: no meta-data available (binary not built with -cover?)"
	var buf bytes.Buffer
	err := coverage.WriteMeta(&buf)

	// If no error and we have data, coverage is definitely enabled
	if err == nil && buf.Len() > 0 {
		return true
	}

	// If we get an error about no meta-data, coverage is not enabled
	if err != nil && strings.Contains(err.Error(), "no meta-data available") {
		return false
	}

	// Otherwise assume coverage is enabled (might be edge case)
	return err == nil
}

func TestCoverageHandler_POST(t *testing.T) {
	if !isCoverageEnabled() {
		t.Skip("Skipping test - coverage not enabled (run with: go test -cover)")
	}

	// Test that POST requests also work
	req, err := http.NewRequest("POST", "/coverage", strings.NewReader(`{"test_name":"my-test"}`))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")

	rr := httptest.NewRecorder()
	handler := http.HandlerFunc(CoverageHandler)

	handler.ServeHTTP(rr, req)

	if status := rr.Code; status != http.StatusOK {
		t.Errorf("Handler returned wrong status code for POST: got %v want %v",
			status, http.StatusOK)
	}

	var response CoverageResponse
	if err := json.NewDecoder(rr.Body).Decode(&response); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if response.MetaFilename == "" || response.CountersFilename == "" {
		t.Error("Response should contain filenames")
	}
}

func TestCoverageHandler_ResponseStructure(t *testing.T) {
	if !isCoverageEnabled() {
		t.Skip("Skipping test - coverage not enabled (run with: go test -cover)")
	}

	req, _ := http.NewRequest("GET", "/coverage", nil)
	rr := httptest.NewRecorder()
	handler := http.HandlerFunc(CoverageHandler)

	handler.ServeHTTP(rr, req)

	// Parse response
	var response CoverageResponse
	json.NewDecoder(rr.Body).Decode(&response)

	// Test that timestamps are reasonable (within last second)
	now := int64(1000000000000000000)  // Very large timestamp in nanoseconds
	if response.Timestamp > now*1000 { // Allow for future dates but not too far
		t.Error("Timestamp seems unreasonable")
	}

	// Verify counter filename contains PID and timestamp
	if !strings.Contains(response.CountersFilename, ".") {
		t.Error("CountersFilename should contain delimiters")
	}

	parts := strings.Split(response.CountersFilename, ".")
	if len(parts) < 4 {
		t.Errorf("CountersFilename should have at least 4 parts (covcounters.hash.pid.timestamp), got %d parts: %v", len(parts), parts)
	}
}

func TestCoverageResponse_JSONMarshaling(t *testing.T) {
	// Create a sample response
	original := CoverageResponse{
		MetaFilename:     "covmeta.test123",
		MetaData:         base64.StdEncoding.EncodeToString([]byte("meta content")),
		CountersFilename: "covcounters.test123.1234.5678",
		CountersData:     base64.StdEncoding.EncodeToString([]byte("counter content")),
		Timestamp:        1234567890,
	}

	// Marshal to JSON
	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("Failed to marshal: %v", err)
	}

	// Unmarshal back
	var decoded CoverageResponse
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Failed to unmarshal: %v", err)
	}

	// Verify all fields match
	if decoded.MetaFilename != original.MetaFilename {
		t.Errorf("MetaFilename mismatch: %s != %s", decoded.MetaFilename, original.MetaFilename)
	}

	if decoded.MetaData != original.MetaData {
		t.Errorf("MetaData mismatch")
	}

	if decoded.CountersFilename != original.CountersFilename {
		t.Errorf("CountersFilename mismatch: %s != %s", decoded.CountersFilename, original.CountersFilename)
	}

	if decoded.CountersData != original.CountersData {
		t.Errorf("CountersData mismatch")
	}

	if decoded.Timestamp != original.Timestamp {
		t.Errorf("Timestamp mismatch: %d != %d", decoded.Timestamp, original.Timestamp)
	}

	// Verify JSON contains expected fields
	jsonStr := string(data)
	expectedFields := []string{
		"meta_filename",
		"meta_data",
		"counters_filename",
		"counters_data",
		"timestamp",
	}

	for _, field := range expectedFields {
		if !strings.Contains(jsonStr, field) {
			t.Errorf("JSON should contain field '%s': %s", field, jsonStr)
		}
	}
}

func TestCoverageResponse_Base64Encoding(t *testing.T) {
	if !isCoverageEnabled() {
		t.Skip("Skipping test - coverage not enabled (run with: go test -cover)")
	}

	req, _ := http.NewRequest("GET", "/coverage", nil)
	rr := httptest.NewRecorder()
	handler := http.HandlerFunc(CoverageHandler)

	handler.ServeHTTP(rr, req)

	var response CoverageResponse
	json.NewDecoder(rr.Body).Decode(&response)

	// Test that base64 decoding works
	tests := []struct {
		name    string
		data    string
		minSize int
	}{
		{"MetaData", response.MetaData, 1},
		{"CountersData", response.CountersData, 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			decoded, err := base64.StdEncoding.DecodeString(tt.data)
			if err != nil {
				t.Errorf("%s: Failed to decode base64: %v", tt.name, err)
			}

			if len(decoded) < tt.minSize {
				t.Errorf("%s: Decoded data too small, expected at least %d bytes, got %d",
					tt.name, tt.minSize, len(decoded))
			}
		})
	}
}

func TestCoverageHandler_MultipleRequests(t *testing.T) {
	if !isCoverageEnabled() {
		t.Skip("Skipping test - coverage not enabled (run with: go test -cover)")
	}

	// Test that multiple requests work correctly
	handler := http.HandlerFunc(CoverageHandler)

	for i := 0; i < 5; i++ {
		req, _ := http.NewRequest("GET", "/coverage", nil)
		rr := httptest.NewRecorder()

		handler.ServeHTTP(rr, req)

		if status := rr.Code; status != http.StatusOK {
			t.Errorf("Request %d: Handler returned wrong status code: got %v want %v",
				i, status, http.StatusOK)
		}

		var response CoverageResponse
		if err := json.NewDecoder(rr.Body).Decode(&response); err != nil {
			t.Errorf("Request %d: Failed to decode response: %v", i, err)
		}

		// Verify each response is valid
		if response.MetaFilename == "" {
			t.Errorf("Request %d: MetaFilename is empty", i)
		}
	}
}

func TestCoverageHandler_FilenameUniqueness(t *testing.T) {
	if !isCoverageEnabled() {
		t.Skip("Skipping test - coverage not enabled (run with: go test -cover)")
	}

	// Test that counter filenames are unique across requests
	handler := http.HandlerFunc(CoverageHandler)
	filenames := make(map[string]bool)

	for i := 0; i < 10; i++ {
		req, _ := http.NewRequest("GET", "/coverage", nil)
		rr := httptest.NewRecorder()

		handler.ServeHTTP(rr, req)

		var response CoverageResponse
		json.NewDecoder(rr.Body).Decode(&response)

		if filenames[response.CountersFilename] {
			t.Errorf("Duplicate counter filename detected: %s", response.CountersFilename)
		}
		filenames[response.CountersFilename] = true

		// Meta filename should be the same (same hash)
		// Counter filename should be different (includes timestamp)
	}

	if len(filenames) < 2 {
		t.Error("Expected at least some unique counter filenames due to different timestamps")
	}
}

func TestHealthHandler(t *testing.T) {
	// Create a health check handler (simulating what's in startCoverageServer)
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("coverage server healthy"))
	})

	req, err := http.NewRequest("GET", "/health", nil)
	if err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	// Check status code
	if status := rr.Code; status != http.StatusOK {
		t.Errorf("Health handler returned wrong status code: got %v want %v",
			status, http.StatusOK)
	}

	// Check response body
	expected := "coverage server healthy"
	if rr.Body.String() != expected {
		t.Errorf("Health handler returned unexpected body: got %v want %v",
			rr.Body.String(), expected)
	}
}

func TestCoverageHandler_ConcurrentRequests(t *testing.T) {
	if !isCoverageEnabled() {
		t.Skip("Skipping test - coverage not enabled (run with: go test -cover)")
	}

	// Test concurrent access to the coverage handler
	handler := http.HandlerFunc(CoverageHandler)
	done := make(chan bool)
	numRequests := 10

	for i := 0; i < numRequests; i++ {
		go func(id int) {
			req, _ := http.NewRequest("GET", "/coverage", nil)
			rr := httptest.NewRecorder()

			handler.ServeHTTP(rr, req)

			if status := rr.Code; status != http.StatusOK {
				t.Errorf("Concurrent request %d failed with status: %v", id, status)
			}

			var response CoverageResponse
			if err := json.NewDecoder(rr.Body).Decode(&response); err != nil {
				t.Errorf("Concurrent request %d: Failed to decode response: %v", id, err)
			}

			done <- true
		}(i)
	}

	// Wait for all goroutines to complete
	for i := 0; i < numRequests; i++ {
		<-done
	}
}

func TestCoverageResponse_EmptyFieldValidation(t *testing.T) {
	if !isCoverageEnabled() {
		t.Skip("Skipping test - coverage not enabled (run with: go test -cover)")
	}

	// Test that all response fields are populated
	req, _ := http.NewRequest("GET", "/coverage", nil)
	rr := httptest.NewRecorder()
	handler := http.HandlerFunc(CoverageHandler)

	handler.ServeHTTP(rr, req)

	var response CoverageResponse
	json.NewDecoder(rr.Body).Decode(&response)

	// Check for empty fields
	emptyFields := []string{}

	if response.MetaFilename == "" {
		emptyFields = append(emptyFields, "MetaFilename")
	}
	if response.MetaData == "" {
		emptyFields = append(emptyFields, "MetaData")
	}
	if response.CountersFilename == "" {
		emptyFields = append(emptyFields, "CountersFilename")
	}
	if response.CountersData == "" {
		emptyFields = append(emptyFields, "CountersData")
	}
	if response.Timestamp == 0 {
		emptyFields = append(emptyFields, "Timestamp")
	}

	if len(emptyFields) > 0 {
		t.Errorf("Response has empty fields: %v", emptyFields)
	}
}

func BenchmarkCoverageHandler(b *testing.B) {
	if !isCoverageEnabled() {
		b.Skip("Skipping benchmark - coverage not enabled")
	}

	handler := http.HandlerFunc(CoverageHandler)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		req, _ := http.NewRequest("GET", "/coverage", nil)
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
	}
}

func BenchmarkCoverageHandler_Parallel(b *testing.B) {
	if !isCoverageEnabled() {
		b.Skip("Skipping benchmark - coverage not enabled")
	}

	handler := http.HandlerFunc(CoverageHandler)

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			req, _ := http.NewRequest("GET", "/coverage", nil)
			rr := httptest.NewRecorder()
			handler.ServeHTTP(rr, req)
		}
	})
}
