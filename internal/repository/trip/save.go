package trip

import (
	"context"
	"errors"
	"fmt"

	sq "github.com/Masterminds/squirrel"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/ShtyroffI/tripgo-lab/internal/domain/trip"
)

// Save вставляет новую поездку. Вторую активную поездку водителя отсекает
// частичный уникальный индекс trips_one_active_per_driver_idx: его 23505
// превращается в trip.ErrDriverBusy
func (r *Repository) Save(ctx context.Context, t trip.Trip) error {
	query, args, err := psql.Insert(tripsTable).
		Columns(
			"id", "user_id", "driver_id",
			"start_latitude", "start_longitude", "end_latitude", "end_longitude",
			"price", "status", "started_at",
		).
		Values(
			t.ID, t.UserID, t.DriverID,
			t.Start.Latitude, t.Start.Longitude, t.End.Latitude, t.End.Longitude,
			t.Price, t.Status, t.StartedAt,
		).
		ToSql()
	if err != nil {
		return fmt.Errorf("build insert trip query: %w", err)
	}

	ctx, cancel := r.withTimeout(ctx)
	defer cancel()

	if _, err := r.executor(ctx).Exec(ctx, query, args...); err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == pgUniqueViolation {
			return trip.ErrDriverBusy
		}
		return fmt.Errorf("insert trip: %w", err)
	}
	return nil
}

// AddStatusHistory пишет переход в журнал. from равен nil при создании
func (r *Repository) AddStatusHistory(ctx context.Context, tripID uuid.UUID, from *trip.Status, to trip.Status, reason string) error {
	query, args, err := psql.Insert(historyTable).
		Columns("trip_id", "from_status", "to_status", "reason").
		Values(tripID, from, to, reason).
		ToSql()
	if err != nil {
		return fmt.Errorf("build insert trip_status_history query: %w", err)
	}

	ctx, cancel := r.withTimeout(ctx)
	defer cancel()

	if _, err := r.executor(ctx).Exec(ctx, query, args...); err != nil {
		return fmt.Errorf("insert trip_status_history: %w", err)
	}
	return nil
}

// SaveFinished сохраняет завершение поездки, которое уже разрешил домен
// (trip.Finish). Условие status = 'active' в самом UPDATE остаётся
// страховкой от гонки: из двух параллельных запросов статус сменит один,
// второй получит 0 строк и trip.ErrTripCompleted
func (r *Repository) SaveFinished(ctx context.Context, t trip.Trip) error {
	query, args, err := psql.Update(tripsTable).
		Set(colStatus, t.Status).
		Set("finished_at", t.FinishedAt).
		Set("updated_at", sq.Expr("now()")).
		Where(sq.Eq{"id": t.ID, colStatus: trip.StatusActive}).
		ToSql()
	if err != nil {
		return fmt.Errorf("build finish trip query: %w", err)
	}

	ctx, cancel := r.withTimeout(ctx)
	defer cancel()

	tag, err := r.executor(ctx).Exec(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("finish trip: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return trip.ErrTripCompleted
	}
	return nil
}
