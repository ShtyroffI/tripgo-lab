package middleware

import (
	"net/http"
	"strconv"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/metric/noop"
	"go.opentelemetry.io/otel/propagation"
	semconv "go.opentelemetry.io/otel/semconv/v1.43.0"
	"go.opentelemetry.io/otel/trace"
)

// Tracing создаёт корневой спан на входящий запрос (otelhttp) и достаёт
// контекст трейса из заголовков W3C TraceContext. Имя спана
// "{method} {http.route}" ставим после роутинга, когда шаблон известен
func Tracing(tp trace.TracerProvider, propagator propagation.TextMapPropagator) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		named := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ww := wrap(w, r)
			span := trace.SpanFromContext(r.Context())

			status := http.StatusInternalServerError
			defer func() {
				// После паники тоже: спан помечается ошибкой, паника идёт
				// дальше в Recover
				if p := recover(); p != nil {
					finishSpan(span, r, status)
					panic(p)
				}
				finishSpan(span, r, statusOf(ww))
			}()

			next.ServeHTTP(ww, r)
		})

		return otelhttp.NewHandler(named, "",
			otelhttp.WithTracerProvider(tp),
			otelhttp.WithPropagators(propagator),
			// HTTP-метрики пишет Metrics с шаблоном маршрута, у otelhttp
			// их выключаем, чтобы не было двух гистограмм с одним именем
			otelhttp.WithMeterProvider(noop.NewMeterProvider()),
			// otelhttp зовёт форматтер дважды: при старте спана (шаблона
			// ещё нет, имя: только метод) и после ответа, если роутер
			// проставил r.Pattern. Во второй раз берём шаблон из chi
			otelhttp.WithSpanNameFormatter(spanName),
		)
	}
}

// spanName: "{method} {http.route}", например "POST /api/v1/trips", а не
// полный URL с tripId
func spanName(_ string, r *http.Request) string {
	if route := routePattern(r); route != "" {
		return r.Method + " " + route
	}
	return r.Method
}

func finishSpan(span trace.Span, r *http.Request, status int) {
	if route := routePattern(r); route != "" {
		span.SetName(spanName("", r))
		span.SetAttributes(semconv.HTTPRoute(route))
	}

	// 4xx: ошибка клиента, спан не красим. 5xx: ошибка сервиса
	if status >= http.StatusInternalServerError {
		span.SetStatus(codes.Error, http.StatusText(status))
		span.SetAttributes(semconv.ErrorTypeKey.String(strconv.Itoa(status)))
	}
}
