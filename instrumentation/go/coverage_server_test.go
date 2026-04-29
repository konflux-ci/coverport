package coverage

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
	if !isCoverageEnabled() {
		t.Skip("Skipping test - coverage not enabled (run with: go test -cover)")
	}

	req, err := http.NewRequest("GET", "/coverage", nil)
	if err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	handler := http.HandlerFunc(CoverageHandler)

	handler.ServeHTTP(rr, req)

	if status := rr.Code; status != http.StatusOK {
		t.Errorf("Handler returned wrong status code: got %v want %v (body: %s)",
			status, http.StatusOK, rr.Body.String())
	}

	contentType := rr.Header().Get("Content-Type")
	if !strings.Contains(contentType, "application/json") {
		t.Errorf("Expected Content-Type to be application/json, got %s", contentType)
	}

	var response CoverageResponse
	if err := json.NewDecoder(rr.Body).Decode(&response); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if response.MetaFilename == "" {
		t.Error("MetaFilename should not be empty")
	}

	if response.CountersFilename == "" {
		t.Error("CountersFilename should not be empty")
	}

	if response.Timestamp == 0 {
		t.Error("Timestamp should not be zero")
	}

	if !strings.HasPrefix(response.MetaFilename, "covmeta.") {
		t.Errorf("MetaFilename should start with 'covmeta.', got: %s", response.MetaFilename)
	}

	if !strings.HasPrefix(response.CountersFilename, "covcounters.") {
		t.Errorf("CountersFilename should start with 'covcounters.', got: %s", response.CountersFilename)
	}

	if response.MetaData == "" {
		t.Error("MetaData should not be empty")
	}

	if response.CountersData == "" {
		t.Error("CountersData should not be empty")
	}

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

func isCoverageEnabled() bool {
	var buf bytes.Buffer
	err := coverage.WriteMeta(&buf)

	if err == nil && buf.Len() > 0 {
		return true
	}

	if err != nil && strings.Contains(err.Error(), "no meta-data available") {
		return false
	}

	return err == nil
}

func TestCoverageHandler_POST(t *testing.T) {
	if !isCoverageEnabled() {
		t.Skip("Skipping test - coverage not enabled (run with: go test -cover)")
	}

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

func TestCoverageHandler_NoMeta(t *testing.T) {
	if !isCoverageEnabled() {
		t.Skip("Skipping test - coverage not enabled (run with: go test -cover)")
	}

	// First request without nometa to prime the hash cache
	req1, _ := http.NewRequest("GET", "/coverage", nil)
	rr1 := httptest.NewRecorder()
	handler := http.HandlerFunc(CoverageHandler)
	handler.ServeHTTP(rr1, req1)

	var firstResp CoverageResponse
	json.NewDecoder(rr1.Body).Decode(&firstResp)

	// Extract hash from the first response's meta filename
	expectedHash := strings.TrimPrefix(firstResp.MetaFilename, "covmeta.")

	// Second request with ?nometa=1
	req2, _ := http.NewRequest("GET", "/coverage?nometa=1", nil)
	rr2 := httptest.NewRecorder()
	handler.ServeHTTP(rr2, req2)

	if rr2.Code != http.StatusOK {
		t.Errorf("nometa request returned %d, want 200", rr2.Code)
	}

	var noMetaResp CoverageResponse
	if err := json.NewDecoder(rr2.Body).Decode(&noMetaResp); err != nil {
		t.Fatalf("Failed to decode nometa response: %v", err)
	}

	if noMetaResp.MetaFilename != "" {
		t.Errorf("MetaFilename should be empty with nometa=1, got %q", noMetaResp.MetaFilename)
	}

	if noMetaResp.MetaData != "" {
		t.Errorf("MetaData should be empty with nometa=1, got %d chars", len(noMetaResp.MetaData))
	}

	if noMetaResp.CountersData == "" {
		t.Error("CountersData should still be populated with nometa=1")
	}

	if noMetaResp.CountersFilename == "" {
		t.Error("CountersFilename should still be populated with nometa=1")
	}

	// The counter filename should contain the same hash from the cached first request
	if !strings.Contains(noMetaResp.CountersFilename, expectedHash) {
		t.Errorf("CountersFilename should contain cached hash %q, got %q", expectedHash, noMetaResp.CountersFilename)
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

	var response CoverageResponse
	json.NewDecoder(rr.Body).Decode(&response)

	now := int64(1000000000000000000)
	if response.Timestamp > now*1000 {
		t.Error("Timestamp seems unreasonable")
	}

	if !strings.Contains(response.CountersFilename, ".") {
		t.Error("CountersFilename should contain delimiters")
	}

	parts := strings.Split(response.CountersFilename, ".")
	if len(parts) < 4 {
		t.Errorf("CountersFilename should have at least 4 parts (covcounters.hash.pid.timestamp), got %d parts: %v", len(parts), parts)
	}
}

func TestCoverageResponse_JSONMarshaling(t *testing.T) {
	original := CoverageResponse{
		MetaFilename:     "covmeta.test123",
		MetaData:         base64.StdEncoding.EncodeToString([]byte("meta content")),
		CountersFilename: "covcounters.test123.1234.5678",
		CountersData:     base64.StdEncoding.EncodeToString([]byte("counter content")),
		Timestamp:        1234567890,
	}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("Failed to marshal: %v", err)
	}

	var decoded CoverageResponse
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Failed to unmarshal: %v", err)
	}

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

		if response.MetaFilename == "" {
			t.Errorf("Request %d: MetaFilename is empty", i)
		}
	}
}

func TestCoverageHandler_FilenameUniqueness(t *testing.T) {
	if !isCoverageEnabled() {
		t.Skip("Skipping test - coverage not enabled (run with: go test -cover)")
	}

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
	}

	if len(filenames) < 2 {
		t.Error("Expected at least some unique counter filenames due to different timestamps")
	}
}

func TestHealthHandler(t *testing.T) {
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

	if status := rr.Code; status != http.StatusOK {
		t.Errorf("Health handler returned wrong status code: got %v want %v",
			status, http.StatusOK)
	}

	expected := "coverage server healthy"
	if rr.Body.String() != expected {
		t.Errorf("Health handler returned unexpected body: got %v want %v",
			rr.Body.String(), expected)
	}
}

func TestIdentityMiddleware(t *testing.T) {
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	handler := identityMiddleware(inner)

	req, _ := http.NewRequest("HEAD", "/health", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Header().Get("X-Art-Coverage-Server") != "1" {
		t.Error("X-Art-Coverage-Server header should be '1'")
	}

	if rr.Header().Get("X-Art-Coverage-Pid") == "" {
		t.Error("X-Art-Coverage-Pid header should not be empty")
	}

	if rr.Header().Get("X-Art-Coverage-Binary") == "" {
		t.Error("X-Art-Coverage-Binary header should not be empty")
	}
}

func TestIdentityMiddleware_GET(t *testing.T) {
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("ok"))
	})

	handler := identityMiddleware(inner)

	req, _ := http.NewRequest("GET", "/coverage", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Header().Get("X-Art-Coverage-Server") != "1" {
		t.Error("X-Art-Coverage-Server header should be present on GET")
	}

	if rr.Body.String() != "ok" {
		t.Error("Inner handler body should be preserved")
	}
}

func TestCoverageHandler_ConcurrentRequests(t *testing.T) {
	if !isCoverageEnabled() {
		t.Skip("Skipping test - coverage not enabled (run with: go test -cover)")
	}

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

	for i := 0; i < numRequests; i++ {
		<-done
	}
}

func TestCoverageResponse_EmptyFieldValidation(t *testing.T) {
	if !isCoverageEnabled() {
		t.Skip("Skipping test - coverage not enabled (run with: go test -cover)")
	}

	req, _ := http.NewRequest("GET", "/coverage", nil)
	rr := httptest.NewRecorder()
	handler := http.HandlerFunc(CoverageHandler)

	handler.ServeHTTP(rr, req)

	var response CoverageResponse
	json.NewDecoder(rr.Body).Decode(&response)

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

func TestDefaultPort(t *testing.T) {
	if DefaultPort != 53700 {
		t.Errorf("DefaultPort should be 53700, got %d", DefaultPort)
	}
}

func TestMaxRetries(t *testing.T) {
	if MaxRetries != 50 {
		t.Errorf("MaxRetries should be 50, got %d", MaxRetries)
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
