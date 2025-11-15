package main

import (
	"database/sql"
	"fmt"
	"log"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
)

var db *sql.DB

// initDB initializes the database connection and creates tables
func initDB() error {
	// Ensure data directory exists
	if err := os.MkdirAll("./data", 0755); err != nil {
		return err
	}

	dbPath := filepath.Join("./data", "skyweave.db")
	var err error
	db, err = sql.Open("sqlite", dbPath)
	if err != nil {
		return err
	}

	// Check if migration is needed
	if err := checkAndMigrate(); err != nil {
		log.Printf("Migration check failed, recreating database: %v", err)
		// If migration fails, drop and recreate tables
		if err := recreateTables(); err != nil {
			return fmt.Errorf("failed to recreate tables: %w", err)
		}
	}

	return nil
}

// checkAndMigrate checks if the table structure matches the current schema
func checkAndMigrate() error {
	// Try to query the table with all expected columns
	testQuery := `SELECT id, user_id, location_input, location_name, country, 
	              latitude, longitude, target_date, image_path, 
	              weather_condition, weather_description, temperature, feels_like,
	              humidity, clouds, wind_speed, visibility, precipitation, ai_prompt,
	              prediction_id, status, error_message, result_image_path, created_at, updated_at
	              FROM requests LIMIT 0`

	_, err := db.Exec(testQuery)
	if err != nil {
		// Table doesn't exist or structure is wrong
		return fmt.Errorf("table structure mismatch: %w", err)
	}

	return nil
}

// recreateTables drops existing tables and creates new ones with current schema
func recreateTables() error {
	log.Println("Dropping old tables...")

	// Drop existing tables
	_, err := db.Exec("DROP TABLE IF EXISTS requests")
	if err != nil {
		return fmt.Errorf("failed to drop requests table: %w", err)
	}

	log.Println("Creating new tables with updated schema...")

	// Create tables with current schema
	schema := `
	CREATE TABLE IF NOT EXISTS requests (
		id TEXT PRIMARY KEY,
		user_id TEXT NOT NULL,
		location_input TEXT NOT NULL,
		location_name TEXT,
		country TEXT,
		latitude REAL,
		longitude REAL,
		target_date TEXT NOT NULL,
		image_path TEXT NOT NULL,
		weather_condition TEXT,
		weather_description TEXT,
		temperature REAL,
		feels_like REAL,
		humidity INTEGER,
		clouds INTEGER,
		wind_speed REAL,
		visibility INTEGER,
		precipitation TEXT,
		ai_prompt TEXT,
		prediction_id TEXT,
		status TEXT NOT NULL DEFAULT 'pending',
		error_message TEXT,
		result_image_path TEXT,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);
	
	CREATE INDEX IF NOT EXISTS idx_user_id ON requests(user_id);
	CREATE INDEX IF NOT EXISTS idx_status ON requests(status);
	CREATE INDEX IF NOT EXISTS idx_prediction_id ON requests(prediction_id);
	`

	_, err = db.Exec(schema)
	if err != nil {
		return fmt.Errorf("failed to create tables: %w", err)
	}

	log.Println("Database schema updated successfully!")
	return nil
}

// Request represents a weather image editing request
type Request struct {
	ID                 string
	UserID             string
	LocationInput      string
	LocationName       string
	Country            string
	Latitude           float64
	Longitude          float64
	TargetDate         string
	ImagePath          string
	WeatherCondition   string
	WeatherDescription string
	Temperature        float64
	FeelsLike          float64
	Humidity           int
	Clouds             int
	WindSpeed          float64
	Visibility         int
	Precipitation      string
	AIPrompt           string
	PredictionID       string
	Status             string // pending, geocoding, weather_fetching, weather_fetched, confirmed, processing, completed, cancelled, error
	ErrorMessage       string
	ResultImagePath    string
}

// saveRequest saves a new request to the database
func saveRequest(req *Request) error {
	query := `INSERT INTO requests (id, user_id, location_input, target_date, image_path, status)
	          VALUES (?, ?, ?, ?, ?, ?)`
	_, err := db.Exec(query, req.ID, req.UserID, req.LocationInput, req.TargetDate, req.ImagePath, req.Status)
	return err
}

// updateRequestGeocode updates geocoding information for a request
func updateRequestGeocode(id string, locationName, country string, lat, lon float64) error {
	query := `UPDATE requests SET location_name = ?, country = ?, latitude = ?, longitude = ?, 
	          status = 'geocoding', updated_at = CURRENT_TIMESTAMP WHERE id = ?`
	_, err := db.Exec(query, locationName, country, lat, lon, id)
	return err
}

// updateRequestWeather updates weather information for a request
func updateRequestWeather(id string, weatherData *WeatherData, prompt string) error {
	condition := weatherData.Condition
	description := weatherData.Description

	precipitation := ""
	if weatherData.Rain > 0 {
		precipitation = fmt.Sprintf("Rain: %.1fmm", weatherData.Rain)
	} else if weatherData.Snow > 0 {
		precipitation = fmt.Sprintf("Snow: %.1fmm", weatherData.Snow)
	}

	query := `UPDATE requests SET 
	          weather_condition = ?, weather_description = ?, temperature = ?, 
	          feels_like = ?, humidity = ?, clouds = ?, wind_speed = ?, 
	          visibility = ?, precipitation = ?, ai_prompt = ?,
	          status = 'weather_fetched', updated_at = CURRENT_TIMESTAMP 
	          WHERE id = ?`

	_, err := db.Exec(query, condition, description, weatherData.Temp, weatherData.FeelsLike,
		weatherData.Humidity, weatherData.Clouds, weatherData.WindSpeed, weatherData.Visibility, precipitation,
		prompt, id)
	return err
}

// updateRequestError updates error status for a request
func updateRequestError(id, errorMsg string) error {
	query := `UPDATE requests SET status = 'error', error_message = ?, 
	          updated_at = CURRENT_TIMESTAMP WHERE id = ?`
	_, err := db.Exec(query, errorMsg, id)
	return err
}

// updateRequestPredictionID updates the Replicate prediction ID for a request
func updateRequestPredictionID(id, predictionID string) error {
	query := `UPDATE requests SET prediction_id = ?, status = 'processing',
	          updated_at = CURRENT_TIMESTAMP WHERE id = ?`
	_, err := db.Exec(query, predictionID, id)
	return err
}

// updateRequestStatus updates the status of a request
func updateRequestStatus(id, status string) error {
	query := `UPDATE requests SET status = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`
	_, err := db.Exec(query, status, id)
	return err
}

// updateRequestResult updates the result image path and marks as completed
func updateRequestResult(id, resultPath string) error {
	query := `UPDATE requests SET result_image_path = ?, status = 'completed', 
	          updated_at = CURRENT_TIMESTAMP WHERE id = ?`
	_, err := db.Exec(query, resultPath, id)
	return err
}

// getRequest retrieves a request by ID
func getRequest(id string) (*Request, error) {
	query := `SELECT id, user_id, location_input, 
	          COALESCE(location_name, ''), COALESCE(country, ''),
	          COALESCE(latitude, 0), COALESCE(longitude, 0),
	          target_date, image_path, 
	          COALESCE(weather_condition, ''), COALESCE(weather_description, ''),
	          COALESCE(temperature, 0), COALESCE(feels_like, 0),
	          COALESCE(humidity, 0), COALESCE(clouds, 0),
	          COALESCE(wind_speed, 0), COALESCE(visibility, 0),
	          COALESCE(precipitation, ''), COALESCE(ai_prompt, ''),
	          COALESCE(prediction_id, ''),
	          status, COALESCE(error_message, ''), COALESCE(result_image_path, '')
	          FROM requests WHERE id = ?`

	req := &Request{}
	err := db.QueryRow(query, id).Scan(
		&req.ID, &req.UserID, &req.LocationInput,
		&req.LocationName, &req.Country, &req.Latitude, &req.Longitude,
		&req.TargetDate, &req.ImagePath,
		&req.WeatherCondition, &req.WeatherDescription,
		&req.Temperature, &req.FeelsLike, &req.Humidity, &req.Clouds,
		&req.WindSpeed, &req.Visibility, &req.Precipitation, &req.AIPrompt,
		&req.PredictionID,
		&req.Status, &req.ErrorMessage, &req.ResultImagePath,
	)
	if err != nil {
		return nil, err
	}
	return req, nil
}
