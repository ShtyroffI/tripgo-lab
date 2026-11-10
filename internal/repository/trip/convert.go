package trip

import (
	"github.com/jackc/pgx/v5"

	"github.com/ShtyroffI/tripgo-lab/internal/domain/trip"
)

// tripColumns должен совпадать по порядку со scanTrip
var tripColumns = []string{
	"id", "user_id", "driver_id",
	"start_latitude", "start_longitude", "end_latitude", "end_longitude",
	"price", "status", "started_at", "finished_at",
}

// scanTrip переводит строку trips в доменную модель
func scanTrip(row pgx.Row) (trip.Trip, error) {
	var t trip.Trip
	err := row.Scan(
		&t.ID, &t.UserID, &t.DriverID,
		&t.Start.Latitude, &t.Start.Longitude, &t.End.Latitude, &t.End.Longitude,
		&t.Price, &t.Status, &t.StartedAt, &t.FinishedAt,
	)
	if err != nil {
		return trip.Trip{}, err
	}

	// Время отдаём в UTC, как и при создании
	t.StartedAt = t.StartedAt.UTC()
	if t.FinishedAt != nil {
		finishedAt := t.FinishedAt.UTC()
		t.FinishedAt = &finishedAt
	}
	return t, nil
}
