//go:generate go tool mockgen -source ${GOFILE} -destination mocks_test.go -package ${GOPACKAGE}_test
package finish

import (
	"context"

	"github.com/google/uuid"

	"github.com/ShtyroffI/tripgo-lab/internal/domain/trip"
)

type (
	tripRepository interface {
		GetForUpdate(ctx context.Context, id uuid.UUID) (trip.Trip, error)
		SaveFinished(ctx context.Context, t trip.Trip) error
		AddStatusHistory(ctx context.Context, tripID uuid.UUID, from *trip.Status, to trip.Status, reason string) error
	}

	txManager interface {
		Do(ctx context.Context, fn func(ctx context.Context) error) error
	}
)
