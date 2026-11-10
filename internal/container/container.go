// Package container: единственное место, где создаются конкретные
// реализации и связываются между собой. Остальной код получает зависимости
// аргументами конструктора и знает только интерфейсы
package container

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ShtyroffI/tripgo-lab/internal/config"
	"github.com/ShtyroffI/tripgo-lab/internal/http/middleware"
	"github.com/ShtyroffI/tripgo-lab/internal/http/server"
	"github.com/ShtyroffI/tripgo-lab/internal/http/service"
	httpcreate "github.com/ShtyroffI/tripgo-lab/internal/http/trip/create"
	httpfinish "github.com/ShtyroffI/tripgo-lab/internal/http/trip/finish"
	httpget "github.com/ShtyroffI/tripgo-lab/internal/http/trip/get"
	"github.com/ShtyroffI/tripgo-lab/internal/infra/postgres"
	"github.com/ShtyroffI/tripgo-lab/internal/infra/postgres/transaction"
	"github.com/ShtyroffI/tripgo-lab/internal/infra/telemetry"
	triprepo "github.com/ShtyroffI/tripgo-lab/internal/repository/trip"
	"github.com/ShtyroffI/tripgo-lab/internal/usecase/trip/create"
	"github.com/ShtyroffI/tripgo-lab/internal/usecase/trip/finish"
	"github.com/ShtyroffI/tripgo-lab/internal/usecase/trip/get"
)

type Container struct {
	Log    *slog.Logger
	Server *http.Server

	telemetry *telemetry.Telemetry
	pool      *pgxpool.Pool
}

// New собирает сервис. ctx нужен для подключения к бд при старте
func New(ctx context.Context, cfg config.Config) (*Container, error) {
	log := telemetry.NewLogger(os.Stdout, cfg.LogLevel, cfg.OtelServiceName)

	tel, err := telemetry.Setup(ctx, telemetry.Config{
		Protocol:       cfg.OtelExporterProtocol,
		ServiceVersion: cfg.ServiceVersion,
	}, log)
	if err != nil {
		return nil, fmt.Errorf("setup telemetry: %w", err)
	}

	c, err := build(ctx, cfg, log, tel)
	if err != nil {
		// Телеметрия уже поднята: останавливаем, чтобы не оставить горутины
		return nil, errors.Join(err, tel.Shutdown(ctx))
	}
	return c, nil
}

func build(ctx context.Context, cfg config.Config, log *slog.Logger, tel *telemetry.Telemetry) (*Container, error) {
	pool, err := postgres.New(ctx, postgres.Config{
		URL:             cfg.DatabaseURL,
		MaxConns:        cfg.DatabaseMaxConns,
		MinConns:        cfg.DatabaseMinConns,
		MaxConnLifetime: cfg.DatabaseMaxConnLifetime,
		ConnectTimeout:  cfg.DatabaseConnectTimeout,
		TracerProvider:  tel.TracerProvider,
		MeterProvider:   tel.MeterProvider,
	})
	if err != nil {
		return nil, err
	}

	srv, err := buildServer(cfg, log, tel, pool)
	if err != nil {
		pool.Close()
		return nil, err
	}

	return &Container{Log: log, Server: srv, telemetry: tel, pool: pool}, nil
}

func buildServer(cfg config.Config, log *slog.Logger, tel *telemetry.Telemetry, pool *pgxpool.Pool) (*http.Server, error) {
	txManager := transaction.New(pool)
	trips := triprepo.New(txManager, cfg.DatabaseQueryTimeout)

	tripMetrics, err := telemetry.NewTripMetrics(tel.MeterProvider, trips)
	if err != nil {
		return nil, err
	}

	createUC := create.New(trips, txManager, tripMetrics, log)
	getUC := get.New(trips)
	finishUC := finish.New(trips, txManager, tripMetrics, log)

	httpMetrics, err := middleware.Metrics(tel.MeterProvider)
	if err != nil {
		return nil, err
	}

	return server.New(cfg.HTTPAddr, server.Handlers{
		Create:  httpcreate.New(createUC, log),
		Get:     httpget.New(getUC, log),
		Finish:  httpfinish.New(finishUC, log),
		Service: service.New(pool, cfg.DatabaseQueryTimeout),
	}, log,
		// Порядок фиксирован заданием: Recover → Tracing → Logging → Metrics
		middleware.Recover(log),
		middleware.Tracing(tel.TracerProvider, tel.Propagator),
		middleware.Logging(log),
		httpMetrics,
	), nil
}

// Close освобождает ресурсы после остановки HTTP-сервера: сначала
// дописывает телеметрию (последний сбор метрик ходит в бд), потом
// закрывает пул. Телеметрия не критичная зависимость: если коллектор
// недоступен, остаток данных теряется, это Warn, а не ошибка остановки
func (c *Container) Close(ctx context.Context) {
	if err := c.telemetry.Shutdown(ctx); err != nil {
		c.Log.Warn("telemetry shutdown failed, unsent data dropped", "error", err)
	}
	c.pool.Close()
}
