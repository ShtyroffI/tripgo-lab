package finish_test

import (
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/ShtyroffI/tripgo-lab/internal/domain/trip"
	"github.com/ShtyroffI/tripgo-lab/internal/usecase/trip/finish"
)

// runTx выполняет замыкание сразу, чтобы тело транзакции действительно сработало
func runTx(ctx context.Context, fn func(context.Context) error) error { return fn(ctx) }

func newUsecase(t *testing.T) (*finish.Usecase, *MocktripRepository) {
	t.Helper()
	ctrl := gomock.NewController(t)
	repo := NewMocktripRepository(ctrl)
	tx := NewMocktxManager(ctrl)
	tx.EXPECT().Do(gomock.Any(), gomock.Any()).DoAndReturn(runTx)
	return finish.New(repo, tx, slog.New(slog.DiscardHandler)), repo
}

func activeTrip() trip.Trip {
	return trip.Trip{
		ID:        uuid.New(),
		UserID:    uuid.New(),
		DriverID:  uuid.New(),
		Price:     500,
		Status:    trip.StatusActive,
		StartedAt: time.Now().UTC(),
	}
}

func TestUsecase_Finish_ActiveTrip_Success(t *testing.T) {
	t.Parallel()

	uc, repo := newUsecase(t)
	tr := activeTrip()
	active := trip.StatusActive

	repo.EXPECT().GetForUpdate(gomock.Any(), tr.ID).Return(tr, nil)
	repo.EXPECT().SaveFinished(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, saved trip.Trip) error {
			assert.Equal(t, trip.StatusCompleted, saved.Status)
			assert.Equal(t, tr.ID, saved.ID)
			require.NotNil(t, saved.FinishedAt)
			return nil
		},
	)
	repo.EXPECT().AddStatusHistory(gomock.Any(), tr.ID, &active, trip.StatusCompleted, gomock.Any()).Return(nil)

	got, err := uc.Finish(t.Context(), tr.ID)

	require.NoError(t, err)
	assert.Equal(t, trip.StatusCompleted, got.Status)
	require.NotNil(t, got.FinishedAt)
	assert.Equal(t, time.UTC, got.FinishedAt.Location())
	assert.Equal(t, tr.ID, got.ID)
}

func TestUsecase_Finish_AlreadyCompleted_ErrTripCompleted(t *testing.T) {
	t.Parallel()

	uc, repo := newUsecase(t)
	tr := activeTrip()
	tr.Status = trip.StatusCompleted
	// SaveFinished и AddStatusHistory не ожидаются: gomock упадёт при вызове
	repo.EXPECT().GetForUpdate(gomock.Any(), tr.ID).Return(tr, nil)

	_, err := uc.Finish(t.Context(), tr.ID)

	require.ErrorIs(t, err, trip.ErrTripCompleted)
}

func TestUsecase_Finish_NotFound(t *testing.T) {
	t.Parallel()

	uc, repo := newUsecase(t)
	id := uuid.New()
	repo.EXPECT().GetForUpdate(gomock.Any(), id).Return(trip.Trip{}, trip.ErrTripNotFound)

	_, err := uc.Finish(t.Context(), id)

	require.ErrorIs(t, err, trip.ErrTripNotFound)
}

func TestUsecase_Finish_SaveFinishedRace_ErrTripCompleted(t *testing.T) {
	t.Parallel()

	uc, repo := newUsecase(t)
	tr := activeTrip()
	repo.EXPECT().GetForUpdate(gomock.Any(), tr.ID).Return(tr, nil)
	repo.EXPECT().SaveFinished(gomock.Any(), gomock.Any()).Return(trip.ErrTripCompleted)

	_, err := uc.Finish(t.Context(), tr.ID)

	require.ErrorIs(t, err, trip.ErrTripCompleted)
}

func TestUsecase_Finish_RepositoryError(t *testing.T) {
	t.Parallel()

	dbErr := errors.New("db down")

	tests := []struct {
		name  string
		setup func(repo *MocktripRepository, tr trip.Trip)
	}{
		{"get for update fails", func(repo *MocktripRepository, tr trip.Trip) {
			repo.EXPECT().GetForUpdate(gomock.Any(), tr.ID).Return(trip.Trip{}, dbErr)
		}},
		{"save finished fails", func(repo *MocktripRepository, tr trip.Trip) {
			repo.EXPECT().GetForUpdate(gomock.Any(), tr.ID).Return(tr, nil)
			repo.EXPECT().SaveFinished(gomock.Any(), gomock.Any()).Return(dbErr)
		}},
		{"history fails", func(repo *MocktripRepository, tr trip.Trip) {
			repo.EXPECT().GetForUpdate(gomock.Any(), tr.ID).Return(tr, nil)
			repo.EXPECT().SaveFinished(gomock.Any(), gomock.Any()).Return(nil)
			repo.EXPECT().AddStatusHistory(gomock.Any(), tr.ID, gomock.Any(), trip.StatusCompleted, gomock.Any()).Return(dbErr)
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			uc, repo := newUsecase(t)
			tr := activeTrip()
			tt.setup(repo, tr)

			_, err := uc.Finish(t.Context(), tr.ID)

			require.ErrorIs(t, err, dbErr)
			require.NotErrorIs(t, err, trip.ErrTripCompleted)
			require.NotErrorIs(t, err, trip.ErrTripNotFound)
		})
	}
}
