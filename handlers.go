package main

import (
	"html/template"
	"log"
	"net/http"
	"os"
	"time"
)

var templates *template.Template

// initTemplates loads all HTML templates
func initTemplates() {
	var err error
	templates, err = template.ParseGlob("templates/*.html")
	if err != nil {
		log.Fatal(err)
	}
}

// home handler displays the welcome page
func home(w http.ResponseWriter, r *http.Request) {
	templates.ExecuteTemplate(w, "home.html", nil)
}

// startHandler displays the form for creating a new request
func startHandler(w http.ResponseWriter, r *http.Request) {
	// Generate user ID
	userID, err := generateID(8)
	if err != nil {
		http.Error(w, "Failed to generate user ID", http.StatusInternalServerError)
		return
	}

	data := struct {
		UserID string
	}{
		UserID: userID,
	}

	templates.ExecuteTemplate(w, "start.html", data)
}

// submitHandler handles form submission
func submitHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Parse multipart form (32MB max)
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		http.Error(w, "Failed to parse form", http.StatusBadRequest)
		return
	}

	userID := r.FormValue("user_id")
	city := r.FormValue("city")
	date := r.FormValue("date")

	// Get uploaded file
	file, header, err := r.FormFile("photo")
	if err != nil {
		http.Error(w, "Failed to get uploaded file", http.StatusBadRequest)
		return
	}
	defer file.Close()

	// Generate request ID
	requestID, err := generateID(16)
	if err != nil {
		http.Error(w, "Failed to generate request ID", http.StatusInternalServerError)
		return
	}

	// Save uploaded file
	imagePath, err := saveUploadedFile(file, header, requestID)
	if err != nil {
		http.Error(w, "Failed to save file", http.StatusInternalServerError)
		return
	}

	// Create request record
	req := &Request{
		ID:         requestID,
		UserID:     userID,
		City:       city,
		TargetDate: date,
		ImagePath:  imagePath,
		Status:     "pending",
	}

	if err := saveRequest(req); err != nil {
		http.Error(w, "Failed to save request", http.StatusInternalServerError)
		return
	}

	// Simulate weather API call
	weather, temp := simulateWeather(city, date)
	if err := updateRequestWeather(requestID, weather, temp); err != nil {
		http.Error(w, "Failed to update weather", http.StatusInternalServerError)
		return
	}

	// Redirect to confirmation page
	http.Redirect(w, r, "/weather/"+requestID, http.StatusSeeOther)
}

// weatherHandler displays weather confirmation page
func weatherHandler(w http.ResponseWriter, r *http.Request) {
	requestID := r.PathValue("id")

	req, err := getRequest(requestID)
	if err != nil {
		http.Error(w, "Request not found", http.StatusNotFound)
		return
	}

	data := struct {
		Request *Request
	}{
		Request: req,
	}

	templates.ExecuteTemplate(w, "confirm.html", data)
}

// confirmHandler handles user confirmation or cancellation
func confirmHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	requestID := r.FormValue("request_id")
	action := r.FormValue("action")

	if action == "cancel" {
		updateRequestStatus(requestID, "cancelled")
		http.Redirect(w, r, "/status/"+requestID, http.StatusSeeOther)
		return
	}

	// Confirm action - start async processing
	updateRequestStatus(requestID, "confirmed")

	// Simulate async processing in background
	go func() {
		time.Sleep(3 * time.Second) // Simulate API call delay

		// Get the original image path
		req, err := getRequest(requestID)
		if err != nil {
			log.Printf("Failed to get request: %v", err)
			return
		}

		// For now, just copy the original image as "result"
		// In production, this would be the Replicate API result
		updateRequestResult(requestID, req.ImagePath)
	}()

	// Redirect to processing page
	http.Redirect(w, r, "/processing/"+requestID, http.StatusSeeOther)
}

// processingHandler displays the processing page with HTMX polling
func processingHandler(w http.ResponseWriter, r *http.Request) {
	requestID := r.PathValue("id")

	data := struct {
		RequestID string
	}{
		RequestID: requestID,
	}

	templates.ExecuteTemplate(w, "processing.html", data)
}

// statusHandler returns the current status for HTMX polling
func statusHandler(w http.ResponseWriter, r *http.Request) {
	requestID := r.PathValue("id")

	req, err := getRequest(requestID)
	if err != nil {
		http.Error(w, "Request not found", http.StatusNotFound)
		return
	}

	data := struct {
		Status    string
		RequestID string
	}{
		Status:    req.Status,
		RequestID: requestID,
	}

	templates.ExecuteTemplate(w, "status.html", data)
}

// imageHandler serves the processed image
func imageHandler(w http.ResponseWriter, r *http.Request) {
	requestID := r.PathValue("id")

	req, err := getRequest(requestID)
	if err != nil {
		http.Error(w, "Request not found", http.StatusNotFound)
		return
	}

	if req.Status != "completed" {
		http.Error(w, "Image not ready", http.StatusNotFound)
		return
	}

	// Serve the image file
	imagePath := req.ResultImagePath
	if _, err := os.Stat(imagePath); os.IsNotExist(err) {
		http.Error(w, "Image file not found", http.StatusNotFound)
		return
	}

	http.ServeFile(w, r, imagePath)
}
