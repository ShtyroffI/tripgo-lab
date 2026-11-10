package server

import (
	"net/http"

	"github.com/ShtyroffI/tripgo-lab/api"
)

// Каждая ручка живёт в своём пакете. Сервер знает о них только эти методы
// и собирает из них api.ServerInterface
type (
	createHandler interface {
		CreateTrip(w http.ResponseWriter, r *http.Request, params api.CreateTripParams)
	}

	getHandler interface {
		GetTrip(w http.ResponseWriter, r *http.Request, tripID api.TripId)
	}

	finishHandler interface {
		FinishTrip(w http.ResponseWriter, r *http.Request, tripID api.TripId)
	}

	serviceHandler interface {
		Health(w http.ResponseWriter, r *http.Request)
		Ready(w http.ResponseWriter, r *http.Request)
	}
)
