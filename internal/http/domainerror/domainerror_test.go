package domainerror_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ShtyroffI/tripgo-lab/api"
	"github.com/ShtyroffI/tripgo-lab/internal/domain/trip"
	"github.com/ShtyroffI/tripgo-lab/internal/http/domainerror"
)

const (
	secretText  = "pq: password=secret relation trips ..."
	codeInvalid = "invalid_request"
	codeMissing = "trip_not_found"
	fieldPrice  = "price"
)

func TestProblem(t *testing.T) {
	t.Parallel()

	vErr := &trip.ValidationError{Field: fieldPrice, Reason: "must be >= 0"}

	tests := []struct {
		name       string
		err        error
		wantStatus int
		wantCode   string
		wantDetail string
	}{
		{"validation error", vErr, http.StatusBadRequest, codeInvalid, fieldPrice},
		{"wrapped validation error", fmt.Errorf("create trip: %w", vErr), http.StatusBadRequest, codeInvalid, fieldPrice},
		{"bad request", &domainerror.BadRequestError{Detail: "body is broken"}, http.StatusBadRequest, codeInvalid, "body is broken"},
		{"trip not found", trip.ErrTripNotFound, http.StatusNotFound, codeMissing, ""},
		{"wrapped trip not found", fmt.Errorf("get trip: %w", trip.ErrTripNotFound), http.StatusNotFound, codeMissing, ""},
		{"trip completed", trip.ErrTripCompleted, http.StatusConflict, "trip_completed", ""},
		{"driver busy", trip.ErrDriverBusy, http.StatusConflict, "driver_busy", ""},
		{"wrapped driver busy", fmt.Errorf("create trip: %w", trip.ErrDriverBusy), http.StatusConflict, "driver_busy", ""},
		{"unknown error", errors.New(secretText), http.StatusInternalServerError, "internal_error", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			status, body := domainerror.Problem(tt.err)

			assert.Equal(t, tt.wantStatus, status)
			assert.Equal(t, tt.wantStatus, int(body.Status))
			assert.Equal(t, tt.wantCode, body.Code)
			assert.NotEmpty(t, body.Type)
			assert.NotEmpty(t, body.Title)
			if tt.wantDetail != "" {
				require.NotNil(t, body.Detail)
				assert.Contains(t, *body.Detail, tt.wantDetail)
			}
		})
	}
}

func TestProblem_UnknownErrorHidesDetails(t *testing.T) {
	t.Parallel()

	_, body := domainerror.Problem(errors.New(secretText))

	raw, err := json.Marshal(body)
	require.NoError(t, err)
	assert.NotContains(t, string(raw), "secret")
	assert.NotContains(t, string(raw), "relation")
}

func TestWrite(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		err        error
		wantStatus int
		wantCode   string
	}{
		{"not found", trip.ErrTripNotFound, http.StatusNotFound, codeMissing},
		{"internal error", errors.New(secretText), http.StatusInternalServerError, "internal_error"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "/api/v1/trips/42", nil)

			domainerror.Write(rec, req, slog.New(slog.DiscardHandler), tt.err)

			assert.Equal(t, tt.wantStatus, rec.Code)
			assert.Equal(t, "application/problem+json", rec.Header().Get("Content-Type"))
			assert.NotContains(t, rec.Body.String(), "secret")

			var body api.Problem
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
			assert.Equal(t, tt.wantCode, body.Code)
			assert.Equal(t, tt.wantStatus, int(body.Status))
			require.NotNil(t, body.Instance)
			assert.Equal(t, "/api/v1/trips/42", *body.Instance)
		})
	}
}
