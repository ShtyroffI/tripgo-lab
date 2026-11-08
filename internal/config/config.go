// Package config: настройки из переменных окружения
package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"time"
)

type Config struct {
	HTTPAddr        string
	LogLevel        string
	ShutdownTimeout time.Duration

	DatabaseURL             string
	DatabaseMaxConns        int32
	DatabaseMinConns        int32
	DatabaseMaxConnLifetime time.Duration
	DatabaseConnectTimeout  time.Duration
	DatabaseQueryTimeout    time.Duration
}

// loader копит ошибки, чтобы показать все проблемы разом
type loader struct {
	errs []error
}

func (l *loader) string(name string) string {
	v := os.Getenv(name)
	if v == "" {
		l.errs = append(l.errs, fmt.Errorf("%s: переменная не задана", name))
	}
	return v
}

func (l *loader) duration(name string) time.Duration {
	v := os.Getenv(name)
	if v == "" {
		l.errs = append(l.errs, fmt.Errorf("%s: переменная не задана", name))
		return 0
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		l.errs = append(l.errs, fmt.Errorf("%s: не похоже на длительность (%q): %w", name, v, err))
		return 0
	}
	return d
}

func (l *loader) int32(name string) int32 {
	v := os.Getenv(name)
	if v == "" {
		l.errs = append(l.errs, fmt.Errorf("%s: переменная не задана", name))
		return 0
	}
	n, err := strconv.ParseInt(v, 10, 32)
	if err != nil {
		l.errs = append(l.errs, fmt.Errorf("%s: не похоже на число (%q): %w", name, v, err))
		return 0
	}
	return int32(n)
}

func Load() (Config, error) {
	l := &loader{}

	cfg := Config{
		HTTPAddr:        l.string("HTTP_ADDR"),
		LogLevel:        l.string("LOG_LEVEL"),
		ShutdownTimeout: l.duration("SHUTDOWN_TIMEOUT"),

		DatabaseURL:             l.string("DATABASE_URL"),
		DatabaseMaxConns:        l.int32("DATABASE_MAX_CONNS"),
		DatabaseMinConns:        l.int32("DATABASE_MIN_CONNS"),
		DatabaseMaxConnLifetime: l.duration("DATABASE_MAX_CONN_LIFETIME"),
		DatabaseConnectTimeout:  l.duration("DATABASE_CONNECT_TIMEOUT"),
		DatabaseQueryTimeout:    l.duration("DATABASE_QUERY_TIMEOUT"),
	}

	// Смысл проверяем, только если всё разобралось, иначе нули от ошибок
	if len(l.errs) == 0 {
		l.errs = cfg.validate()
	}
	if len(l.errs) > 0 {
		return Config{}, fmt.Errorf("конфигурация: %w", errors.Join(l.errs...))
	}

	return cfg, nil
}

func (c Config) validate() []error {
	var errs []error

	if c.ShutdownTimeout <= 0 {
		errs = append(errs, errors.New("SHUTDOWN_TIMEOUT: должен быть больше 0"))
	}
	if c.DatabaseConnectTimeout <= 0 {
		errs = append(errs, errors.New("DATABASE_CONNECT_TIMEOUT: должен быть больше 0"))
	}
	if c.DatabaseQueryTimeout <= 0 {
		errs = append(errs, errors.New("DATABASE_QUERY_TIMEOUT: должен быть больше 0"))
	}
	if c.DatabaseMaxConnLifetime <= 0 {
		errs = append(errs, errors.New("DATABASE_MAX_CONN_LIFETIME: должен быть больше 0"))
	}
	if c.DatabaseMaxConns < 1 {
		errs = append(errs, errors.New("DATABASE_MAX_CONNS: должен быть не меньше 1"))
	}
	if c.DatabaseMinConns < 0 {
		errs = append(errs, errors.New("DATABASE_MIN_CONNS: не может быть отрицательным"))
	}
	if c.DatabaseMinConns > c.DatabaseMaxConns {
		errs = append(errs, errors.New("DATABASE_MIN_CONNS: не может быть больше DATABASE_MAX_CONNS"))
	}
	return errs
}
