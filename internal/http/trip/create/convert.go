package create

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/ShtyroffI/tripgo-lab/api"
	"github.com/ShtyroffI/tripgo-lab/internal/domain/trip"
	"github.com/ShtyroffI/tripgo-lab/internal/http/domainerror"
	"github.com/ShtyroffI/tripgo-lab/internal/model"
)

// maxBodyBytes ограничивает размер тела запроса
const maxBodyBytes = 1 << 20

// decodeRequest читает тело и переводит его во вход usecase. Здесь только
// разбор JSON; правила значений (диапазоны, цена) проверяет домен
func decodeRequest(w http.ResponseWriter, r *http.Request) (model.CreateTripInput, error) {
	raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxBodyBytes))
	if err != nil {
		return model.CreateTripInput{}, &domainerror.BadRequestError{Detail: "Request body is too large or unreadable"}
	}
	if err := checkRequiredFields(raw); err != nil {
		return model.CreateTripInput{}, err
	}

	var body api.CreateTripJSONRequestBody
	dec := json.NewDecoder(bytes.NewReader(raw))
	// Неизвестные поля запрещены: клиент не может передать свой id
	dec.DisallowUnknownFields()
	if err := dec.Decode(&body); err != nil {
		return model.CreateTripInput{}, &domainerror.BadRequestError{Detail: "Request body is not valid JSON: " + err.Error()}
	}

	return toInput(body), nil
}

func toInput(body api.TripData) model.CreateTripInput {
	return model.CreateTripInput{
		UserID:   body.UserId,
		DriverID: body.DriverId,
		Start:    trip.Point{Latitude: body.StartPoint.Latitude, Longitude: body.StartPoint.Longitude},
		End:      trip.Point{Latitude: body.EndPoint.Latitude, Longitude: body.EndPoint.Longitude},
		Price:    body.Price,
	}
}

// checkRequiredFields нужен, чтобы пропущенное поле или null не стали нулём
// после декодирования. Настоящий 0 в price и координатах допустим
func checkRequiredFields(raw []byte) error {
	var body map[string]json.RawMessage
	if err := json.Unmarshal(raw, &body); err != nil {
		return &domainerror.BadRequestError{Detail: "Request body must be a JSON object"}
	}

	for _, name := range []string{"user_id", "driver_id", "start_point", "end_point", "price"} {
		if !hasValue(body[name]) {
			return &trip.ValidationError{Field: name, Reason: "is required"}
		}
	}

	for _, name := range []string{"start_point", "end_point"} {
		var point map[string]json.RawMessage
		if err := json.Unmarshal(body[name], &point); err != nil {
			return &trip.ValidationError{Field: name, Reason: "must be an object"}
		}
		for _, field := range []string{"latitude", "longitude"} {
			if !hasValue(point[field]) {
				return &trip.ValidationError{Field: fmt.Sprintf("%s.%s", name, field), Reason: "is required"}
			}
		}
	}
	return nil
}

// hasValue: ключа нет (raw пустой) или значение null, значит поля нет
func hasValue(raw json.RawMessage) bool {
	return len(raw) > 0 && !bytes.Equal(bytes.TrimSpace(raw), []byte("null"))
}
