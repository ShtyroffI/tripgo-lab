// Package container: единственное место, где создаются конкретные
// реализации и связываются между собой. Остальной код получает зависимости
// аргументами конструктора и знает только интерфейсы
package container

import (
	"context"
	"log/slog"
	"net/http"
	"os"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ShtyroffI/tripgo-lab/internal/config"
	"github.com/ShtyroffI/tripgo-lab/internal/http/server"
	"github.com/ShtyroffI/tripgo-lab/internal/http/service"
	httpcreate "github.com/ShtyroffI/tripgo-lab/internal/http/trip/create"
	httpfinish "github.com/ShtyroffI/tripgo-lab/internal/http/trip/finish"
	httpget "github.com/ShtyroffI/tripgo-lab/internal/http/trip/get"
	"github.com/ShtyroffI/tripgo-lab/internal/infra/postgres"
	"github.com/ShtyroffI/tripgo-lab/internal/infra/postgres/transaction"
	triprepo "github.com/ShtyroffI/tripgo-lab/internal/repository/trip"
	"github.com/ShtyroffI/tripgo-lab/internal/usecase/trip/create"
	"github.com/ShtyroffI/tripgo-lab/internal/usecase/trip/finish"
	"github.com/ShtyroffI/tripgo-lab/internal/usecase/trip/get"
)

type Container struct {
	Log    *slog.Logger
	Server *http.Server

	pool *pgxpool.Pool
}

// New собирает сервис. ctx нужен для подключения к бд при старте
func New(ctx context.Context, cfg config.Config) (*Container, error) {
	log := newLogger(cfg.LogLevel)

	pool, err := postgres.New(ctx, postgres.Config{
		URL:             cfg.DatabaseURL,
		MaxConns:        cfg.DatabaseMaxConns,
		MinConns:        cfg.DatabaseMinConns,
		MaxConnLifetime: cfg.DatabaseMaxConnLifetime,
		ConnectTimeout:  cfg.DatabaseConnectTimeout,
	})
	if err != nil {
		return nil, err
	}

	txManager := transaction.New(pool)
	trips := triprepo.New(txManager, cfg.DatabaseQueryTimeout)

	createUC := create.New(trips, txManager, log)
	getUC := get.New(trips)
	finishUC := finish.New(trips, txManager, log)

	srv := server.New(cfg.HTTPAddr, server.Handlers{
		Create:  httpcreate.New(createUC, log),
		Get:     httpget.New(getUC, log),
		Finish:  httpfinish.New(finishUC, log),
		Service: service.New(pool, cfg.DatabaseQueryTimeout),
	}, log)

	return &Container{Log: log, Server: srv, pool: pool}, nil
}

// Close освобождает то, что держит контейнер. Вызывать после остановки
// HTTP-сервера, когда текущие запросы уже обработаны
func (c *Container) Close() {
	c.pool.Close()
}

func newLogger(level string) *slog.Logger {
	var lvl slog.Level
	if err := lvl.UnmarshalText([]byte(level)); err != nil {
		lvl = slog.LevelInfo
	}
	return slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: lvl}))
}
