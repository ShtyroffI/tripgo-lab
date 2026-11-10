// Package model: структуры, которые ходят между слоями. Транспорт собирает
// их из запроса, usecase превращает в доменные сущности. Так usecase не
// зависит от сгенерированных типов API, а транспорт не собирает доменную
// модель сам
package model

import (
	"github.com/google/uuid"

	"github.com/ShtyroffI/tripgo-lab/internal/domain/trip"
)

// CreateTripInput: что нужно usecase для создания поездки
type CreateTripInput struct {
	UserID   uuid.UUID
	DriverID uuid.UUID
	Start    trip.Point
	End      trip.Point
	Price    int64
}

// Params переводит вход в параметры доменной модели
func (in CreateTripInput) Params() trip.Params {
	return trip.Params{
		UserID:   in.UserID,
		DriverID: in.DriverID,
		Start:    in.Start,
		End:      in.End,
		Price:    in.Price,
	}
}
