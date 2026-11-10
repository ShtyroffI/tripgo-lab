package finish_test

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
	"github.com/ShtyroffI/tripgo-lab/internal/http/trip/finish"
)

func call(t *testing.T, id uuid.UUID, uc *Mockusecase) *httptest.ResponseRecorder {
	t.Helper()
	h := finish.New(uc, slog.New(slog.DiscardHandler))
	rec := httptest.NewRecorder()
	h.FinishTrip(rec, httptest.NewRequest(http.MethodPost, "/api/v1/trips/"+id.String()+"/finish", nil), id)
	return rec
}

func TestHandler_FinishTrip_Success(t *testing.T) {
	t.Parallel()

	uc := NewMockusecase(gomock.NewController(t))
	id := uuid.New()
	finishedAt := time.Date(2026, 1, 2, 4, 0, 0, 0, time.UTC)
	uc.EXPECT().Finish(gomock.Any(), id).Return(trip.Trip{
		ID:         id,
		Status:     trip.StatusCompleted,
		StartedAt:  finishedAt.Add(-time.Hour),
		FinishedAt: &finishedAt,
	}, nil)

	rec := call(t, id, uc)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "application/json", rec.Header().Get("Content-Type"))
	var got api.Trip
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
	assert.Equal(t, id, got.Id)
	assert.Equal(t, api.TripStatus("completed"), got.Status)
	require.NotNil(t, got.FinishedAt)
	assert.True(t, finishedAt.Equal(*got.FinishedAt))
}

func TestHandler_FinishTrip_Errors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		err        error
		wantStatus int
		wantCode   string
	}{
		{"completed trip returns 409 trip_completed", trip.ErrTripCompleted, http.StatusConflict, "trip_completed"},
		{"missing trip returns 404 trip_not_found", trip.ErrTripNotFound, http.StatusNotFound, "trip_not_found"},
		{"repository error returns 500 without details", errors.New("pq: password=secret"), http.StatusInternalServerError, "internal_error"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			uc := NewMockusecase(gomock.NewController(t))
			id := uuid.New()
			uc.EXPECT().Finish(gomock.Any(), id).Return(trip.Trip{}, tt.err)

			rec := call(t, id, uc)

			assert.Equal(t, tt.wantStatus, rec.Code)
			assert.Equal(t, "application/problem+json", rec.Header().Get("Content-Type"))
			assert.Contains(t, rec.Body.String(), `"code":"`+tt.wantCode+`"`)
			assert.NotContains(t, rec.Body.String(), "secret")
			assert.NotContains(t, rec.Body.String(), "pq:")
		})
	}
}
