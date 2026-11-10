package postgres

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/exaring/otelpgx"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/metric/noop"
	semconv "go.opentelemetry.io/otel/semconv/v1.43.0"
	"go.opentelemetry.io/otel/trace"
)

const meterName = "github.com/ShtyroffI/tripgo-lab/internal/infra/postgres"

// Границы корзин для db.client.operation.duration (семантические конвенции)
var durationBuckets = []float64{0.001, 0.005, 0.01, 0.05, 0.1, 0.5, 1, 5, 10}

// queryTracer инструментирует каждый запрос на уровне пула, поэтому
// репозиторий про телеметрию не знает и забыть обернуть запрос нельзя.
// Спаны создаёт otelpgx. Поверх него добавляем то, чего otelpgx не
// пишет: db.collection.name и error.type в спане и метрику
// db.client.operation.duration с лейблами db.operation.name,
// db.collection.name и error.type
type queryTracer struct {
	spans    *otelpgx.Tracer
	duration metric.Float64Histogram
}

type queryStateKey struct{}

type queryState struct {
	start      time.Time
	operation  string
	collection string
}

func newQueryTracer(tp trace.TracerProvider, mp metric.MeterProvider) (*queryTracer, error) {
	duration, err := mp.Meter(meterName).Float64Histogram("db.client.operation.duration",
		metric.WithDescription("Длительность запроса в бд"),
		metric.WithUnit("s"),
		metric.WithExplicitBucketBoundaries(durationBuckets...))
	if err != nil {
		return nil, fmt.Errorf("create db.client.operation.duration: %w", err)
	}

	return &queryTracer{
		spans: otelpgx.NewTracer(
			otelpgx.WithTracerProvider(tp),
			// Метрику длительности пишем сами, со всеми нужными лейблами
			otelpgx.WithMeterProvider(noop.NewMeterProvider()),
		),
		duration: duration,
	}, nil
}

func (t *queryTracer) TraceQueryStart(ctx context.Context, conn *pgx.Conn, data pgx.TraceQueryStartData) context.Context {
	ctx = t.spans.TraceQueryStart(ctx, conn, data)

	operation, collection := parseQuery(data.SQL)
	if collection != "" {
		trace.SpanFromContext(ctx).SetAttributes(semconv.DBCollectionName(collection))
	}

	return context.WithValue(ctx, queryStateKey{}, queryState{
		start:      time.Now(),
		operation:  operation,
		collection: collection,
	})
}

func (t *queryTracer) TraceQueryEnd(ctx context.Context, conn *pgx.Conn, data pgx.TraceQueryEndData) {
	state, _ := ctx.Value(queryStateKey{}).(queryState)

	var attrs []attribute.KeyValue
	if state.operation != "" {
		attrs = append(attrs, semconv.DBOperationName(state.operation))
	}
	if state.collection != "" {
		attrs = append(attrs, semconv.DBCollectionName(state.collection))
	}
	if data.Err != nil {
		errType := errorType(data.Err)
		attrs = append(attrs, errType)

		span := trace.SpanFromContext(ctx)
		span.SetAttributes(errType)
		span.SetStatus(codes.Error, data.Err.Error())
	}

	// Спан закрывает otelpgx, поэтому атрибуты ставим до этого
	t.spans.TraceQueryEnd(ctx, conn, data)

	if !state.start.IsZero() {
		t.duration.Record(ctx, time.Since(state.start).Seconds(), metric.WithAttributes(attrs...))
	}
}

// errorType: для ошибок PostgreSQL код SQLSTATE (23505 и т.д.), иначе тип
// ошибки, как рекомендуют семантические конвенции
func errorType(err error) attribute.KeyValue {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return semconv.ErrorTypeKey.String(pgErr.Code)
	}
	return semconv.ErrorType(err)
}

// parseQuery достаёт из SQL операцию (SELECT, INSERT, ...) и таблицу.
// Запросы строит squirrel, у них простая форма, полный разбор SQL не нужен.
// Значения параметров сюда не попадают: в SQL только плейсхолдеры $1, $2
func parseQuery(sql string) (operation, collection string) {
	fields := strings.Fields(sql)
	if len(fields) == 0 || strings.HasPrefix(fields[0], "--") {
		return "", ""
	}

	operation = strings.ToUpper(fields[0])

	var after string
	switch operation {
	case "SELECT", "DELETE":
		after = "FROM"
	case "INSERT":
		after = "INTO"
	case "UPDATE":
		if len(fields) > 1 {
			return operation, cleanIdentifier(fields[1])
		}
		return operation, ""
	default:
		return operation, ""
	}

	for i := 1; i < len(fields)-1; i++ {
		if strings.EqualFold(fields[i], after) {
			return operation, cleanIdentifier(fields[i+1])
		}
	}
	return operation, ""
}

func cleanIdentifier(s string) string {
	if i := strings.IndexByte(s, '('); i >= 0 {
		s = s[:i]
	}
	return strings.Trim(s, `",;`)
}
