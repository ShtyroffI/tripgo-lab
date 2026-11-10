// Package domainerror: единственное место, где ошибка превращается в
// HTTP-ответ. Хендлеры ошибку не разбирают, а отдают сюда
package domainerror

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/ShtyroffI/tripgo-lab/api"
	"github.com/ShtyroffI/tripgo-lab/internal/domain/trip"
	"github.com/ShtyroffI/tripgo-lab/internal/http/response"
)

// BadRequestError: запрос не разобрался на уровне транспорта (не JSON,
// неверный параметр пути). До домена такой запрос не доходит
type BadRequestError struct {
	Detail string
}

func (e *BadRequestError) Error() string {
	return e.Detail
}

const problemTypeBase = "https://tripgo.example/problems/"

// Problem переводит ошибку в статус и тело по RFC 9457. Всё, что не
// известная доменная или транспортная ошибка, становится 500 без деталей:
// текст ошибки бд, SQL и стектрейс наружу не попадают
func Problem(err error) (status int, body api.Problem) {
	var (
		validationErr *trip.ValidationError
		badRequestErr *BadRequestError
	)

	switch {
	case errors.As(err, &validationErr):
		body = newProblem(http.StatusBadRequest, "invalid-request", "Invalid request", "invalid_request",
			validationErr.Error())
	case errors.As(err, &badRequestErr):
		body = newProblem(http.StatusBadRequest, "invalid-request", "Invalid request", "invalid_request",
			badRequestErr.Detail)
	case errors.Is(err, trip.ErrTripNotFound):
		body = newProblem(http.StatusNotFound, "trip-not-found", "Trip not found", "trip_not_found",
			"Trip was not found")
	case errors.Is(err, trip.ErrTripCompleted):
		body = newProblem(http.StatusConflict, "trip-completed", "Trip completed", "trip_completed",
			"Operation is not allowed for a completed trip")
	case errors.Is(err, trip.ErrDriverBusy):
		body = newProblem(http.StatusConflict, "driver-busy", "Driver busy", "driver_busy",
			"Driver already has an active trip")
	default:
		body = newProblem(http.StatusInternalServerError, "internal-error", "Internal Server Error", "internal_error",
			"Internal server error")
	}

	return int(body.Status), body
}

// Write отвечает клиенту ошибкой. Внутренние ошибки (5xx) логирует здесь,
// один раз: выше по стеку их только оборачивают
func Write(w http.ResponseWriter, r *http.Request, log *slog.Logger, err error) {
	status, body := Problem(err)
	if status >= http.StatusInternalServerError {
		log.ErrorContext(r.Context(), "request failed", "error", err)
	}

	instance := r.URL.Path
	body.Instance = &instance
	response.ProblemJSON(w, status, body)
}

func newProblem(status int32, slug, title, code, detail string) api.Problem {
	return api.Problem{
		Type:   problemTypeBase + slug,
		Title:  title,
		Status: status,
		Code:   code,
		Detail: &detail,
	}
}
