package telemetry

import (
	"context"
	"fmt"

	"go.opentelemetry.io/otel/metric"
)

// meterName: имя инструментирующего пакета для бизнес-метрик
const meterName = "github.com/ShtyroffI/tripgo-lab"

// TripMetrics: бизнес-метрики поездок. Ошибки (trip_not_found, driver_busy)
// отдельно не считаем, они видны в http.server.request.duration по
// http.response.status_code
type TripMetrics struct {
	created   metric.Int64Counter
	completed metric.Int64Counter
}

// activeTripsCounter считает поездки в статусе active. Реализует репозиторий
type activeTripsCounter interface {
	CountActive(ctx context.Context) (int64, error)
}

// NewTripMetrics регистрирует счётчики tripgo.trips.created и
// tripgo.trips.completed и gauge tripgo.trips.active. Gauge снимается в
// момент сбора метрик запросом count(*) в бд
func NewTripMetrics(mp metric.MeterProvider, active activeTripsCounter) (*TripMetrics, error) {
	meter := mp.Meter(meterName)

	created, err := meter.Int64Counter("tripgo.trips.created",
		metric.WithDescription("Созданные поездки"),
		metric.WithUnit("{trip}"))
	if err != nil {
		return nil, fmt.Errorf("create tripgo.trips.created: %w", err)
	}

	completed, err := meter.Int64Counter("tripgo.trips.completed",
		metric.WithDescription("Завершённые поездки"),
		metric.WithUnit("{trip}"))
	if err != nil {
		return nil, fmt.Errorf("create tripgo.trips.completed: %w", err)
	}

	_, err = meter.Int64ObservableGauge("tripgo.trips.active",
		metric.WithDescription("Поездки в статусе active на момент сбора"),
		metric.WithUnit("{trip}"),
		metric.WithInt64Callback(func(ctx context.Context, o metric.Int64Observer) error {
			n, err := active.CountActive(ctx)
			if err != nil {
				return fmt.Errorf("count active trips: %w", err)
			}
			o.Observe(n)
			return nil
		}))
	if err != nil {
		return nil, fmt.Errorf("create tripgo.trips.active: %w", err)
	}

	return &TripMetrics{created: created, completed: completed}, nil
}

func (m *TripMetrics) TripCreated(ctx context.Context) {
	m.created.Add(ctx, 1)
}

func (m *TripMetrics) TripCompleted(ctx context.Context) {
	m.completed.Add(ctx, 1)
}
