// Package get: получение поездки
package get

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/ShtyroffI/tripgo-lab/internal/domain/trip"
)

type Usecase struct {
	trips tripRepository
}

func New(trips tripRepository) *Usecase {
	return &Usecase{trips: trips}
}

// Get читает поездку одним запросом, транзакция не нужна
func (u *Usecase) Get(ctx context.Context, id uuid.UUID) (trip.Trip, error) {
	t, err := u.trips.Get(ctx, id)
	if err != nil {
		return trip.Trip{}, fmt.Errorf("get trip: %w", err)
	}
	return t, nil
}
