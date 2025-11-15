package main

import (
	"fmt"
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

	now := time.Now()
	// Calculate date range: 1 year ago to 16 days ahead
	minDate := now.AddDate(-1, 0, 0).Format("2006-01-02")
	maxDate := now.AddDate(0, 0, 16).Format("2006-01-02")

	data := struct {
		UserID  string
		MinDate string
		MaxDate string
	}{
		UserID:  userID,
		MinDate: minDate,
		MaxDate: maxDate,
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
	location := r.FormValue("location")
	dateStr := r.FormValue("date")
	timeOfDay := r.FormValue("time_of_day")

	// Parse target date
	targetDate, err := time.Parse("2006-01-02", dateStr)
	if err != nil {
		http.Error(w, "Invalid date format", http.StatusBadRequest)
		return
	}

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
		ID:            requestID,
		UserID:        userID,
		LocationInput: location,
		TargetDate:    dateStr,
		TimeOfDay:     timeOfDay,
		ImagePath:     imagePath,
		Status:        "pending",
	}

	if err := saveRequest(req); err != nil {
		http.Error(w, "Failed to save request", http.StatusInternalServerError)
		return
	}

	// Start async processing
	go processWeatherRequest(requestID, location, targetDate)

	// Redirect to processing page immediately
	http.Redirect(w, r, "/processing/"+requestID, http.StatusSeeOther)
}

// processWeatherRequest handles async geocoding and weather fetching
func processWeatherRequest(requestID, location string, targetDate time.Time) {
	// Step 1: Geocode location
	geoResult, err := geocodeLocation(location)
	if err != nil {
		log.Printf("Geocoding failed for request %s: %v", requestID, err)
		updateRequestError(requestID, fmt.Sprintf("Failed to find location: %v", err))
		return
	}

	// Update with geocoding results
	if err := updateRequestGeocode(requestID, geoResult.Name, geoResult.Country,
		geoResult.Lat, geoResult.Lon); err != nil {
		log.Printf("Failed to update geocode for request %s: %v", requestID, err)
		return
	}

	// Update status to weather_fetching
	updateRequestStatus(requestID, "weather_fetching")

	// Step 2: Fetch weather data
	weatherData, err := getHistoricalWeather(geoResult.Lat, geoResult.Lon, targetDate)
	if err != nil {
		log.Printf("Weather fetch failed for request %s: %v", requestID, err)
		updateRequestError(requestID, fmt.Sprintf("Failed to fetch weather: %v", err))
		return
	}

	// Step 3: Generate AI prompt
	locationStr := geoResult.Name
	if geoResult.Country != "" {
		locationStr += ", " + geoResult.Country
	}

	// Get the time of day from the request
	req, err := getRequest(requestID)
	if err != nil {
		log.Printf("Failed to get request for prompt generation: %v", err)
		updateRequestError(requestID, "Failed to retrieve request details")
		return
	}

	prompt := generatePrompt(weatherData, locationStr, req.TimeOfDay)

	// Update with weather data and prompt
	if err := updateRequestWeather(requestID, weatherData, prompt); err != nil {
		log.Printf("Failed to update weather for request %s: %v", requestID, err)
		updateRequestError(requestID, "Failed to save weather data")
		return
	}

	log.Printf("Weather data fetched successfully for request %s", requestID)
}

// weatherHandler displays weather confirmation page (now accessed via processing page)
func weatherHandler(w http.ResponseWriter, r *http.Request) {
	requestID := r.PathValue("id")

	req, err := getRequest(requestID)
	if err != nil {
		http.Error(w, "Request not found", http.StatusNotFound)
		return
	}

	// Only show if weather has been fetched
	if req.Status != "weather_fetched" {
		http.Redirect(w, r, "/processing/"+requestID, http.StatusSeeOther)
		return
	}

	data := struct {
		Request *Request
	}{
		Request: req,
	}

	templates.ExecuteTemplate(w, "confirm.html", data)
} // confirmHandler handles user confirmation or cancellation
func confirmHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	requestID := r.FormValue("request_id")
	action := r.FormValue("action")

	if action == "cancel" {
		updateRequestStatus(requestID, "cancelled")
		http.Redirect(w, r, "/start", http.StatusSeeOther)
		return
	}

	// Check current status to prevent duplicate processing
	req, err := getRequest(requestID)
	if err != nil {
		http.Error(w, "Request not found", http.StatusNotFound)
		return
	}

	// Only start processing if status is weather_fetched
	// This prevents duplicate API calls if user clicks confirm multiple times
	if req.Status != "weather_fetched" {
		// Already processing or completed, just redirect
		http.Redirect(w, r, "/processing/"+requestID, http.StatusSeeOther)
		return
	}

	// Confirm action - start async Replicate processing
	updateRequestStatus(requestID, "confirmed")

	// Start real AI image editing with Replicate
	go processImageWithReplicate(requestID)

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
		Status       string
		RequestID    string
		ErrorMessage string
	}{
		Status:       req.Status,
		RequestID:    requestID,
		ErrorMessage: req.ErrorMessage,
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
