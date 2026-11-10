package middleware

import (
	"log/slog"
	"net/http"
	"time"
)

// Logging пишет одну запись на запрос, когда ответ уже готов. Тела запроса
// и ответа не логируем: там персональные данные. Info для 2xx и 4xx
// (ошибка клиента не поломка сервиса), Error только для 5xx
func Logging(log *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ww := wrap(w, r)
			start := time.Now()

			defer func() {
				if p := recover(); p != nil {
					logRequest(log, r, http.StatusInternalServerError, start)
					panic(p)
				}
				logRequest(log, r, statusOf(ww), start)
			}()

			next.ServeHTTP(ww, r)
		})
	}
}

func logRequest(log *slog.Logger, r *http.Request, status int, start time.Time) {
	level := slog.LevelInfo
	if status >= http.StatusInternalServerError {
		level = slog.LevelError
	}

	// Контекст запроса несёт спан из Tracing: trace_id и span_id добавит
	// хендлер логгера
	log.Log(r.Context(), level, "request handled",
		"method", r.Method,
		"route", routePattern(r),
		"status", status,
		"duration_ms", time.Since(start).Milliseconds(),
	)
}
