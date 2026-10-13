package httpserver

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/ShtyroffI/tripgo-lab/api"
)

// Без таймаутов зависшее соединение живёт бесконечно
const (
	readTimeout       = 10 * time.Second
	readHeaderTimeout = 5 * time.Second
	writeTimeout      = 10 * time.Second
	idleTimeout       = 60 * time.Second
)

func NewServer(addr string, h *Handler) *http.Server {
	router := chi.NewRouter()

	opts := api.ChiServerOptions{
		BaseRouter: router,
		ErrorHandlerFunc: func(w http.ResponseWriter, r *http.Request, err error) {
			writeInvalidRequest(w, r, err.Error())
		},
	}
	api.HandlerWithOptions(h, opts)

	return &http.Server{
		Addr:              addr,
		Handler:           router,
		ReadTimeout:       readTimeout,
		ReadHeaderTimeout: readHeaderTimeout,
		WriteTimeout:      writeTimeout,
		IdleTimeout:       idleTimeout,
	}
}
