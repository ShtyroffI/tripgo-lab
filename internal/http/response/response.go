// Package response: запись JSON-ответов и перевод поездки в тип контракта.
// Общий код для всех ручек, чтобы не копировать его по пакетам
package response

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/ShtyroffI/tripgo-lab/api"
	"github.com/ShtyroffI/tripgo-lab/internal/domain/trip"
)

// JSON пишет тело с Content-Type application/json
func JSON(w http.ResponseWriter, status int, body any) {
	write(w, "application/json", status, body)
}

// ProblemJSON пишет ошибку по RFC 9457
func ProblemJSON(w http.ResponseWriter, status int, body api.Problem) {
	write(w, "application/problem+json", status, body)
}

func write(w http.ResponseWriter, contentType string, status int, body any) {
	w.Header().Set("Content-Type", contentType)
	w.WriteHeader(status)
	// Заголовок уже ушёл, ответить клиенту иначе не получится: ошибка
	// записи значит, что клиент отключился
	_ = json.NewEncoder(w).Encode(body)
}

// Trip переводит доменную поездку в тип контракта. Время всегда в UTC
func Trip(t trip.Trip) api.Trip {
	var finishedAt *time.Time
	if t.FinishedAt != nil {
		utc := t.FinishedAt.UTC()
		finishedAt = &utc
	}

	return api.Trip{
		Id:         t.ID,
		UserId:     t.UserID,
		DriverId:   t.DriverID,
		StartPoint: api.Coordinates{Latitude: t.Start.Latitude, Longitude: t.Start.Longitude},
		EndPoint:   api.Coordinates{Latitude: t.End.Latitude, Longitude: t.End.Longitude},
		Price:      t.Price,
		Status:     api.TripStatus(t.Status),
		StartedAt:  t.StartedAt.UTC(),
		FinishedAt: finishedAt,
	}
}
