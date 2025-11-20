package main

import (
	"crypto/rand"
	"encoding/hex"
	"log"
	"net/http"
	"os"
	"time"
)

var accessPassphrase string

func init() {
	accessPassphrase = os.Getenv("ACCESS_PASSPHRASE")
	if accessPassphrase == "" {
		log.Println("Warning: ACCESS_PASSPHRASE not set - authentication disabled")
	}
}

// generateSessionID generates a random session ID
func generateSessionID() (string, error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes), nil
}

// getSessionCookie retrieves the session cookie from request
func getSessionCookie(r *http.Request) string {
	cookie, err := r.Cookie("skyweave_session")
	if err != nil {
		return ""
	}
	return cookie.Value
}

// setSessionCookie sets the session cookie
func setSessionCookie(w http.ResponseWriter, sessionID string) {
	http.SetCookie(w, &http.Cookie{
		Name:     "skyweave_session",
		Value:    sessionID,
		Path:     "/",
		MaxAge:   86400, // 24 hours
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
}

// requireAuth middleware checks if user is authenticated
func requireAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// If no passphrase is set, skip authentication
		if accessPassphrase == "" {
			next(w, r)
			return
		}

		// Check session cookie
		sessionID := getSessionCookie(r)
		if sessionID != "" && isValidSession(sessionID) {
			next(w, r)
			return
		}

		// Not authenticated, redirect to login
		http.Redirect(w, r, "/login", http.StatusSeeOther)
	}
}

// loginHandler displays the login page
func loginHandler(w http.ResponseWriter, r *http.Request) {
	// If no passphrase is set, redirect to home
	if accessPassphrase == "" {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	// If already authenticated, redirect to home
	sessionID := getSessionCookie(r)
	if sessionID != "" && isValidSession(sessionID) {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	data := struct {
		Error string
	}{
		Error: "",
	}

	if r.Method == http.MethodPost {
		passphrase := r.FormValue("passphrase")

		if passphrase == accessPassphrase {
			// Create new session
			sessionID, err := generateSessionID()
			if err != nil {
				log.Printf("Failed to generate session ID: %v", err)
				http.Error(w, "Internal server error", http.StatusInternalServerError)
				return
			}

			if err := createSession(sessionID); err != nil {
				log.Printf("Failed to create session: %v", err)
				http.Error(w, "Internal server error", http.StatusInternalServerError)
				return
			}

			setSessionCookie(w, sessionID)
			http.Redirect(w, r, "/", http.StatusSeeOther)
			return
		} else {
			data.Error = "Invalid passphrase. Please try again."
		}
	}

	templates.ExecuteTemplate(w, "login.html", data)
}

// startSessionCleanup starts a background goroutine to clean up expired sessions
func startSessionCleanup() {
	ticker := time.NewTicker(1 * time.Hour)
	go func() {
		for range ticker.C {
			if err := cleanupExpiredSessions(); err != nil {
				log.Printf("Failed to cleanup expired sessions: %v", err)
			} else {
				log.Println("Cleaned up expired sessions")
			}
		}
	}()
}
