package create_test

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
	"github.com/ShtyroffI/tripgo-lab/internal/model"
	"github.com/ShtyroffI/tripgo-lab/internal/usecase/trip/create"
)

// runTx выполняет замыкание сразу, чтобы тело транзакции действительно сработало
func runTx(ctx context.Context, fn func(context.Context) error) error { return fn(ctx) }

func validInput() model.CreateTripInput {
	return model.CreateTripInput{
		UserID:   uuid.New(),
		DriverID: uuid.New(),
		Start:    trip.Point{Latitude: 55.75, Longitude: 37.61},
		End:      trip.Point{Latitude: 55.80, Longitude: 37.70},
		Price:    1450,
	}
}

// newUsecase: мок метрик без ожиданий, поэтому любой вызов TripCreated,
// кроме явно ожидаемого в тесте успеха, роняет тест
func newUsecase(t *testing.T) (*create.Usecase, *MocktripRepository, *MocktxManager, *Mockmetrics) {
	t.Helper()
	ctrl := gomock.NewController(t)
	repo := NewMocktripRepository(ctrl)
	tx := NewMocktxManager(ctrl)
	m := NewMockmetrics(ctrl)
	return create.New(repo, tx, m, slog.New(slog.DiscardHandler)), repo, tx, m
}

func TestUsecase_Create_Success(t *testing.T) {
	t.Parallel()

	uc, repo, tx, metrics := newUsecase(t)
	in := validInput()

	// Счётчик созданных поездок растёт ровно один раз
	metrics.EXPECT().TripCreated(gomock.Any()).Times(1)

	var saved trip.Trip
	tx.EXPECT().Do(gomock.Any(), gomock.Any()).DoAndReturn(runTx)
	repo.EXPECT().Save(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, tr trip.Trip) error {
			saved = tr
			return nil
		},
	)
	repo.EXPECT().AddStatusHistory(gomock.Any(), gomock.Any(), nil, trip.StatusActive, gomock.Any()).DoAndReturn(
		func(_ context.Context, id uuid.UUID, _ *trip.Status, _ trip.Status, _ string) error {
			assert.Equal(t, saved.ID, id)
			return nil
		},
	)

	got, err := uc.Create(t.Context(), in)

	require.NoError(t, err)
	assert.Equal(t, saved, got)
	assert.NotEqual(t, uuid.Nil, got.ID)
	assert.Equal(t, trip.StatusActive, got.Status)
	assert.Equal(t, in.UserID, got.UserID)
	assert.Equal(t, in.DriverID, got.DriverID)
	assert.Equal(t, in.Start, got.Start)
	assert.Equal(t, in.End, got.End)
	assert.Equal(t, in.Price, got.Price)
	assert.Equal(t, time.UTC, got.StartedAt.Location())
	assert.Nil(t, got.FinishedAt)
}

func TestUsecase_Create_InvalidInput(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		patch func(in *model.CreateTripInput)
		field string
	}{
		{"negative price", func(in *model.CreateTripInput) { in.Price = -1 }, "price"},
		{"nil user", func(in *model.CreateTripInput) { in.UserID = uuid.Nil }, "user_id"},
		{"nil driver", func(in *model.CreateTripInput) { in.DriverID = uuid.Nil }, "driver_id"},
		{"latitude out of range", func(in *model.CreateTripInput) { in.Start.Latitude = 91 }, "start_point.latitude"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// Моки без EXPECT: любой вызов репозитория или tx провалит тест
			uc, _, _, _ := newUsecase(t)
			in := validInput()
			tt.patch(&in)

			_, err := uc.Create(t.Context(), in)

			var vErr *trip.ValidationError
			require.ErrorAs(t, err, &vErr)
			assert.Equal(t, tt.field, vErr.Field)
		})
	}
}

func TestUsecase_Create_DriverBusy(t *testing.T) {
	t.Parallel()

	uc, repo, tx, _ := newUsecase(t)

	tx.EXPECT().Do(gomock.Any(), gomock.Any()).DoAndReturn(runTx)
	repo.EXPECT().Save(gomock.Any(), gomock.Any()).Return(trip.ErrDriverBusy)
	// AddStatusHistory не ожидается: после ошибки Save его вызывать нельзя

	_, err := uc.Create(t.Context(), validInput())

	require.ErrorIs(t, err, trip.ErrDriverBusy)
}

func TestUsecase_Create_RepositoryError(t *testing.T) {
	t.Parallel()

	dbErr := errors.New("db down")

	tests := []struct {
		name  string
		setup func(repo *MocktripRepository)
	}{
		{"save fails", func(repo *MocktripRepository) {
			repo.EXPECT().Save(gomock.Any(), gomock.Any()).Return(dbErr)
		}},
		{"history fails", func(repo *MocktripRepository) {
			repo.EXPECT().Save(gomock.Any(), gomock.Any()).Return(nil)
			repo.EXPECT().AddStatusHistory(gomock.Any(), gomock.Any(), nil, trip.StatusActive, gomock.Any()).Return(dbErr)
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			uc, repo, tx, _ := newUsecase(t)
			tx.EXPECT().Do(gomock.Any(), gomock.Any()).DoAndReturn(runTx)
			tt.setup(repo)

			_, err := uc.Create(t.Context(), validInput())

			require.ErrorIs(t, err, dbErr)
			require.NotErrorIs(t, err, trip.ErrDriverBusy)
			var vErr *trip.ValidationError
			assert.NotErrorAs(t, err, &vErr)
		})
	}
}
