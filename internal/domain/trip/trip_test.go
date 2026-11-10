package trip_test

import (
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ShtyroffI/tripgo-lab/internal/domain/trip"
)

func validParams() trip.Params {
	return trip.Params{
		UserID:   uuid.MustParse("5cb72c04-7650-45c9-a79b-bcdba0631e0c"),
		DriverID: uuid.MustParse("8860b315-ec86-42eb-a17c-7c163d721ff5"),
		Start:    trip.Point{Latitude: 59.9398, Longitude: 30.3146},
		End:      trip.Point{Latitude: 59.9290, Longitude: 30.3626},
		Price:    1450,
	}
}

func TestNew(t *testing.T) {
	id := uuid.New()
	moscow := time.FixedZone("MSK", 3*60*60)
	startedAt := time.Date(2026, 8, 24, 15, 0, 0, 0, moscow)

	got, err := trip.New(id, validParams(), startedAt)
	require.NoError(t, err)

	p := validParams()
	assert.Equal(t, id, got.ID)
	assert.Equal(t, p.UserID, got.UserID)
	assert.Equal(t, p.DriverID, got.DriverID)
	assert.Equal(t, p.Start, got.Start)
	assert.Equal(t, p.End, got.End)
	assert.Equal(t, p.Price, got.Price)
	assert.Equal(t, trip.StatusActive, got.Status)
	assert.Nil(t, got.FinishedAt)
	// Время в ответах всегда UTC: переводим уже при создании
	assert.Equal(t, time.UTC, got.StartedAt.Location())
	assert.True(t, startedAt.Equal(got.StartedAt))
}

func TestParamsValidate(t *testing.T) {
	tests := []struct {
		name      string
		modify    func(p *trip.Params)
		wantField string // пусто, если ошибки быть не должно
	}{
		{name: "valid", modify: func(*trip.Params) {}},
		{name: "zero price is allowed", modify: func(p *trip.Params) { p.Price = 0 }},
		{name: "zero coordinates are allowed", modify: func(p *trip.Params) { p.Start = trip.Point{} }},
		{name: "latitude on the border", modify: func(p *trip.Params) { p.Start.Latitude = -90; p.End.Latitude = 90 }},
		{name: "longitude on the border", modify: func(p *trip.Params) { p.Start.Longitude = -180; p.End.Longitude = 180 }},
		{name: "empty user_id", modify: func(p *trip.Params) { p.UserID = uuid.Nil }, wantField: "user_id"},
		{name: "empty driver_id", modify: func(p *trip.Params) { p.DriverID = uuid.Nil }, wantField: "driver_id"},
		{name: "negative price", modify: func(p *trip.Params) { p.Price = -1 }, wantField: "price"},
		{name: "start latitude too big", modify: func(p *trip.Params) { p.Start.Latitude = 90.0001 }, wantField: "start_point.latitude"},
		{name: "start latitude too small", modify: func(p *trip.Params) { p.Start.Latitude = -90.0001 }, wantField: "start_point.latitude"},
		{name: "start longitude too big", modify: func(p *trip.Params) { p.Start.Longitude = 180.0001 }, wantField: "start_point.longitude"},
		{name: "end latitude out of range", modify: func(p *trip.Params) { p.End.Latitude = -91 }, wantField: "end_point.latitude"},
		{name: "end longitude out of range", modify: func(p *trip.Params) { p.End.Longitude = -181 }, wantField: "end_point.longitude"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := validParams()
			tt.modify(&p)

			err := p.Validate()
			if tt.wantField == "" {
				assert.NoError(t, err)
				return
			}

			var vErr *trip.ValidationError
			require.ErrorAs(t, err, &vErr)
			assert.Equal(t, tt.wantField, vErr.Field)
			assert.NotEmpty(t, vErr.Reason)
		})
	}
}

func TestNewRejectsInvalidParams(t *testing.T) {
	p := validParams()
	p.Price = -10

	got, err := trip.New(uuid.New(), p, time.Now())

	var vErr *trip.ValidationError
	require.ErrorAs(t, err, &vErr)
	assert.Equal(t, "price", vErr.Field)
	assert.Equal(t, trip.Trip{}, got)
}

func TestValidationErrorMessage(t *testing.T) {
	err := &trip.ValidationError{Field: "price", Reason: "must be >= 0"}
	assert.Equal(t, "price: must be >= 0", err.Error())
}

func TestFinish(t *testing.T) {
	finishedAt := time.Date(2026, 8, 24, 13, 0, 0, 0, time.FixedZone("MSK", 3*60*60))
	earlier := time.Date(2026, 8, 24, 9, 0, 0, 0, time.UTC)

	tests := []struct {
		name           string
		trip           trip.Trip
		wantErr        error
		wantStatus     trip.Status
		wantFinishedAt *time.Time
	}{
		{
			name:           "active trip becomes completed",
			trip:           trip.Trip{Status: trip.StatusActive},
			wantStatus:     trip.StatusCompleted,
			wantFinishedAt: &finishedAt,
		},
		{
			name:           "completed trip cannot be finished again",
			trip:           trip.Trip{Status: trip.StatusCompleted, FinishedAt: &earlier},
			wantErr:        trip.ErrTripCompleted,
			wantStatus:     trip.StatusCompleted,
			wantFinishedAt: &earlier,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tr := tt.trip

			err := tr.Finish(finishedAt)

			if tt.wantErr != nil {
				assert.True(t, errors.Is(err, tt.wantErr), "want %v, got %v", tt.wantErr, err)
			} else {
				require.NoError(t, err)
			}
			assert.Equal(t, tt.wantStatus, tr.Status)
			require.NotNil(t, tr.FinishedAt)
			assert.True(t, tt.wantFinishedAt.Equal(*tr.FinishedAt))
			assert.Equal(t, time.UTC, tr.FinishedAt.Location())
		})
	}
}
