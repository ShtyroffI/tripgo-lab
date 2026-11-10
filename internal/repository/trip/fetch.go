package trip

import (
	"context"
	"errors"
	"fmt"

	sq "github.com/Masterminds/squirrel"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/ShtyroffI/tripgo-lab/internal/domain/trip"
)

// Get читает поездку. Нет строки: trip.ErrTripNotFound
func (r *Repository) Get(ctx context.Context, id uuid.UUID) (trip.Trip, error) {
	return r.get(ctx, id, false)
}

// GetForUpdate читает поездку и блокирует строку до конца транзакции
// (SELECT ... FOR UPDATE). Параллельный finish ждёт, пока первый
// закоммитится, и уже видит завершённую поездку. Вызывать внутри Do,
// иначе блокировка снимется сразу после запроса
func (r *Repository) GetForUpdate(ctx context.Context, id uuid.UUID) (trip.Trip, error) {
	return r.get(ctx, id, true)
}

func (r *Repository) get(ctx context.Context, id uuid.UUID, forUpdate bool) (trip.Trip, error) {
	builder := psql.Select(tripColumns...).
		From(tripsTable).
		Where(sq.Eq{"id": id})
	if forUpdate {
		builder = builder.Suffix("FOR UPDATE")
	}

	query, args, err := builder.ToSql()
	if err != nil {
		return trip.Trip{}, fmt.Errorf("build select trip query: %w", err)
	}

	ctx, cancel := r.withTimeout(ctx)
	defer cancel()

	t, err := scanTrip(r.executor(ctx).QueryRow(ctx, query, args...))
	if errors.Is(err, pgx.ErrNoRows) {
		return trip.Trip{}, trip.ErrTripNotFound
	}
	if err != nil {
		return trip.Trip{}, fmt.Errorf("select trip: %w", err)
	}
	return t, nil
}
