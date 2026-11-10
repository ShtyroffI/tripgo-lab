package service_test

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"

	"github.com/ShtyroffI/tripgo-lab/internal/http/service"
)

func TestHandler_Health(t *testing.T) {
	t.Parallel()

	// У pinger нет ожиданий: вызов Ping провалит тест
	h := service.New(NewMockpinger(gomock.NewController(t)), time.Second)
	rec := httptest.NewRecorder()

	h.Health(rec, httptest.NewRequest(http.MethodGet, "/health", nil))

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.JSONEq(t, `{"status":"ok"}`, rec.Body.String())
}

func TestHandler_Ready(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		pingErr    error
		wantStatus int
		wantBody   string
	}{
		{"database is up", nil, http.StatusOK, `{"status":"ok"}`},
		{"database is down", errors.New("connection refused"), http.StatusServiceUnavailable, `{"status":"unavailable"}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			pinger := NewMockpinger(gomock.NewController(t))
			pinger.EXPECT().Ping(gomock.Any()).Return(tt.pingErr)
			h := service.New(pinger, time.Second)
			rec := httptest.NewRecorder()

			h.Ready(rec, httptest.NewRequest(http.MethodGet, "/ready", nil))

			assert.Equal(t, tt.wantStatus, rec.Code)
			assert.JSONEq(t, tt.wantBody, rec.Body.String())
			assert.NotContains(t, rec.Body.String(), "refused")
		})
	}
}
