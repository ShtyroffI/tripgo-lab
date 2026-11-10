//go:generate go tool mockgen -source ${GOFILE} -destination mocks_test.go -package ${GOPACKAGE}_test
package get

import (
	"context"

	"github.com/google/uuid"

	"github.com/ShtyroffI/tripgo-lab/internal/domain/trip"
)

type usecase interface {
	Get(ctx context.Context, id uuid.UUID) (trip.Trip, error)
}
