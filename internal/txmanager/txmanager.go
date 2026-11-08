// Транзакция передаётся через context, а не аргументом
package txmanager

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Executor interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// ctxKey приватный
type ctxKey struct{}

type Manager struct {
	pool *pgxpool.Pool
}

func New(pool *pgxpool.Pool) *Manager {
	return &Manager{pool: pool}
}

func (m *Manager) Do(ctx context.Context, fn func(ctx context.Context) error) error {
	if _, ok := ctx.Value(ctxKey{}).(pgx.Tx); ok {
		return fn(ctx)
	}

	tx, err := m.pool.BeginTx(ctx, pgx.TxOptions{
		IsoLevel: pgx.ReadCommitted,
	})
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}

	txCtx := context.WithValue(ctx, ctxKey{}, tx)

	defer func() {
		if p := recover(); p != nil {
			_ = tx.Rollback(context.WithoutCancel(ctx))
			panic(p)
		}
	}()

	if err := fn(txCtx); err != nil {
		if rbErr := tx.Rollback(context.WithoutCancel(ctx)); rbErr != nil {
			return fmt.Errorf("%w (rollback failed: %v)", err, rbErr)
		}
		return err
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit tx: %w", err)
	}
	return nil
}

// ExecutorFromContext возвращает транзакцию из ctx, если её нет - пул
func ExecutorFromContext(ctx context.Context, pool *pgxpool.Pool) Executor {
	if tx, ok := ctx.Value(ctxKey{}).(pgx.Tx); ok {
		return tx
	}
	return pool
}
