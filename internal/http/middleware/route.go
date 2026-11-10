// Package middleware: обёртки над роутером. Порядок фиксирован:
// Recover → Tracing → Logging → Metrics → роутер. Подключаются через
// chi Router.Use, поэтому видят шаблон маршрута, а не путь с tripId
package middleware

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
)

// routePattern: шаблон сработавшего маршрута (/api/v1/trips/{tripId}).
// Известен после того, как роутер обработал запрос. Пусто, если маршрут
// не нашёлся
func routePattern(r *http.Request) string {
	if rctx := chi.RouteContext(r.Context()); rctx != nil {
		return rctx.RoutePattern()
	}
	return ""
}

// findRoutePattern ищет шаблон до обработки запроса: нужен там, где
// значение пишется в начале запроса (http.server.active_requests)
func findRoutePattern(r *http.Request) string {
	rctx := chi.RouteContext(r.Context())
	if rctx == nil || rctx.Routes == nil {
		return ""
	}
	return rctx.Routes.Find(chi.NewRouteContext(), r.Method, r.URL.Path)
}

// wrap запоминает статус ответа. Если хендлер не вызвал WriteHeader,
// статус 200
func wrap(w http.ResponseWriter, r *http.Request) chimw.WrapResponseWriter {
	return chimw.NewWrapResponseWriter(w, r.ProtoMajor)
}

func statusOf(ww chimw.WrapResponseWriter) int {
	if s := ww.Status(); s != 0 {
		return s
	}
	return http.StatusOK
}
