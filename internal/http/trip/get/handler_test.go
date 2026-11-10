package get_test

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/ShtyroffI/tripgo-lab/api"
	"github.com/ShtyroffI/tripgo-lab/internal/domain/trip"
	"github.com/ShtyroffI/tripgo-lab/internal/http/trip/get"
)

func call(t *testing.T, id uuid.UUID, uc *Mockusecase) *httptest.ResponseRecorder {
	t.Helper()
	h := get.New(uc, slog.New(slog.DiscardHandler))
	rec := httptest.NewRecorder()
	h.GetTrip(rec, httptest.NewRequest(http.MethodGet, "/api/v1/trips/"+id.String(), nil), id)
	return rec
}

func TestHandler_GetTrip_Success(t *testing.T) {
	t.Parallel()

	uc := NewMockusecase(gomock.NewController(t))
	id := uuid.New()
	want := trip.Trip{
		ID: id, UserID: uuid.New(), DriverID: uuid.New(), Price: 700,
		Start:     trip.Point{Latitude: 1, Longitude: 2},
		End:       trip.Point{Latitude: 3, Longitude: 4},
		Status:    trip.StatusActive,
		StartedAt: time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC),
	}
	uc.EXPECT().Get(gomock.Any(), id).Return(want, nil)

	rec := call(t, id, uc)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "application/json", rec.Header().Get("Content-Type"))
	var got api.Trip
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
	assert.Equal(t, id, got.Id)
	assert.Equal(t, want.UserID, got.UserId)
	assert.Equal(t, want.DriverID, got.DriverId)
	assert.Equal(t, int64(700), got.Price)
	assert.Equal(t, api.TripStatus("active"), got.Status)
	assert.Equal(t, 3.0, got.EndPoint.Latitude)
	assert.True(t, want.StartedAt.Equal(got.StartedAt))
	assert.Nil(t, got.FinishedAt)
}

func TestHandler_GetTrip_Errors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		err        error
		wantStatus int
		wantCode   string
	}{
		{"missing trip returns 404 trip_not_found", trip.ErrTripNotFound, http.StatusNotFound, "trip_not_found"},
		{"repository error returns 500 without details", errors.New("pq: password=secret"), http.StatusInternalServerError, "internal_error"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			uc := NewMockusecase(gomock.NewController(t))
			id := uuid.New()
			uc.EXPECT().Get(gomock.Any(), id).Return(trip.Trip{}, tt.err)

			rec := call(t, id, uc)

			assert.Equal(t, tt.wantStatus, rec.Code)
			assert.Equal(t, "application/problem+json", rec.Header().Get("Content-Type"))
			assert.Contains(t, rec.Body.String(), `"code":"`+tt.wantCode+`"`)
			assert.NotContains(t, rec.Body.String(), "secret")
			assert.NotContains(t, rec.Body.String(), "pq:")
		})
	}
}
