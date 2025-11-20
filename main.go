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

	// Start session cleanup background task
	startSessionCleanup()

	mux := http.NewServeMux()

	// Public routes (no authentication required)
	mux.HandleFunc("GET /login", loginHandler)
	mux.HandleFunc("POST /login", loginHandler)

	// Protected routes (authentication required)
	mux.HandleFunc("GET /{$}", requireAuth(home))
	mux.HandleFunc("GET /start", requireAuth(startHandler))
	mux.HandleFunc("POST /submit", requireAuth(submitHandler))
	mux.HandleFunc("GET /weather/{id}", requireAuth(weatherHandler))
	mux.HandleFunc("POST /confirm", requireAuth(confirmHandler))
	mux.HandleFunc("GET /processing/{id}", requireAuth(processingHandler))
	mux.HandleFunc("GET /status/{id}", requireAuth(statusHandler))
	mux.HandleFunc("GET /image/{id}", requireAuth(imageHandler))

	// Support PORT environment variable
	port := os.Getenv("PORT")
	if port == "" {
		port = "4000"
	}

	log.Print("starting server on :" + port)

	err := http.ListenAndServe(":"+port, mux)
	log.Fatal(err)
}
