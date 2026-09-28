package httpserver

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ShtyroffI/tripgo-lab/api"
	"github.com/ShtyroffI/tripgo-lab/internal/domain"
	"github.com/ShtyroffI/tripgo-lab/internal/repository"
)

// TxManager: хендлеры не знают про pgx
type TxManager interface {
	Do(ctx context.Context, fn func(ctx context.Context) error) error
}

type Handler struct {
	trips        *repository.TripRepository
	tx           TxManager
	pool         *pgxpool.Pool
	queryTimeout time.Duration
	log          *slog.Logger
}

func NewHandler(trips *repository.TripRepository, tx TxManager, pool *pgxpool.Pool, queryTimeout time.Duration, log *slog.Logger) *Handler {
	return &Handler{trips: trips, tx: tx, pool: pool, queryTimeout: queryTimeout, log: log}
}

var _ api.ServerInterface = (*Handler)(nil)

// CreateTrip пишет trips и trip_status_history в одной транзакции
func (h *Handler) CreateTrip(w http.ResponseWriter, r *http.Request, _ api.CreateTripParams) {
	raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxBodyBytes))
	if err != nil {
		writeInvalidRequest(w, r, "Request body is too large or unreadable")
		return
	}
	if err := checkRequiredFields(raw); err != nil {
		writeInvalidRequest(w, r, err.Error())
		return
	}

	var body api.CreateTripJSONRequestBody
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&body); err != nil {
		writeInvalidRequest(w, r, "Request body is not valid JSON: "+err.Error())
		return
	}
	if err := validateTripData(body); err != nil {
		writeInvalidRequest(w, r, err.Error())
		return
	}

	trip := domain.Trip{
		ID:             uuid.New(),
		UserID:         body.UserId,
		DriverID:       body.DriverId,
		StartLatitude:  body.StartPoint.Latitude,
		StartLongitude: body.StartPoint.Longitude,
		EndLatitude:    body.EndPoint.Latitude,
		EndLongitude:   body.EndPoint.Longitude,
		Price:          body.Price,
		Status:         domain.TripStatusActive,
		StartedAt:      time.Now().UTC(),
	}

	err = h.tx.Do(r.Context(), func(ctx context.Context) error {
		if err := h.trips.Create(ctx, trip); err != nil {
			return err
		}
		return h.trips.AddStatusHistory(ctx, trip.ID, nil, domain.TripStatusActive, "trip created")
	})
	switch {
	case errors.Is(err, domain.ErrDriverBusy):
		writeDriverBusy(w, r)
		return
	case err != nil:
		h.log.Error("create trip", "error", err)
		writeInternalError(w, r)
		return
	}

	w.Header().Set("Location", fmt.Sprintf("/api/v1/trips/%s", trip.ID))
	writeJSON(w, http.StatusCreated, toAPITrip(trip))
}

func (h *Handler) GetTrip(w http.ResponseWriter, r *http.Request, tripId api.TripId) {
	trip, err := h.trips.GetByID(r.Context(), uuid.UUID(tripId))
	switch {
	case errors.Is(err, domain.ErrTripNotFound):
		writeTripNotFound(w, r)
		return
	case err != nil:
		h.log.Error("get trip", "error", err)
		writeInternalError(w, r)
		return
	}

	writeJSON(w, http.StatusOK, toAPITrip(trip))
}

// FinishTrip: смена статуса и запись в журнал в одной транзакции
func (h *Handler) FinishTrip(w http.ResponseWriter, r *http.Request, tripId api.TripId) {
	id := uuid.UUID(tripId)

	var trip domain.Trip
	err := h.tx.Do(r.Context(), func(ctx context.Context) error {
		finished, err := h.trips.Finish(ctx, id)
		if err != nil {
			return err
		}
		trip = finished

		from := domain.TripStatusActive
		return h.trips.AddStatusHistory(ctx, id, &from, domain.TripStatusCompleted, "trip finished")
	})
	switch {
	case errors.Is(err, domain.ErrTripNotFound):
		writeTripNotFound(w, r)
		return
	case errors.Is(err, domain.ErrTripCompleted):
		writeTripCompleted(w, r)
		return
	case err != nil:
		h.log.Error("finish trip", "error", err)
		writeInternalError(w, r)
		return
	}

	writeJSON(w, http.StatusOK, toAPITrip(trip))
}

// Health не ходит в БД
func (h *Handler) Health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, api.HealthResponse{Status: api.HealthResponseStatus("ok")})
}

// Ready проверяет доступность БД
func (h *Handler) Ready(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), h.queryTimeout)
	defer cancel()

	if err := h.pool.Ping(ctx); err != nil {
		writeJSON(w, http.StatusServiceUnavailable, api.HealthResponse{Status: api.HealthResponseStatus("unavailable")})
		return
	}
	writeJSON(w, http.StatusOK, api.HealthResponse{Status: api.HealthResponseStatus("ok")})
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func toAPITrip(t domain.Trip) api.Trip {
	var finishedAt *time.Time
	if t.FinishedAt != nil {
		utc := t.FinishedAt.UTC()
		finishedAt = &utc
	}

	return api.Trip{
		Id:         t.ID,
		UserId:     t.UserID,
		DriverId:   t.DriverID,
		StartPoint: api.Coordinates{Latitude: t.StartLatitude, Longitude: t.StartLongitude},
		EndPoint:   api.Coordinates{Latitude: t.EndLatitude, Longitude: t.EndLongitude},
		Price:      t.Price,
		Status:     api.TripStatus(t.Status),
		StartedAt:  t.StartedAt.UTC(),
		FinishedAt: finishedAt,
	}
}

// maxBodyBytes ограничивает размер тела запроса
const maxBodyBytes = 1 << 20

// checkRequiredFields нужен, чтобы пропущенное число не стало нулём
func checkRequiredFields(raw []byte) error {
	var body map[string]json.RawMessage
	if err := json.Unmarshal(raw, &body); err != nil {
		return errors.New("request body must be a JSON object")
	}

	for _, name := range []string{"user_id", "driver_id", "start_point", "end_point", "price"} {
		if _, ok := body[name]; !ok {
			return fmt.Errorf("%s is required", name)
		}
	}

	for _, name := range []string{"start_point", "end_point"} {
		var point map[string]json.RawMessage
		if err := json.Unmarshal(body[name], &point); err != nil {
			return fmt.Errorf("%s must be an object", name)
		}
		for _, field := range []string{"latitude", "longitude"} {
			if _, ok := point[field]; !ok {
				return fmt.Errorf("%s.%s is required", name, field)
			}
		}
	}
	return nil
}

func validateTripData(body api.TripData) error {
	if body.UserId == uuid.Nil {
		return errors.New("user_id is required")
	}
	if body.DriverId == uuid.Nil {
		return errors.New("driver_id is required")
	}
	if body.Price < 0 {
		return errors.New("price must be >= 0")
	}
	if err := validateCoordinates(body.StartPoint); err != nil {
		return fmt.Errorf("start_point: %w", err)
	}
	if err := validateCoordinates(body.EndPoint); err != nil {
		return fmt.Errorf("end_point: %w", err)
	}
	return nil
}

func validateCoordinates(c api.Coordinates) error {
	if c.Latitude < -90 || c.Latitude > 90 {
		return errors.New("latitude must be between -90 and 90")
	}
	if c.Longitude < -180 || c.Longitude > 180 {
		return errors.New("longitude must be between -180 and 180")
	}
	return nil
}
