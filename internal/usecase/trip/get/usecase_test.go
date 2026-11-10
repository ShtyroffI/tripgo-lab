package get_test

import (
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/ShtyroffI/tripgo-lab/internal/domain/trip"
	"github.com/ShtyroffI/tripgo-lab/internal/usecase/trip/get"
)

func TestUsecase_Get_Success(t *testing.T) {
	t.Parallel()

	repo := NewMocktripRepository(gomock.NewController(t))
	id := uuid.New()
	want := trip.Trip{ID: id, Status: trip.StatusActive, StartedAt: time.Now().UTC()}
	repo.EXPECT().Get(gomock.Any(), id).Return(want, nil)

	got, err := get.New(repo).Get(t.Context(), id)

	require.NoError(t, err)
	assert.Equal(t, want, got)
}

func TestUsecase_Get_Errors(t *testing.T) {
	t.Parallel()

	dbErr := errors.New("db down")

	tests := []struct {
		name       string
		repoErr    error
		wantTarget error
		wantDomain bool
	}{
		{"missing trip passes through as ErrTripNotFound", trip.ErrTripNotFound, trip.ErrTripNotFound, true},
		{"repository error is wrapped", dbErr, dbErr, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			repo := NewMocktripRepository(gomock.NewController(t))
			id := uuid.New()
			repo.EXPECT().Get(gomock.Any(), id).Return(trip.Trip{}, tt.repoErr)

			got, err := get.New(repo).Get(t.Context(), id)

			require.ErrorIs(t, err, tt.wantTarget)
			assert.Equal(t, tt.wantDomain, errors.Is(err, trip.ErrTripNotFound))
			assert.Equal(t, trip.Trip{}, got)
		})
	}
}
