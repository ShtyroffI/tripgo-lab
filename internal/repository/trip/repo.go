// Package trip: хранение поездок в PostgreSQL через squirrel и pgx
package trip

import (
	"context"
	"time"

	sq "github.com/Masterminds/squirrel"

	"github.com/ShtyroffI/tripgo-lab/internal/infra/postgres/transaction"
)

const (
	tripsTable   = "trips"
	historyTable = "trip_status_history"

	// colStatus используется и в выборке, и в условиях WHERE
	colStatus = "status"
)

// pgUniqueViolation: код ошибки PostgreSQL 23505
const pgUniqueViolation = "23505"

var psql = sq.StatementBuilder.PlaceholderFormat(sq.Dollar)

type Repository struct {
	executors    executorProvider
	queryTimeout time.Duration
}

func New(executors executorProvider, queryTimeout time.Duration) *Repository {
	return &Repository{executors: executors, queryTimeout: queryTimeout}
}

// executor: транзакция из ctx, если её открыл usecase, иначе пул
func (r *Repository) executor(ctx context.Context) transaction.Executor {
	return r.executors.Executor(ctx)
}

// withTimeout: у каждого запроса в бд свой таймаут DATABASE_QUERY_TIMEOUT
func (r *Repository) withTimeout(ctx context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(ctx, r.queryTimeout)
}
