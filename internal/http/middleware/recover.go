package middleware

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"runtime/debug"

	"github.com/ShtyroffI/tripgo-lab/internal/http/domainerror"
	"github.com/ShtyroffI/tripgo-lab/internal/http/response"
)

// errPanic: паника в обработчике. В ответ уходит 500 internal_error без
// деталей, сама паника и стектрейс только в лог
var errPanic = errors.New("panic in handler")

// Recover ловит панику, логирует её со стектрейсом и отвечает 500.
// Процесс не падает. Стоит первым, чтобы поймать панику и в middleware
func Recover(log *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				p := recover()
				if p == nil {
					return
				}
				// ErrAbortHandler: штатный способ оборвать ответ, его
				// пробрасываем дальше, net/http обработает сам
				if p == http.ErrAbortHandler {
					panic(p)
				}

				log.ErrorContext(r.Context(), "panic recovered",
					"panic", fmt.Sprint(p),
					"stack", string(debug.Stack()),
					"method", r.Method,
					"path", r.URL.Path,
				)

				status, body := domainerror.Problem(errPanic)
				instance := r.URL.Path
				body.Instance = &instance
				response.ProblemJSON(w, status, body)
			}()

			next.ServeHTTP(w, r)
		})
	}
}
