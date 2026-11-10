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

// version: версия сборки для атрибута service.version.
// Проставляется при сборке: go build -ldflags "-X main.version=..."
var version = "dev"

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
	cfg.ServiceVersion = version

	// Отменяется по SIGINT/SIGTERM
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	c, err := container.New(ctx, cfg)
	if err != nil {
		return fmt.Errorf("build service: %w", err)
	}

	serverErr := make(chan error, 1)
	go func() {
		c.Log.Info("service started", "addr", cfg.HTTPAddr, "version", version)
		if err := c.Server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErr <- err
		}
	}()

	var runErr error
	select {
	case <-ctx.Done():
		c.Log.Info("shutdown signal received")
	case err := <-serverErr:
		runErr = fmt.Errorf("http server: %w", err)
	}

	return errors.Join(runErr, shutdown(c, cfg))
}

// shutdown: перестаём принимать запросы и ждём текущие, затем дописываем
// телеметрию и закрываем пул. Общий бюджет SHUTDOWN_TIMEOUT, по его
// истечении выходим принудительно и пишем об этом в лог
func shutdown(c *container.Container, cfg config.Config) error {
	ctx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer cancel()

	err := c.Server.Shutdown(ctx)
	if err != nil {
		c.Log.Error("graceful shutdown timed out, forcing exit", "error", err)
		err = fmt.Errorf("graceful shutdown: %w", err)
	}
	c.Close(ctx)

	if err == nil {
		c.Log.Info("service stopped")
	}
	return err
}
