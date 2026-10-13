package httpserver

import (
	"encoding/json"
	"net/http"

	"github.com/ShtyroffI/tripgo-lab/api"
)

// writeProblem пишет ошибку по RFC 9457
func writeProblem(w http.ResponseWriter, r *http.Request, status int, problemType, title, code, detail string) {
	instance := r.URL.Path
	p := api.Problem{
		Type:     problemType,
		Title:    title,
		Status:   int32(status),
		Code:     code,
		Instance: &instance,
	}
	if detail != "" {
		p.Detail = &detail
	}

	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(p)
}

func writeInvalidRequest(w http.ResponseWriter, r *http.Request, detail string) {
	writeProblem(w, r, http.StatusBadRequest,
		"https://tripgo.example/problems/invalid-request", "Invalid request", "invalid_request", detail)
}

func writeTripNotFound(w http.ResponseWriter, r *http.Request) {
	writeProblem(w, r, http.StatusNotFound,
		"https://tripgo.example/problems/trip-not-found", "Trip not found", "trip_not_found", "Trip was not found")
}

func writeTripCompleted(w http.ResponseWriter, r *http.Request) {
	writeProblem(w, r, http.StatusConflict,
		"https://tripgo.example/problems/trip-completed", "Trip completed", "trip_completed",
		"Operation is not allowed for a completed trip")
}

func writeDriverBusy(w http.ResponseWriter, r *http.Request) {
	writeProblem(w, r, http.StatusConflict,
		"https://tripgo.example/problems/driver-busy", "Driver busy", "driver_busy",
		"Driver already has an active trip")
}

func writeInternalError(w http.ResponseWriter, r *http.Request) {
	writeProblem(w, r, http.StatusInternalServerError,
		"https://tripgo.example/problems/internal-error", "Internal Server Error", "internal_error",
		"Internal server error")
}
