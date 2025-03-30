package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
)

func main() {
	// Define command line flags
	port := flag.Int("port", 8080, "Port to listen on")
	flag.Parse()

	// Create a simple HTTP handler
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "Hello, you've requested: %s\n", r.URL.Path)
	})

	// Start server
	addr := fmt.Sprintf(":%d", *port)
	log.Printf("Starting HTTP server on port %d", *port)
	log.Fatal(http.ListenAndServe(addr, nil))
}
