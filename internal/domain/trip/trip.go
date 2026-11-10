// Package trip: поездка, её статусы, переходы и правила. Ничего не знает
// про транспорт, бд и телеметрию
package trip

import (
	"time"

	"github.com/google/uuid"
)

// Status: разрешён один переход, из active в completed
type Status string

const (
	StatusActive    Status = "active"
	StatusCompleted Status = "completed"
)

// Допустимые диапазоны координат (WGS 84)
const (
	maxLatitude  = 90
	maxLongitude = 180
)

type Point struct {
	Latitude  float64
	Longitude float64
}

type Trip struct {
	ID       uuid.UUID
	UserID   uuid.UUID
	DriverID uuid.UUID

	Start Point
	End   Point

	Price int64

	Status Status

	StartedAt  time.Time
	FinishedAt *time.Time
}

// Params: из чего создаётся новая поездка
type Params struct {
	UserID   uuid.UUID
	DriverID uuid.UUID
	Start    Point
	End      Point
	Price    int64
}

// New проверяет правила и создаёт активную поездку. id и время старта
// передаются снаружи, чтобы домен не зависел от генератора и часов
func New(id uuid.UUID, p Params, startedAt time.Time) (Trip, error) {
	if err := p.Validate(); err != nil {
		return Trip{}, err
	}

	return Trip{
		ID:        id,
		UserID:    p.UserID,
		DriverID:  p.DriverID,
		Start:     p.Start,
		End:       p.End,
		Price:     p.Price,
		Status:    StatusActive,
		StartedAt: startedAt.UTC(),
	}, nil
}

// Validate возвращает *ValidationError на первое нарушенное правило
func (p Params) Validate() error {
	if p.UserID == uuid.Nil {
		return newValidationError("user_id", "must not be empty")
	}
	if p.DriverID == uuid.Nil {
		return newValidationError("driver_id", "must not be empty")
	}
	if p.Price < 0 {
		return newValidationError("price", "must be >= 0")
	}
	if err := p.Start.validate("start_point"); err != nil {
		return err
	}
	return p.End.validate("end_point")
}

func (p Point) validate(field string) error {
	if p.Latitude < -maxLatitude || p.Latitude > maxLatitude {
		return newValidationError(field+".latitude", "must be between -90 and 90")
	}
	if p.Longitude < -maxLongitude || p.Longitude > maxLongitude {
		return newValidationError(field+".longitude", "must be between -180 and 180")
	}
	return nil
}

// Finish завершает активную поездку. Повторное завершение запрещено
func (t *Trip) Finish(at time.Time) error {
	if t.Status != StatusActive {
		return ErrTripCompleted
	}

	finishedAt := at.UTC()
	t.Status = StatusCompleted
	t.FinishedAt = &finishedAt
	return nil
}
