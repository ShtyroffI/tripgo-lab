// Package create: ручка POST /api/v1/trips
package create

import (
	"fmt"
	"log/slog"
	"net/http"

	"github.com/ShtyroffI/tripgo-lab/api"
	"github.com/ShtyroffI/tripgo-lab/internal/http/domainerror"
	"github.com/ShtyroffI/tripgo-lab/internal/http/response"
)

type Handler struct {
	uc  usecase
	log *slog.Logger
}

func New(uc usecase, log *slog.Logger) *Handler {
	return &Handler{uc: uc, log: log}
}

// CreateTrip: разбор запроса, вызов usecase, ответ. Idempotency-Key
// принимается, но не используется (задание со звёздочкой лабы 1)
func (h *Handler) CreateTrip(w http.ResponseWriter, r *http.Request, _ api.CreateTripParams) {
	in, err := decodeRequest(w, r)
	if err != nil {
		domainerror.Write(w, r, h.log, err)
		return
	}

	t, err := h.uc.Create(r.Context(), in)
	if err != nil {
		domainerror.Write(w, r, h.log, err)
		return
	}

	w.Header().Set("Location", fmt.Sprintf("/api/v1/trips/%s", t.ID))
	response.JSON(w, http.StatusCreated, response.Trip(t))
}
