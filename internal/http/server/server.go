// Package server: роутер chi и http.Server с таймаутами
package server

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/ShtyroffI/tripgo-lab/api"
	"github.com/ShtyroffI/tripgo-lab/internal/http/domainerror"
)

// Без таймаутов зависшее соединение живёт бесконечно. Отдельных
// переменных окружения для них в курсе нет
const (
	readTimeout       = 10 * time.Second
	readHeaderTimeout = 5 * time.Second
	writeTimeout      = 10 * time.Second
	idleTimeout       = 60 * time.Second
)

// Handlers: ручки, из которых собирается API
type Handlers struct {
	Create  createHandler
	Get     getHandler
	Finish  finishHandler
	Service serviceHandler
}

// routes собирает ручки из разных пакетов в один api.ServerInterface
type routes struct {
	Handlers
}

var _ api.ServerInterface = routes{}

func (rt routes) CreateTrip(w http.ResponseWriter, r *http.Request, params api.CreateTripParams) {
	rt.Create.CreateTrip(w, r, params)
}

func (rt routes) GetTrip(w http.ResponseWriter, r *http.Request, tripID api.TripId) {
	rt.Get.GetTrip(w, r, tripID)
}

func (rt routes) FinishTrip(w http.ResponseWriter, r *http.Request, tripID api.TripId) {
	rt.Finish.FinishTrip(w, r, tripID)
}

func (rt routes) Health(w http.ResponseWriter, r *http.Request) {
	rt.Service.Health(w, r)
}

func (rt routes) Ready(w http.ResponseWriter, r *http.Request) {
	rt.Service.Ready(w, r)
}

// New собирает роутер. middlewares подключаются через Use до регистрации
// маршрутов в переданном порядке (Recover → Tracing → Logging → Metrics):
// так они работают внутри chi и видят шаблон маршрута
func New(addr string, h Handlers, log *slog.Logger, middlewares ...func(http.Handler) http.Handler) *http.Server {
	router := chi.NewRouter()
	router.Use(middlewares...)

	api.HandlerWithOptions(routes{Handlers: h}, api.ChiServerOptions{
		BaseRouter: router,
		// Ошибки разбора параметров (tripId не UUID) отдаём тем же маппером
		ErrorHandlerFunc: func(w http.ResponseWriter, r *http.Request, err error) {
			domainerror.Write(w, r, log, &domainerror.BadRequestError{Detail: err.Error()})
		},
	})

	return &http.Server{
		Addr:              addr,
		Handler:           router,
		ReadTimeout:       readTimeout,
		ReadHeaderTimeout: readHeaderTimeout,
		WriteTimeout:      writeTimeout,
		IdleTimeout:       idleTimeout,
	}
}
