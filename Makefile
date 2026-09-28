.PHONY: generate migrate migrate-down run test

# Переменные из .env, его создаёт tripgoctl
-include .env
export

# Результат генерации коммитим, руками не правим
generate:
	go tool oapi-codegen \
		-generate types,chi-server \
		-package api \
		-o api/api.gen.go \
		-include-tags Trips,Service \
		contracts/openapi/trip-service.openapi.yaml

migrate:
	go tool goose -dir migrations postgres "$$DATABASE_URL" up

migrate-down:
	go tool goose -dir migrations postgres "$$DATABASE_URL" down-to 0

run:
	go run ./cmd/trip-service

test:
	go test -race ./...
