//go:build integration

package trip_test

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/ShtyroffI/tripgo-lab/internal/infra/postgres"
	"github.com/ShtyroffI/tripgo-lab/internal/infra/postgres/transaction"
	triprepo "github.com/ShtyroffI/tripgo-lab/internal/repository/trip"
)

const (
	queryTimeout  = 5 * time.Second
	migrationsDir = "../../../migrations"
)

// Общие для всех тестов пакета: один контейнер и один пул
var (
	testPool *pgxpool.Pool
	testTx   *transaction.Manager
	testRepo *triprepo.Repository
)

func TestMain(m *testing.M) {
	os.Exit(run(m))
}

// run отдельно от TestMain, чтобы defer отработали до os.Exit
func run(m *testing.M) int {
	ctx := context.Background()

	container, err := tcpostgres.Run(ctx, "postgres:16-alpine",
		tcpostgres.WithDatabase("tripgo_test"),
		tcpostgres.WithUsername("test"),
		tcpostgres.WithPassword("test"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(60*time.Second),
		),
	)
	if err != nil {
		fmt.Fprintln(os.Stderr, "start postgres container:", err)
		return 1
	}
	defer func() {
		if err := testcontainers.TerminateContainer(container); err != nil {
			fmt.Fprintln(os.Stderr, "terminate postgres container:", err)
		}
	}()

	dsn, err := container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		fmt.Fprintln(os.Stderr, "connection string:", err)
		return 1
	}

	testPool, err = postgres.New(ctx, postgres.Config{
		URL:             dsn,
		MaxConns:        10,
		MinConns:        1,
		MaxConnLifetime: time.Hour,
		ConnectTimeout:  10 * time.Second,
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, "create pool:", err)
		return 1
	}
	defer testPool.Close()

	if err := migrate(ctx, testPool); err != nil {
		fmt.Fprintln(os.Stderr, "migrate:", err)
		return 1
	}

	testTx = transaction.New(testPool)
	testRepo = triprepo.New(testTx, queryTimeout)

	return m.Run()
}

// migrate накатывает настоящие миграции из migrations/
func migrate(ctx context.Context, pool *pgxpool.Pool) error {
	db := stdlib.OpenDBFromPool(pool)
	defer db.Close()

	if err := goose.SetDialect("postgres"); err != nil {
		return err
	}
	return goose.UpContext(ctx, db, migrationsDir)
}
