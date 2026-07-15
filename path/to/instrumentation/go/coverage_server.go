package main

import (
    "encoding/json"
    "fmt"
    "net/http"
)

type Coverage struct {
    Flags string `json:"flags"`
}

func main() {
    http.HandleFunc("/upload", func(w http.ResponseWriter, r *http.Request) {
        var coverage Coverage
        err := json.NewDecoder(r.Body).Decode(&coverage)
        if err != nil {
            http.Error(w, err.Error(), http.StatusBadRequest)
            return
        }
        fmt.Println("Received coverage data:", coverage.Flags)
        // Upload coverage to Codecov
        upload_coverage(coverage.Flags)
    })
    http.ListenAndServe(":53700", nil)
}

func upload_coverage(flags string) {
    // Use the codecov-action to upload coverage to Codecov
    // with the specified flags
    // ...
}