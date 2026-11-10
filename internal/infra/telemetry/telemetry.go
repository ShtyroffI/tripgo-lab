// Package telemetry: инициализация OpenTelemetry (трейсы и метрики по OTLP)
// и логгера slog с trace_id и span_id. Провайдеры не регистрируются
// глобально: container передаёт их тем, кому они нужны
package telemetry

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/propagation"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.43.0"
	"go.opentelemetry.io/otel/trace"
)

// Протоколы OTLP из OTEL_EXPORTER_OTLP_PROTOCOL
const (
	ProtocolHTTP = "http/protobuf"
	ProtocolGRPC = "grpc"
)

type Config struct {
	// Protocol: http/protobuf или grpc. Адрес коллектора
	// (OTEL_EXPORTER_OTLP_ENDPOINT) экспортёры читают из окружения сами
	Protocol string
	// ServiceVersion: версия сборки для атрибута service.version
	ServiceVersion string
}

type Telemetry struct {
	TracerProvider trace.TracerProvider
	MeterProvider  metric.MeterProvider
	Propagator     propagation.TextMapPropagator

	shutdown []func(context.Context) error
}

// Setup создаёт провайдеры трейсов и метрик. К коллектору при старте не
// подключается: экспорт асинхронный, поэтому без коллектора сервис
// стартует и работает, а ошибки экспорта уходят в лог как Warn
func Setup(ctx context.Context, cfg Config, log *slog.Logger) (*Telemetry, error) {
	// Ресурс: service.name и атрибуты из OTEL_SERVICE_NAME и
	// OTEL_RESOURCE_ATTRIBUTES (service.namespace, deployment.environment),
	// версию сборки добавляем сами
	res, err := resource.New(ctx,
		resource.WithFromEnv(),
		resource.WithTelemetrySDK(),
		resource.WithAttributes(semconv.ServiceVersion(cfg.ServiceVersion)),
	)
	if err != nil {
		return nil, fmt.Errorf("create otel resource: %w", err)
	}

	spanExporter, metricExporter, err := newExporters(ctx, cfg.Protocol)
	if err != nil {
		return nil, err
	}

	// Сэмплер не задаём: SDK берёт его из OTEL_TRACES_SAMPLER
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithResource(res),
		sdktrace.WithBatcher(spanExporter),
	)
	mp := sdkmetric.NewMeterProvider(
		sdkmetric.WithResource(res),
		sdkmetric.WithReader(sdkmetric.NewPeriodicReader(metricExporter)),
	)

	// Обработчик ошибок у otel только глобальный. Без него недоступность
	// коллектора печаталась бы в stderr не JSON. Это деградация, с которой
	// сервис справляется сам, поэтому Warn
	otel.SetErrorHandler(otel.ErrorHandlerFunc(func(err error) {
		log.Warn("telemetry export failed", "error", err)
	}))

	return &Telemetry{
		TracerProvider: tp,
		MeterProvider:  mp,
		// Контекст трейса между сервисами: W3C TraceContext
		Propagator: propagation.TraceContext{},
		shutdown:   []func(context.Context) error{tp.Shutdown, mp.Shutdown},
	}, nil
}

func newExporters(ctx context.Context, protocol string) (sdktrace.SpanExporter, sdkmetric.Exporter, error) {
	var (
		spanExporter   sdktrace.SpanExporter
		metricExporter sdkmetric.Exporter
		err            error
	)

	switch protocol {
	case ProtocolHTTP:
		if spanExporter, err = otlptracehttp.New(ctx); err != nil {
			return nil, nil, fmt.Errorf("create otlp http span exporter: %w", err)
		}
		if metricExporter, err = otlpmetrichttp.New(ctx); err != nil {
			return nil, nil, fmt.Errorf("create otlp http metric exporter: %w", err)
		}
	case ProtocolGRPC:
		if spanExporter, err = otlptracegrpc.New(ctx); err != nil {
			return nil, nil, fmt.Errorf("create otlp grpc span exporter: %w", err)
		}
		if metricExporter, err = otlpmetricgrpc.New(ctx); err != nil {
			return nil, nil, fmt.Errorf("create otlp grpc metric exporter: %w", err)
		}
	default:
		return nil, nil, fmt.Errorf("unsupported otlp protocol %q", protocol)
	}

	return spanExporter, metricExporter, nil
}

// Shutdown отправляет то, что накопилось, и останавливает провайдеры.
// Вызывать до закрытия пула бд: последний сбор метрик ходит в бд
func (t *Telemetry) Shutdown(ctx context.Context) error {
	errs := make([]error, 0, len(t.shutdown))
	for _, fn := range t.shutdown {
		if err := fn(ctx); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}
