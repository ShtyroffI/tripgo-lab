package middleware

import (
	"fmt"
	"net/http"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	semconv "go.opentelemetry.io/otel/semconv/v1.43.0"
)

const meterName = "github.com/ShtyroffI/tripgo-lab/internal/http"

// Границы корзин гистограммы в секундах, рекомендованные семантическими
// конвенциями OpenTelemetry для http.server.request.duration
var durationBuckets = []float64{0.005, 0.01, 0.025, 0.05, 0.075, 0.1, 0.25, 0.5, 0.75, 1, 2.5, 5, 7.5, 10}

// Metrics пишет http.server.request.duration и http.server.active_requests.
// В http.route только шаблон маршрута: с tripId на каждую поездку
// появлялась бы своя временная серия, и метрика бы не агрегировалась
func Metrics(mp metric.MeterProvider) (func(http.Handler) http.Handler, error) {
	meter := mp.Meter(meterName)

	duration, err := meter.Float64Histogram("http.server.request.duration",
		metric.WithDescription("Длительность обработки HTTP-запроса"),
		metric.WithUnit("s"),
		metric.WithExplicitBucketBoundaries(durationBuckets...))
	if err != nil {
		return nil, fmt.Errorf("create http.server.request.duration: %w", err)
	}

	active, err := meter.Int64UpDownCounter("http.server.active_requests",
		metric.WithDescription("Запросы, которые обрабатываются прямо сейчас"),
		metric.WithUnit("{request}"))
	if err != nil {
		return nil, fmt.Errorf("create http.server.active_requests: %w", err)
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ww := wrap(w, r)
			start := time.Now()

			activeAttrs := metric.WithAttributes(requestAttrs(r.Method, findRoutePattern(r))...)
			active.Add(r.Context(), 1, activeAttrs)

			record := func(status int) {
				active.Add(r.Context(), -1, activeAttrs)
				attrs := append(requestAttrs(r.Method, routePattern(r)), semconv.HTTPResponseStatusCode(status))
				duration.Record(r.Context(), time.Since(start).Seconds(), metric.WithAttributes(attrs...))
			}

			defer func() {
				if p := recover(); p != nil {
					record(http.StatusInternalServerError)
					panic(p)
				}
				record(statusOf(ww))
			}()

			next.ServeHTTP(ww, r)
		})
	}, nil
}

// requestAttrs: метод и шаблон маршрута. Маршрут не нашёлся: атрибут
// http.route не ставим, как велят семантические конвенции
func requestAttrs(method, route string) []attribute.KeyValue {
	attrs := []attribute.KeyValue{semconv.HTTPRequestMethodKey.String(method)}
	if route != "" {
		attrs = append(attrs, semconv.HTTPRoute(route))
	}
	return attrs
}
