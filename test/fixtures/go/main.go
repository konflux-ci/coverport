package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"

	_ "github.com/konflux-ci/coverport/instrumentation/go"
)

func greet(name string) string {
	if name == "" {
		return "Hello, World!"
	}
	if strings.ToLower(name) == "coverport" {
		return "Hello from the CoverPort test fixture!"
	}
	return fmt.Sprintf("Hello, %s!", name)
}

func helloHandler(w http.ResponseWriter, r *http.Request) {
	name := r.URL.Query().Get("name")
	msg := greet(name)
	w.Header().Set("Content-Type", "text/plain")
	fmt.Fprintln(w, msg)
}

func main() {
	port := os.Getenv("APP_PORT")
	if port == "" {
		port = "8080"
	}

	http.HandleFunc("/hello", helloHandler)

	log.Printf("Test fixture app listening on :%s", port)
	if err := http.ListenAndServe(":"+port, nil); err != nil {
		log.Fatal(err)
	}
}
