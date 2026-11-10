.PHONY: generate migrate migrate-down run lint test test-integration

# Переменные из .env, его создаёт tripgoctl
-include .env
export

# Пакеты, по которым считаем обязательное покрытие (не ниже 50%)
CORE_PKGS_RE := /internal/(domain|usecase)/
# Пакеты с интеграционными тестами (тег integration, нужен Docker)
INTEGRATION_PKGS := ./internal/repository/...

# Вся кодогенерация: OpenAPI и моки (go:generate рядом с contract.go).
# Результат коммитим, руками не правим
generate:
	go tool oapi-codegen \
		-generate types,chi-server \
		-package api \
		-o api/api.gen.go \
		-include-tags Trips,Service \
		contracts/openapi/trip-service.openapi.yaml
	go generate ./...

migrate:
	go tool goose -dir migrations postgres "$$DATABASE_URL" up

migrate-down:
	go tool goose -dir migrations postgres "$$DATABASE_URL" down-to 0

run:
	go run ./cmd/trip-service

lint:
	golangci-lint run ./...

# Все тесты: unit и интеграционные (PostgreSQL поднимает testcontainers).
# В конце печатаем общее покрытие и покрытие domain + usecase
test:
	go test -race -tags integration -coverprofile=coverage.out ./...
	@echo "Покрытие, всего:"
	@go tool cover -func=coverage.out | tail -1
	@grep -E '^mode:|$(CORE_PKGS_RE)' coverage.out > coverage.core.out
	@echo "Покрытие, domain + usecase:"
	@go tool cover -func=coverage.core.out | tail -1

# Только интеграционные тесты репозитория
test-integration:
	go test -race -tags integration $(INTEGRATION_PKGS)
