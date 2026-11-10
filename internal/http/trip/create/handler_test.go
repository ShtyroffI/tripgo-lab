package create_test

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/ShtyroffI/tripgo-lab/api"
	"github.com/ShtyroffI/tripgo-lab/internal/domain/trip"
	"github.com/ShtyroffI/tripgo-lab/internal/http/trip/create"
	"github.com/ShtyroffI/tripgo-lab/internal/model"
)

const (
	userID   = "11111111-1111-1111-1111-111111111111"
	driverID = "22222222-2222-2222-2222-222222222222"
)

func body(price, startLat string) string {
	return `{"user_id":"` + userID + `","driver_id":"` + driverID + `",` +
		`"start_point":{"latitude":` + startLat + `,"longitude":37.61},` +
		`"end_point":{"latitude":55.80,"longitude":37.70},"price":` + price + `}`
}

func call(t *testing.T, uc *Mockusecase, payload string) *httptest.ResponseRecorder {
	t.Helper()
	h := create.New(uc, slog.New(slog.DiscardHandler))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/trips", strings.NewReader(payload))
	h.CreateTrip(rec, req, api.CreateTripParams{})
	return rec
}

func TestHandler_CreateTrip_Created(t *testing.T) {
	t.Parallel()

	uc := NewMockusecase(gomock.NewController(t))
	id := uuid.New()
	startedAt := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	wantIn := model.CreateTripInput{
		UserID:   uuid.MustParse(userID),
		DriverID: uuid.MustParse(driverID),
		Start:    trip.Point{Latitude: 55.75, Longitude: 37.61},
		End:      trip.Point{Latitude: 55.80, Longitude: 37.70},
		Price:    1450,
	}
	uc.EXPECT().Create(gomock.Any(), wantIn).Return(trip.Trip{
		ID: id, UserID: wantIn.UserID, DriverID: wantIn.DriverID,
		Start: wantIn.Start, End: wantIn.End, Price: wantIn.Price,
		Status: trip.StatusActive, StartedAt: startedAt,
	}, nil)

	rec := call(t, uc, body("1450", "55.75"))

	assert.Equal(t, http.StatusCreated, rec.Code)
	assert.Equal(t, "/api/v1/trips/"+id.String(), rec.Header().Get("Location"))
	assert.Equal(t, "application/json", rec.Header().Get("Content-Type"))
	var got api.Trip
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
	assert.Equal(t, id, got.Id)
	assert.Equal(t, wantIn.UserID, got.UserId)
	assert.Equal(t, wantIn.DriverID, got.DriverId)
	assert.Equal(t, int64(1450), got.Price)
	assert.Equal(t, api.TripStatus("active"), got.Status)
	assert.Equal(t, 55.75, got.StartPoint.Latitude)
	assert.True(t, startedAt.Equal(got.StartedAt))
}

func TestHandler_CreateTrip_ZeroValuesAccepted(t *testing.T) {
	t.Parallel()

	uc := NewMockusecase(gomock.NewController(t))
	// Нули (price и latitude) допустимы: usecase должен быть вызван
	uc.EXPECT().Create(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ any, in model.CreateTripInput) (trip.Trip, error) {
			assert.Equal(t, int64(0), in.Price)
			assert.InDelta(t, 0.0, in.Start.Latitude, 0)
			return trip.Trip{ID: uuid.New(), Status: trip.StatusActive}, nil
		},
	)

	rec := call(t, uc, body("0", "0"))

	assert.Equal(t, http.StatusCreated, rec.Code)
}

func TestHandler_CreateTrip_InvalidBody_400InvalidRequest(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		payload string
	}{
		{"not json", `not a json`},
		{"empty body", ``},
		{"json array", `[1,2,3]`},
		{"missing price", `{"user_id":"` + userID + `","driver_id":"` + driverID + `",` +
			`"start_point":{"latitude":1,"longitude":2},"end_point":{"latitude":3,"longitude":4}}`},
		{"null price", `{"user_id":"` + userID + `","driver_id":"` + driverID + `",` +
			`"start_point":{"latitude":1,"longitude":2},"end_point":{"latitude":3,"longitude":4},"price":null}`},
		{"null user_id", `{"user_id":null,"driver_id":"` + driverID + `",` +
			`"start_point":{"latitude":1,"longitude":2},"end_point":{"latitude":3,"longitude":4},"price":1}`},
		{"missing start latitude", `{"user_id":"` + userID + `","driver_id":"` + driverID + `",` +
			`"start_point":{"longitude":2},"end_point":{"latitude":3,"longitude":4},"price":1}`},
		{"start point is not an object", `{"user_id":"` + userID + `","driver_id":"` + driverID + `",` +
			`"start_point":5,"end_point":{"latitude":3,"longitude":4},"price":1}`},
		{"unknown field id", `{"id":"` + userID + `","user_id":"` + userID + `","driver_id":"` + driverID + `",` +
			`"start_point":{"latitude":1,"longitude":2},"end_point":{"latitude":3,"longitude":4},"price":1}`},
		{"user_id is not uuid", `{"user_id":"abc","driver_id":"` + driverID + `",` +
			`"start_point":{"latitude":1,"longitude":2},"end_point":{"latitude":3,"longitude":4},"price":1}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// Без EXPECT: вызов usecase провалит тест
			uc := NewMockusecase(gomock.NewController(t))

			rec := call(t, uc, tt.payload)

			assert.Equal(t, http.StatusBadRequest, rec.Code)
			assert.Equal(t, "application/problem+json", rec.Header().Get("Content-Type"))
			assert.Contains(t, rec.Body.String(), `"code":"invalid_request"`)
		})
	}
}

func TestHandler_CreateTrip_UsecaseErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		err        error
		wantStatus int
		wantCode   string
	}{
		{
			"validation error returns 400 invalid_request",
			&trip.ValidationError{Field: "price", Reason: "must be >= 0"}, http.StatusBadRequest, "invalid_request",
		},
		{"busy driver returns 409 driver_busy", trip.ErrDriverBusy, http.StatusConflict, "driver_busy"},
		{
			"repository error returns 500 without details",
			errors.New("connection refused to 10.0.0.1"), http.StatusInternalServerError, "internal_error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			uc := NewMockusecase(gomock.NewController(t))
			uc.EXPECT().Create(gomock.Any(), gomock.Any()).Return(trip.Trip{}, tt.err)

			rec := call(t, uc, body("1450", "55.75"))

			assert.Equal(t, tt.wantStatus, rec.Code)
			assert.Equal(t, "application/problem+json", rec.Header().Get("Content-Type"))
			assert.Contains(t, rec.Body.String(), `"code":"`+tt.wantCode+`"`)
			assert.NotContains(t, rec.Body.String(), "connection refused")
			assert.NotContains(t, rec.Body.String(), "10.0.0.1")
			assert.Empty(t, rec.Header().Get("Location"))
		})
	}
}
