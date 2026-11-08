// Запросы к PostgreSQL через squirrel и pgx
package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	sq "github.com/Masterminds/squirrel"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ShtyroffI/tripgo-lab/internal/domain"
	"github.com/ShtyroffI/tripgo-lab/internal/txmanager"
)

var psql = sq.StatementBuilder.PlaceholderFormat(sq.Dollar)

// pgUniqueViolation: код ошибки PostgreSQL 23505
const pgUniqueViolation = "23505"

// tripColumns должен совпадать по порядку со scanTrip
var tripColumns = []string{
	"id", "user_id", "driver_id",
	"start_latitude", "start_longitude", "end_latitude", "end_longitude",
	"price", "status", "started_at", "finished_at",
}

type TripRepository struct {
	pool         *pgxpool.Pool
	queryTimeout time.Duration
}

func NewTripRepository(pool *pgxpool.Pool, queryTimeout time.Duration) *TripRepository {
	return &TripRepository{pool: pool, queryTimeout: queryTimeout}
}

func (r *TripRepository) executor(ctx context.Context) txmanager.Executor {
	return txmanager.ExecutorFromContext(ctx, r.pool)
}

// Create: 23505 превращается в ErrDriverBusy
func (r *TripRepository) Create(ctx context.Context, t domain.Trip) error {
	query, args, err := psql.Insert("trips").
		Columns(
			"id", "user_id", "driver_id",
			"start_latitude", "start_longitude", "end_latitude", "end_longitude",
			"price", "status", "started_at",
		).
		Values(
			t.ID, t.UserID, t.DriverID,
			t.StartLatitude, t.StartLongitude, t.EndLatitude, t.EndLongitude,
			t.Price, t.Status, t.StartedAt,
		).
		ToSql()
	if err != nil {
		return fmt.Errorf("build insert trip query: %w", err)
	}

	ctx, cancel := context.WithTimeout(ctx, r.queryTimeout)
	defer cancel()

	if _, err := r.executor(ctx).Exec(ctx, query, args...); err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == pgUniqueViolation {
			return domain.ErrDriverBusy
		}
		return fmt.Errorf("insert trip: %w", err)
	}
	return nil
}

// AddStatusHistory: fromStatus равен nil при создании
func (r *TripRepository) AddStatusHistory(ctx context.Context, tripID uuid.UUID, fromStatus *domain.TripStatus, toStatus domain.TripStatus, reason string) error {
	query, args, err := psql.Insert("trip_status_history").
		Columns("trip_id", "from_status", "to_status", "reason").
		Values(tripID, fromStatus, toStatus, reason).
		ToSql()
	if err != nil {
		return fmt.Errorf("build insert trip_status_history query: %w", err)
	}

	ctx, cancel := context.WithTimeout(ctx, r.queryTimeout)
	defer cancel()

	if _, err := r.executor(ctx).Exec(ctx, query, args...); err != nil {
		return fmt.Errorf("insert trip_status_history: %w", err)
	}
	return nil
}

func (r *TripRepository) GetByID(ctx context.Context, id uuid.UUID) (domain.Trip, error) {
	query, args, err := psql.Select(tripColumns...).
		From("trips").
		Where(sq.Eq{"id": id}).
		ToSql()
	if err != nil {
		return domain.Trip{}, fmt.Errorf("build select trip query: %w", err)
	}

	ctx, cancel := context.WithTimeout(ctx, r.queryTimeout)
	defer cancel()

	row := r.executor(ctx).QueryRow(ctx, query, args...)
	t, err := scanTrip(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Trip{}, domain.ErrTripNotFound
	}
	if err != nil {
		return domain.Trip{}, fmt.Errorf("select trip: %w", err)
	}
	return t, nil
}

// Finish проверяет статус прямо в UPDATE, поэтому параллельный finish
// выиграет только один раз. При 0 строк читаем поездку, чтобы отличить
// 404 от 409
func (r *TripRepository) Finish(ctx context.Context, id uuid.UUID) (domain.Trip, error) {
	query, args, err := psql.Update("trips").
		Set("status", domain.TripStatusCompleted).
		Set("finished_at", sq.Expr("now()")).
		Set("updated_at", sq.Expr("now()")).
		Where(sq.Eq{"id": id, "status": domain.TripStatusActive}).
		ToSql()
	if err != nil {
		return domain.Trip{}, fmt.Errorf("build finish trip query: %w", err)
	}

	execCtx, cancel := context.WithTimeout(ctx, r.queryTimeout)
	defer cancel()

	tag, err := r.executor(execCtx).Exec(execCtx, query, args...)
	if err != nil {
		return domain.Trip{}, fmt.Errorf("finish trip: %w", err)
	}

	if tag.RowsAffected() == 0 {
		if _, err := r.GetByID(ctx, id); err != nil {
			return domain.Trip{}, err
		}
		return domain.Trip{}, domain.ErrTripCompleted
	}

	return r.GetByID(ctx, id)
}

func scanTrip(row pgx.Row) (domain.Trip, error) {
	var t domain.Trip
	err := row.Scan(
		&t.ID, &t.UserID, &t.DriverID,
		&t.StartLatitude, &t.StartLongitude, &t.EndLatitude, &t.EndLongitude,
		&t.Price, &t.Status, &t.StartedAt, &t.FinishedAt,
	)
	return t, err
}
