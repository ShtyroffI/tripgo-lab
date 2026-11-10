// Command trip-service: HTTP API поездок. main только читает конфигурацию,
// собирает контейнер, запускает сервер и ждёт сигнала остановки
package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/ShtyroffI/tripgo-lab/internal/config"
	"github.com/ShtyroffI/tripgo-lab/internal/container"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	// Отменяется по SIGINT/SIGTERM
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	c, err := container.New(ctx, cfg)
	if err != nil {
		return fmt.Errorf("build service: %w", err)
	}
	defer c.Close()

	serverErr := make(chan error, 1)
	go func() {
		c.Log.Info("http server starting", "addr", cfg.HTTPAddr)
		if err := c.Server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErr <- err
		}
	}()

	select {
	case <-ctx.Done():
		c.Log.Info("shutdown signal received")
	case err := <-serverErr:
		return fmt.Errorf("http server: %w", err)
	}

	// Сначала перестаём принимать запросы и ждём текущие, потом закрываем
	// пул (defer c.Close). Общий бюджет SHUTDOWN_TIMEOUT
	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer cancel()

	if err := c.Server.Shutdown(shutdownCtx); err != nil {
		c.Log.Error("graceful shutdown timed out, forcing exit", "error", err)
		return fmt.Errorf("graceful shutdown: %w", err)
	}

	c.Log.Info("server stopped gracefully")
	return nil
}
