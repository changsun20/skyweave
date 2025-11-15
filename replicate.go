package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

var replicateAPIToken string

func init() {
	replicateAPIToken = os.Getenv("REPLICATE_API_TOKEN")
	if replicateAPIToken == "" {
		fmt.Println("Warning: REPLICATE_API_TOKEN not set - AI image editing will not work")
	}
}

// ReplicatePredictionRequest represents the request to create a prediction
type ReplicatePredictionRequest struct {
	Input ReplicateInput `json:"input"`
}

// ReplicateInput represents the input parameters for the model
type ReplicateInput struct {
	Prompt       string `json:"prompt"`
	InputImage   string `json:"input_image"`
	OutputFormat string `json:"output_format"`
}

// ReplicatePrediction represents a prediction response from Replicate
type ReplicatePrediction struct {
	ID     string                 `json:"id"`
	Status string                 `json:"status"` // starting, processing, succeeded, failed, canceled
	Input  map[string]interface{} `json:"input"`
	Output interface{}            `json:"output"` // can be string URL or array of URLs
	Error  string                 `json:"error,omitempty"`
	Logs   string                 `json:"logs,omitempty"`
	URLs   struct {
		Get    string `json:"get"`
		Cancel string `json:"cancel"`
	} `json:"urls"`
}

// ReplicateFileUpload represents the file upload response
type ReplicateFileUpload struct {
	URLs struct {
		Get string `json:"get"`
	} `json:"urls"`
}

// uploadFileToReplicate uploads a local file to Replicate and returns the URL
func uploadFileToReplicate(localPath string) (string, error) {
	if replicateAPIToken == "" {
		return "", fmt.Errorf("REPLICATE_API_TOKEN not set")
	}

	// Open the file
	file, err := os.Open(localPath)
	if err != nil {
		return "", fmt.Errorf("failed to open file: %w", err)
	}
	defer file.Close()

	// Create multipart form
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)

	// Add file field
	filename := filepath.Base(localPath)
	part, err := writer.CreateFormFile("content", filename)
	if err != nil {
		return "", fmt.Errorf("failed to create form file: %w", err)
	}

	if _, err = io.Copy(part, file); err != nil {
		return "", fmt.Errorf("failed to copy file: %w", err)
	}

	writer.Close()

	// Make request to Replicate files API
	req, err := http.NewRequest("POST", "https://api.replicate.com/v1/files", &buf)
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+replicateAPIToken)
	req.Header.Set("Content-Type", writer.FormDataContentType())

	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("file upload request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read upload response: %w", err)
	}

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("file upload failed: %s - %s", resp.Status, string(body))
	}

	var upload ReplicateFileUpload
	if err := json.Unmarshal(body, &upload); err != nil {
		return "", fmt.Errorf("failed to parse upload response: %w", err)
	}

	return upload.URLs.Get, nil
}

// createReplicatePrediction creates a new prediction on Replicate
func createReplicatePrediction(prompt, imageURL string) (*ReplicatePrediction, error) {
	if replicateAPIToken == "" {
		return nil, fmt.Errorf("REPLICATE_API_TOKEN not set")
	}

	// Prepare request body
	reqBody := ReplicatePredictionRequest{
		Input: ReplicateInput{
			Prompt:       prompt,
			InputImage:   imageURL,
			OutputFormat: "jpg",
		},
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	// Create request
	req, err := http.NewRequest(
		"POST",
		"https://api.replicate.com/v1/models/black-forest-labs/flux-kontext-pro/predictions",
		bytes.NewBuffer(jsonData),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+replicateAPIToken)
	req.Header.Set("Content-Type", "application/json")

	// Make request
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("prediction request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("prediction creation failed: %s - %s", resp.Status, string(body))
	}

	var prediction ReplicatePrediction
	if err := json.Unmarshal(body, &prediction); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return &prediction, nil
}

// getPredictionStatus checks the status of a prediction
func getPredictionStatus(predictionID string) (*ReplicatePrediction, error) {
	if replicateAPIToken == "" {
		return nil, fmt.Errorf("REPLICATE_API_TOKEN not set")
	}

	url := fmt.Sprintf("https://api.replicate.com/v1/predictions/%s", predictionID)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+replicateAPIToken)

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("status check failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("status check failed: %s - %s", resp.Status, string(body))
	}

	var prediction ReplicatePrediction
	if err := json.Unmarshal(body, &prediction); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return &prediction, nil
}

// downloadImage downloads an image from a URL and saves it locally
func downloadImage(imageURL, savePath string) error {
	resp, err := http.Get(imageURL)
	if err != nil {
		return fmt.Errorf("failed to download image: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download failed with status: %s", resp.Status)
	}

	// Ensure directory exists
	dir := filepath.Dir(savePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	// Create file
	file, err := os.Create(savePath)
	if err != nil {
		return fmt.Errorf("failed to create file: %w", err)
	}
	defer file.Close()

	// Copy data
	if _, err := io.Copy(file, resp.Body); err != nil {
		return fmt.Errorf("failed to save image: %w", err)
	}

	return nil
}

// processImageWithReplicate handles the full image processing workflow
func processImageWithReplicate(requestID string) {
	log.Printf("Starting Replicate processing for request %s", requestID)

	// Get request details
	req, err := getRequest(requestID)
	if err != nil {
		log.Printf("Failed to get request %s: %v", requestID, err)
		updateRequestError(requestID, "Failed to retrieve request details")
		return
	}

	// Upload original image to Replicate
	log.Printf("Uploading image to Replicate for request %s", requestID)
	imageURL, err := uploadFileToReplicate(req.ImagePath)
	if err != nil {
		log.Printf("Failed to upload image for request %s: %v", requestID, err)
		updateRequestError(requestID, fmt.Sprintf("Failed to upload image: %v", err))
		return
	}

	log.Printf("Image uploaded successfully: %s", imageURL)

	// Create prediction
	log.Printf("Creating prediction for request %s with prompt", requestID)
	prediction, err := createReplicatePrediction(req.AIPrompt, imageURL)
	if err != nil {
		log.Printf("Failed to create prediction for request %s: %v", requestID, err)
		updateRequestError(requestID, fmt.Sprintf("Failed to create prediction: %v", err))
		return
	}

	log.Printf("Prediction created: %s (status: %s)", prediction.ID, prediction.Status)

	// Save prediction ID
	if err := updateRequestPredictionID(requestID, prediction.ID); err != nil {
		log.Printf("Failed to save prediction ID for request %s: %v", requestID, err)
	}

	// Poll for completion
	maxAttempts := 120 // 10 minutes (5 seconds * 120)
	for i := 0; i < maxAttempts; i++ {
		time.Sleep(5 * time.Second)

		status, err := getPredictionStatus(prediction.ID)
		if err != nil {
			log.Printf("Failed to check status for prediction %s: %v", prediction.ID, err)
			continue
		}

		log.Printf("Prediction %s status: %s", prediction.ID, status.Status)

		switch status.Status {
		case "succeeded":
			// Extract output URL
			var outputURL string
			switch v := status.Output.(type) {
			case string:
				outputURL = v
			case []interface{}:
				if len(v) > 0 {
					outputURL = v[0].(string)
				}
			}

			if outputURL == "" {
				updateRequestError(requestID, "No output URL in prediction result")
				return
			}

			log.Printf("Prediction succeeded, downloading result: %s", outputURL)

			// Download result image
			resultPath := filepath.Join("./data", "results", requestID+".jpg")
			if err := downloadImage(outputURL, resultPath); err != nil {
				log.Printf("Failed to download result for request %s: %v", requestID, err)
				updateRequestError(requestID, fmt.Sprintf("Failed to download result: %v", err))
				return
			}

			// Update request as completed
			if err := updateRequestResult(requestID, resultPath); err != nil {
				log.Printf("Failed to update result for request %s: %v", requestID, err)
			}

			log.Printf("Request %s completed successfully", requestID)
			return

		case "failed":
			errMsg := "Prediction failed"
			if status.Error != "" {
				errMsg = status.Error
			}
			log.Printf("Prediction failed for request %s: %s", requestID, errMsg)
			updateRequestError(requestID, errMsg)
			return

		case "canceled":
			log.Printf("Prediction canceled for request %s", requestID)
			updateRequestStatus(requestID, "cancelled")
			return
		}
	}

	// Timeout
	log.Printf("Prediction timeout for request %s", requestID)
	updateRequestError(requestID, "Image processing timeout")
}
