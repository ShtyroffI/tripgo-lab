//go:generate go tool mockgen -source ${GOFILE} -destination mocks_test.go -package ${GOPACKAGE}_test
package service

import "context"

// pinger: проверка доступности бд для /ready. Реализует пул pgx
type pinger interface {
	Ping(ctx context.Context) error
}
