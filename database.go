package main

import (
	"database/sql"
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

	// Create tables
	schema := `
	CREATE TABLE IF NOT EXISTS requests (
		id TEXT PRIMARY KEY,
		user_id TEXT NOT NULL,
		city TEXT NOT NULL,
		target_date TEXT NOT NULL,
		image_path TEXT NOT NULL,
		weather_condition TEXT,
		temperature TEXT,
		status TEXT NOT NULL DEFAULT 'pending',
		result_image_path TEXT,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);
	
	CREATE INDEX IF NOT EXISTS idx_user_id ON requests(user_id);
	CREATE INDEX IF NOT EXISTS idx_status ON requests(status);
	`

	_, err = db.Exec(schema)
	return err
}

// Request represents a weather image editing request
type Request struct {
	ID               string
	UserID           string
	City             string
	TargetDate       string
	ImagePath        string
	WeatherCondition string
	Temperature      string
	Status           string // pending, weather_fetched, confirmed, processing, completed, cancelled
	ResultImagePath  string
}

// saveRequest saves a new request to the database
func saveRequest(req *Request) error {
	query := `INSERT INTO requests (id, user_id, city, target_date, image_path, status)
	          VALUES (?, ?, ?, ?, ?, ?)`
	_, err := db.Exec(query, req.ID, req.UserID, req.City, req.TargetDate, req.ImagePath, req.Status)
	return err
}

// updateRequestWeather updates weather information for a request
func updateRequestWeather(id, weather, temp string) error {
	query := `UPDATE requests SET weather_condition = ?, temperature = ?, status = 'weather_fetched', 
	          updated_at = CURRENT_TIMESTAMP WHERE id = ?`
	_, err := db.Exec(query, weather, temp, id)
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
	query := `SELECT id, user_id, city, target_date, image_path, 
	          COALESCE(weather_condition, ''), COALESCE(temperature, ''), 
	          status, COALESCE(result_image_path, '')
	          FROM requests WHERE id = ?`

	req := &Request{}
	err := db.QueryRow(query, id).Scan(
		&req.ID, &req.UserID, &req.City, &req.TargetDate, &req.ImagePath,
		&req.WeatherCondition, &req.Temperature, &req.Status, &req.ResultImagePath,
	)
	if err != nil {
		return nil, err
	}
	return req, nil
}
