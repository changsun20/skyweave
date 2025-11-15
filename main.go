package main

import (
	"log"
	"net/http"
	"os"
)

func main() {
	// Initialize database
	if err := initDB(); err != nil {
		log.Fatal("Failed to initialize database:", err)
	}
	defer db.Close()

	// Initialize templates
	initTemplates()

	mux := http.NewServeMux()

	// Routes
	mux.HandleFunc("GET /{$}", home)
	mux.HandleFunc("GET /start", startHandler)
	mux.HandleFunc("POST /submit", submitHandler)
	mux.HandleFunc("GET /weather/{id}", weatherHandler)
	mux.HandleFunc("POST /confirm", confirmHandler)
	mux.HandleFunc("GET /processing/{id}", processingHandler)
	mux.HandleFunc("GET /status/{id}", statusHandler)
	mux.HandleFunc("GET /image/{id}", imageHandler)

	// Support Railway's PORT environment variable
	port := os.Getenv("PORT")
	if port == "" {
		port = "4000"
	}

	log.Print("starting server on :" + port)

	err := http.ListenAndServe(":"+port, mux)
	log.Fatal(err)
}
