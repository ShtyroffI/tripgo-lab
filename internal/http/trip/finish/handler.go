// Package finish: ручка POST /api/v1/trips/{tripId}/finish
package finish

import (
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

func (h *Handler) FinishTrip(w http.ResponseWriter, r *http.Request, tripID api.TripId) {
	t, err := h.uc.Finish(r.Context(), tripID)
	if err != nil {
		domainerror.Write(w, r, h.log, err)
		return
	}

	response.JSON(w, http.StatusOK, response.Trip(t))
}
