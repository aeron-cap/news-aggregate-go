package main

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"net/http"
)

func respondWithJSON(w http.ResponseWriter, code int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(payload)
}

func respondWithError(w http.ResponseWriter, code int, message, details string) {
	respondWithJSON(w, code, map[string]string{
		"error":   message,
		"details": details,
	})	
}

func hashSession(token string) string {
	hash := sha256.Sum256([]byte(token))
	return hex.EncodeToString(hash[:])
}

func newSessionToken() (string, string) {
	raw := make([]byte, 32)
	rand.Read(raw)

	token := base64.RawURLEncoding.EncodeToString(raw)
	return token, hashSession(token)
}