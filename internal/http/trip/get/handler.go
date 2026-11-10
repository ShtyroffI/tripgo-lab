// Package get: ручка GET /api/v1/trips/{tripId}
package get

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

// GetTrip: tripId уже разобран сгенерированной обёрткой, не UUID даёт 400
// ещё до хендлера
func (h *Handler) GetTrip(w http.ResponseWriter, r *http.Request, tripID api.TripId) {
	t, err := h.uc.Get(r.Context(), tripID)
	if err != nil {
		domainerror.Write(w, r, h.log, err)
		return
	}

	response.JSON(w, http.StatusOK, response.Trip(t))
}
