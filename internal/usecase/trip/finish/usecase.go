// Package finish: завершение поездки
package finish

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"github.com/ShtyroffI/tripgo-lab/internal/domain/trip"
)

const historyReason = "trip finished"

type Usecase struct {
	trips   tripRepository
	tx      txManager
	metrics metrics
	log     *slog.Logger
}

func New(trips tripRepository, tx txManager, m metrics, log *slog.Logger) *Usecase {
	return &Usecase{trips: trips, tx: tx, metrics: m, log: log}
}

// Finish в одной транзакции: блокирует строку (FOR UPDATE), даёт домену
// решить, можно ли завершить, сохраняет результат и пишет журнал.
// Параллельный finish ждёт блокировку и получает trip.ErrTripCompleted
func (u *Usecase) Finish(ctx context.Context, id uuid.UUID) (trip.Trip, error) {
	var finished trip.Trip

	err := u.tx.Do(ctx, func(ctx context.Context) error {
		t, err := u.trips.GetForUpdate(ctx, id)
		if err != nil {
			return err
		}

		from := t.Status
		if err := t.Finish(time.Now()); err != nil {
			return err
		}

		if err := u.trips.SaveFinished(ctx, t); err != nil {
			return err
		}
		if err := u.trips.AddStatusHistory(ctx, t.ID, &from, t.Status, historyReason); err != nil {
			return err
		}

		finished = t
		return nil
	})
	if err != nil {
		return trip.Trip{}, fmt.Errorf("finish trip: %w", err)
	}

	u.metrics.TripCompleted(ctx)
	u.log.InfoContext(ctx, "trip completed", "trip_id", finished.ID)
	return finished, nil
}
