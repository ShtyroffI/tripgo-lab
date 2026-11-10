// Package service: служебные ручки /health и /ready
package service

import (
	"context"
	"net/http"
	"time"

	"github.com/ShtyroffI/tripgo-lab/api"
	"github.com/ShtyroffI/tripgo-lab/internal/http/response"
)

type Handler struct {
	db           pinger
	queryTimeout time.Duration
}

func New(db pinger, queryTimeout time.Duration) *Handler {
	return &Handler{db: db, queryTimeout: queryTimeout}
}

// Health: процесс жив. В бд не ходит, иначе оркестратор перезапускал бы
// здоровый сервис из-за проблем базы
func (h *Handler) Health(w http.ResponseWriter, _ *http.Request) {
	response.JSON(w, http.StatusOK, api.HealthResponse{Status: "ok"})
}

// Ready: сервис готов принимать трафик, то есть бд отвечает
func (h *Handler) Ready(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), h.queryTimeout)
	defer cancel()

	if err := h.db.Ping(ctx); err != nil {
		response.JSON(w, http.StatusServiceUnavailable, api.HealthResponse{Status: "unavailable"})
		return
	}
	response.JSON(w, http.StatusOK, api.HealthResponse{Status: "ok"})
}
