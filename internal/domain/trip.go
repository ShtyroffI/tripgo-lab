// Package domain: поездка и доменные ошибки
package domain

import (
	"errors"
	"time"

	"github.com/google/uuid"
)

// TripStatus: разрешён один переход, из active в completed
type TripStatus string

const (
	TripStatusActive    TripStatus = "active"
	TripStatusCompleted TripStatus = "completed"
)

type Trip struct {
	ID       uuid.UUID
	UserID   uuid.UUID
	DriverID uuid.UUID

	StartLatitude  float64
	StartLongitude float64
	EndLatitude    float64
	EndLongitude   float64

	Price int64

	Status TripStatus

	StartedAt  time.Time
	FinishedAt *time.Time
}

var (
	ErrDriverBusy    = errors.New("driver already has an active trip")
	ErrTripNotFound  = errors.New("trip not found")
	ErrTripCompleted = errors.New("trip already completed")
)
