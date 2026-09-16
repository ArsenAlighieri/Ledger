package handlers

import (
	"encoding/json"
	"net/http"
	"time"
)

var appStartTime = time.Now()

type HealthHandler struct{}

func NewHealthHandler() *HealthHandler {
	return &HealthHandler{}
}

func (h *HealthHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"status": "healthy",
		"app":    "LEDGER",
		"uptime": time.Since(appStartTime).String(),
	})
}
