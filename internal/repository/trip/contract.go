package trip

import (
	"context"

	"github.com/ShtyroffI/tripgo-lab/internal/infra/postgres/transaction"
)

// executorProvider отдаёт, через что выполнять запрос: транзакцию из ctx
// или пул. Реализует transaction.Manager
type executorProvider interface {
	Executor(ctx context.Context) transaction.Executor
}
