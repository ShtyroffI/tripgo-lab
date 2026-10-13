# Сборка из корня репозитория: docker build -f deploy/Dockerfile -t trip-service .

# Стадия сборки: тут есть Go и исходники, в итоговый образ они не попадут
FROM golang:1.27.1-alpine AS build
WORKDIR /src

# Зависимости отдельным слоем, чтобы кэш не сбрасывался при правке кода
COPY go.mod go.sum ./
RUN go mod download

COPY . .
# CGO выключен: бинарь статический, ему не нужны системные библиотеки
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/trip-service ./cmd/trip-service

# Итоговый образ: только бинарь, без shell и пакетного менеджера
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/trip-service /trip-service

# Не root
USER nonroot:nonroot

EXPOSE 8080
ENTRYPOINT ["/trip-service"]
