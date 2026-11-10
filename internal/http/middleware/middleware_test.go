package middleware_test

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"

	"github.com/ShtyroffI/tripgo-lab/internal/http/middleware"
)

const tripRoute = "/api/v1/trips/{tripId}"

type testEnv struct {
	router  chi.Router
	spans   *tracetest.SpanRecorder
	metrics *sdkmetric.ManualReader
	logs    *bytes.Buffer
}

// newRouter собирает chi с middleware в боевом порядке и одной ручкой,
// которая отвечает заданным статусом или паникует
func newRouter(t *testing.T, status int, panics bool) testEnv {
	t.Helper()

	spans := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(spans))
	reader := sdkmetric.NewManualReader()
	mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	logs := &bytes.Buffer{}
	log := slog.New(slog.NewJSONHandler(logs, nil))

	metricsMW, err := middleware.Metrics(mp)
	require.NoError(t, err)

	r := chi.NewRouter()
	r.Use(
		middleware.Recover(log),
		middleware.Tracing(tp, propagation.TraceContext{}),
		middleware.Logging(log),
		metricsMW,
	)
	r.Get(tripRoute, func(w http.ResponseWriter, _ *http.Request) {
		if panics {
			panic("boom")
		}
		w.WriteHeader(status)
	})

	return testEnv{router: r, spans: spans, metrics: reader, logs: logs}
}

func serve(env testEnv, path string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	env.router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, http.NoBody))
	return rec
}

func logRecords(t *testing.T, buf *bytes.Buffer) []map[string]any {
	t.Helper()
	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	records := make([]map[string]any, 0, len(lines))
	for _, line := range lines {
		var rec map[string]any
		require.NoError(t, json.Unmarshal([]byte(line), &rec))
		records = append(records, rec)
	}
	return records
}

func TestSpanNameAndRouteUseTemplate(t *testing.T) {
	env := newRouter(t, http.StatusOK, false)

	serve(env, "/api/v1/trips/5cb72c04-7650-45c9-a79b-bcdba0631e0c")

	ended := env.spans.Ended()
	require.Len(t, ended, 1)
	// Имя спана и http.route: шаблон, а не путь с конкретным tripId
	assert.Equal(t, "GET "+tripRoute, ended[0].Name())
	assert.Contains(t, ended[0].Attributes(), attribute.String("http.route", tripRoute))
	assert.Equal(t, codes.Unset, ended[0].Status().Code)
}

func TestServerErrorMarksSpan(t *testing.T) {
	env := newRouter(t, http.StatusInternalServerError, false)

	serve(env, "/api/v1/trips/1")

	ended := env.spans.Ended()
	require.Len(t, ended, 1)
	assert.Equal(t, codes.Error, ended[0].Status().Code)
	assert.Contains(t, ended[0].Attributes(), attribute.String("error.type", "500"))
}

func TestClientErrorDoesNotMarkSpan(t *testing.T) {
	env := newRouter(t, http.StatusNotFound, false)

	serve(env, "/api/v1/trips/1")

	ended := env.spans.Ended()
	require.Len(t, ended, 1)
	assert.NotEqual(t, codes.Error, ended[0].Status().Code)
}

func TestLoggingOneRecordPerRequest(t *testing.T) {
	tests := []struct {
		name      string
		status    int
		wantLevel string
	}{
		{name: "2xx is info", status: http.StatusCreated, wantLevel: "INFO"},
		{name: "4xx is info", status: http.StatusConflict, wantLevel: "INFO"},
		{name: "5xx is error", status: http.StatusServiceUnavailable, wantLevel: "ERROR"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := newRouter(t, tt.status, false)

			serve(env, "/api/v1/trips/42")

			records := logRecords(t, env.logs)
			require.Len(t, records, 1)
			rec := records[0]
			assert.Equal(t, "request handled", rec["msg"])
			assert.Equal(t, tt.wantLevel, rec["level"])
			assert.Equal(t, tripRoute, rec["route"])
			assert.InDelta(t, tt.status, rec["status"], 0)
			assert.Contains(t, rec, "duration_ms")
		})
	}
}

func TestRecoverReturns500AndLogsPanic(t *testing.T) {
	env := newRouter(t, 0, true)

	rec := serve(env, "/api/v1/trips/1")

	assert.Equal(t, http.StatusInternalServerError, rec.Code)
	assert.Equal(t, "application/problem+json", rec.Header().Get("Content-Type"))
	assert.Contains(t, rec.Body.String(), `"code":"internal_error"`)
	// Значение паники и стектрейс только в логе, не в ответе
	assert.NotContains(t, rec.Body.String(), "boom")

	var panicLogged, requestLogged bool
	for _, r := range logRecords(t, env.logs) {
		switch r["msg"] {
		case "panic recovered":
			panicLogged = true
			assert.Equal(t, "boom", r["panic"])
			assert.NotEmpty(t, r["stack"])
		case "request handled":
			requestLogged = true
			assert.InDelta(t, http.StatusInternalServerError, r["status"], 0)
		}
	}
	assert.True(t, panicLogged)
	assert.True(t, requestLogged)

	ended := env.spans.Ended()
	require.Len(t, ended, 1)
	assert.Equal(t, codes.Error, ended[0].Status().Code)
}

func TestMetricsUseRouteTemplate(t *testing.T) {
	env := newRouter(t, http.StatusOK, false)

	serve(env, "/api/v1/trips/1")
	serve(env, "/api/v1/trips/2")

	var rm metricdata.ResourceMetrics
	require.NoError(t, env.metrics.Collect(context.Background(), &rm))

	var duration metricdata.Histogram[float64]
	var activeFound bool
	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			switch m.Name {
			case "http.server.request.duration":
				var ok bool
				duration, ok = m.Data.(metricdata.Histogram[float64])
				require.True(t, ok)
			case "http.server.active_requests":
				activeFound = true
			}
		}
	}

	assert.True(t, activeFound)
	// Два разных tripId попали в одну серию: в http.route шаблон
	require.Len(t, duration.DataPoints, 1)
	dp := duration.DataPoints[0]
	assert.Equal(t, uint64(2), dp.Count)
	route, ok := dp.Attributes.Value("http.route")
	require.True(t, ok)
	assert.Equal(t, tripRoute, route.AsString())
	statusCode, ok := dp.Attributes.Value("http.response.status_code")
	require.True(t, ok)
	assert.Equal(t, int64(http.StatusOK), statusCode.AsInt64())
}
