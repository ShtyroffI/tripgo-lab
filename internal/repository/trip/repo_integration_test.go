//go:build integration

package trip_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ShtyroffI/tripgo-lab/internal/domain/trip"
)

const testTimeout = 30 * time.Second

func testCtx(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	t.Cleanup(cancel)
	return ctx
}

// newTrip собирает новую активную поездку со свежими id
func newTrip(t *testing.T, driverID uuid.UUID) trip.Trip {
	t.Helper()
	tr, err := trip.New(uuid.New(), trip.Params{
		UserID:   uuid.New(),
		DriverID: driverID,
		Start:    trip.Point{Latitude: 55.751244, Longitude: 37.618423},
		End:      trip.Point{Latitude: 59.9342802, Longitude: 30.3350986},
		Price:    12345,
	}, time.Now())
	require.NoError(t, err)
	return tr
}

// cleanup удаляет поездку, история уходит каскадом
func cleanup(t *testing.T, ids ...uuid.UUID) {
	t.Helper()
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
		defer cancel()
		for _, id := range ids {
			_, err := testPool.Exec(ctx, `DELETE FROM trips WHERE id = $1`, id)
			assert.NoError(t, err)
		}
	})
}

func countTrips(ctx context.Context, t *testing.T, id uuid.UUID) int {
	t.Helper()
	var n int
	require.NoError(t, testPool.QueryRow(ctx, `SELECT count(*) FROM trips WHERE id = $1`, id).Scan(&n))
	return n
}

func countHistory(ctx context.Context, t *testing.T, id uuid.UUID) int {
	t.Helper()
	var n int
	require.NoError(t, testPool.QueryRow(ctx, `SELECT count(*) FROM trip_status_history WHERE trip_id = $1`, id).Scan(&n))
	return n
}

// finish: полный сценарий завершения в одной транзакции
func finish(ctx context.Context, id uuid.UUID) error {
	return testTx.Do(ctx, func(ctx context.Context) error {
		tr, err := testRepo.GetForUpdate(ctx, id)
		if err != nil {
			return err
		}
		if err := tr.Finish(time.Now()); err != nil {
			return err
		}
		if err := testRepo.SaveFinished(ctx, tr); err != nil {
			return err
		}
		from := trip.StatusActive
		return testRepo.AddStatusHistory(ctx, id, &from, trip.StatusCompleted, "finished")
	})
}

func TestSaveAndGetRoundTrip(t *testing.T) {
	ctx := testCtx(t)
	want := newTrip(t, uuid.New())
	cleanup(t, want.ID)

	require.NoError(t, testRepo.Save(ctx, want))

	got, err := testRepo.Get(ctx, want.ID)
	require.NoError(t, err)

	assert.Equal(t, want.ID, got.ID)
	assert.Equal(t, want.UserID, got.UserID)
	assert.Equal(t, want.DriverID, got.DriverID)
	assert.Equal(t, want.Start, got.Start)
	assert.Equal(t, want.End, got.End)
	assert.Equal(t, want.Price, got.Price)
	assert.Equal(t, trip.StatusActive, got.Status)
	assert.WithinDuration(t, want.StartedAt, got.StartedAt, time.Millisecond)
	assert.Equal(t, time.UTC, got.StartedAt.Location())
	assert.Nil(t, got.FinishedAt)
}

func TestGetUnknownTripReturnsNotFound(t *testing.T) {
	ctx := testCtx(t)

	_, err := testRepo.Get(ctx, uuid.New())
	require.ErrorIs(t, err, trip.ErrTripNotFound)
}

func TestGetForUpdateUnknownTripReturnsNotFound(t *testing.T) {
	ctx := testCtx(t)

	err := testTx.Do(ctx, func(ctx context.Context) error {
		_, err := testRepo.GetForUpdate(ctx, uuid.New())
		return err
	})
	require.ErrorIs(t, err, trip.ErrTripNotFound)
}

func TestSaveSecondActiveTripForDriverReturnsDriverBusy(t *testing.T) {
	ctx := testCtx(t)
	driverID := uuid.New()
	first, second := newTrip(t, driverID), newTrip(t, driverID)
	cleanup(t, first.ID, second.ID)

	require.NoError(t, testRepo.Save(ctx, first))

	err := testRepo.Save(ctx, second)
	require.ErrorIs(t, err, trip.ErrDriverBusy)
	assert.Equal(t, 0, countTrips(ctx, t, second.ID))
}

func TestSaveAfterFinishedTripForSameDriverSucceeds(t *testing.T) {
	ctx := testCtx(t)
	driverID := uuid.New()
	first, second := newTrip(t, driverID), newTrip(t, driverID)
	cleanup(t, first.ID, second.ID)

	require.NoError(t, testRepo.Save(ctx, first))
	require.NoError(t, finish(ctx, first.ID))

	require.NoError(t, testRepo.Save(ctx, second))
	got, err := testRepo.Get(ctx, second.ID)
	require.NoError(t, err)
	assert.Equal(t, trip.StatusActive, got.Status)
}

func TestFinishFlowInTransaction(t *testing.T) {
	ctx := testCtx(t)
	tr := newTrip(t, uuid.New())
	cleanup(t, tr.ID)

	require.NoError(t, testTx.Do(ctx, func(ctx context.Context) error {
		from := (*trip.Status)(nil)
		if err := testRepo.Save(ctx, tr); err != nil {
			return err
		}
		return testRepo.AddStatusHistory(ctx, tr.ID, from, trip.StatusActive, "created")
	}))

	before := time.Now()
	require.NoError(t, finish(ctx, tr.ID))

	got, err := testRepo.Get(ctx, tr.ID)
	require.NoError(t, err)
	assert.Equal(t, trip.StatusCompleted, got.Status)
	require.NotNil(t, got.FinishedAt)
	assert.WithinDuration(t, before, *got.FinishedAt, 10*time.Second)
	assert.Equal(t, time.UTC, got.FinishedAt.Location())

	// В журнале: создание (from NULL) и завершение
	rows, err := testPool.Query(ctx,
		`SELECT from_status, to_status FROM trip_status_history WHERE trip_id = $1 ORDER BY id`, tr.ID)
	require.NoError(t, err)
	defer rows.Close()

	type entry struct {
		from *string
		to   string
	}
	var entries []entry
	for rows.Next() {
		var e entry
		require.NoError(t, rows.Scan(&e.from, &e.to))
		entries = append(entries, e)
	}
	require.NoError(t, rows.Err())

	require.Len(t, entries, 2)
	assert.Nil(t, entries[0].from)
	assert.Equal(t, "active", entries[0].to)
	require.NotNil(t, entries[1].from)
	assert.Equal(t, "active", *entries[1].from)
	assert.Equal(t, "completed", entries[1].to)
}

func TestSaveFinishedOnCompletedTripReturnsTripCompleted(t *testing.T) {
	ctx := testCtx(t)
	tr := newTrip(t, uuid.New())
	cleanup(t, tr.ID)

	require.NoError(t, testRepo.Save(ctx, tr))
	require.NoError(t, finish(ctx, tr.ID))

	// Домен ещё считает поездку активной, а в бд она уже завершена
	require.NoError(t, tr.Finish(time.Now()))
	err := testRepo.SaveFinished(ctx, tr)
	require.ErrorIs(t, err, trip.ErrTripCompleted)
}

func TestCreateRollsBackBothInsertsOnHistoryFailure(t *testing.T) {
	ctx := testCtx(t)
	tr := newTrip(t, uuid.New())
	cleanup(t, tr.ID)

	err := testTx.Do(ctx, func(ctx context.Context) error {
		if err := testRepo.Save(ctx, tr); err != nil {
			return err
		}
		// Невалидный статус нарушает CHECK по to_status
		return testRepo.AddStatusHistory(ctx, tr.ID, nil, trip.Status("bogus"), "created")
	})
	require.Error(t, err)

	_, err = testRepo.Get(ctx, tr.ID)
	require.ErrorIs(t, err, trip.ErrTripNotFound)
	assert.Equal(t, 0, countTrips(ctx, t, tr.ID))
	assert.Equal(t, 0, countHistory(ctx, t, tr.ID))
}

func TestCallsOutsideTransactionUsePool(t *testing.T) {
	ctx := testCtx(t)
	tr := newTrip(t, uuid.New())
	cleanup(t, tr.ID)

	require.NoError(t, testRepo.Save(ctx, tr))
	require.NoError(t, testRepo.AddStatusHistory(ctx, tr.ID, nil, trip.StatusActive, "created"))

	got, err := testRepo.Get(ctx, tr.ID)
	require.NoError(t, err)
	assert.Equal(t, tr.ID, got.ID)
	assert.Equal(t, 1, countHistory(ctx, t, tr.ID))
}

func TestConcurrentFinishExactlyOneSucceeds(t *testing.T) {
	ctx := testCtx(t)
	tr := newTrip(t, uuid.New())
	cleanup(t, tr.ID)
	require.NoError(t, testRepo.Save(ctx, tr))

	const workers = 2
	var (
		wg    sync.WaitGroup
		start = make(chan struct{})
		errs  = make([]error, workers)
	)
	for i := range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			errs[i] = finish(ctx, tr.ID)
		}()
	}
	close(start)
	wg.Wait()

	var ok, completed int
	for _, err := range errs {
		switch {
		case err == nil:
			ok++
		case assert.ErrorIs(t, err, trip.ErrTripCompleted):
			completed++
		}
	}
	assert.Equal(t, 1, ok)
	assert.Equal(t, workers-1, completed)
	assert.Equal(t, 1, countHistory(ctx, t, tr.ID))
}
