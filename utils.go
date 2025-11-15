package main

import (
	"crypto/rand"
	"encoding/hex"
	"io"
	"mime/multipart"
	"os"
	"path/filepath"
)

// generateID generates a random hex string of specified length
func generateID(length int) (string, error) {
	bytes := make([]byte, length)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes), nil
}

// saveUploadedFile saves an uploaded file to the data/uploads directory
func saveUploadedFile(file multipart.File, header *multipart.FileHeader, requestID string) (string, error) {
	uploadDir := filepath.Join("./data", "uploads")
	if err := os.MkdirAll(uploadDir, 0755); err != nil {
		return "", err
	}

	// Create filename: requestID_originalname
	ext := filepath.Ext(header.Filename)
	filename := requestID + ext
	filepath := filepath.Join(uploadDir, filename)

	dst, err := os.Create(filepath)
	if err != nil {
		return "", err
	}
	defer dst.Close()

	if _, err = io.Copy(dst, file); err != nil {
		return "", err
	}

	return filepath, nil
}
