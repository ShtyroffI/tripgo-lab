// Package create: создание поездки
package create

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"github.com/ShtyroffI/tripgo-lab/internal/domain/trip"
	"github.com/ShtyroffI/tripgo-lab/internal/model"
)

const historyReason = "trip created"

type Usecase struct {
	trips tripRepository
	tx    txManager
	log   *slog.Logger
}

func New(trips tripRepository, tx txManager, log *slog.Logger) *Usecase {
	return &Usecase{trips: trips, tx: tx, log: log}
}

// Create проверяет данные в домене и пишет trips и trip_status_history в
// одной транзакции: упадёт вторая вставка, откатится и первая
func (u *Usecase) Create(ctx context.Context, in model.CreateTripInput) (trip.Trip, error) {
	t, err := trip.New(uuid.New(), in.Params(), time.Now())
	if err != nil {
		return trip.Trip{}, err
	}

	err = u.tx.Do(ctx, func(ctx context.Context) error {
		if err := u.trips.Save(ctx, t); err != nil {
			return err
		}
		return u.trips.AddStatusHistory(ctx, t.ID, nil, trip.StatusActive, historyReason)
	})
	if err != nil {
		return trip.Trip{}, fmt.Errorf("create trip: %w", err)
	}

	u.log.InfoContext(ctx, "trip created", "trip_id", t.ID)
	return t, nil
}
