# TripGo: Trip Service

Учебный сервис такси (курс «Разработка микросервисов на Go»). В этом README:
как поднять сервис и прогнать тесты, какие переменные окружения он читает, как
устроен код, что сделано в каждой работе и какие приняты решения.

## Лабораторная работа 2: архитектура, тесты, observability

Снаружи сервис ведёт себя так же, как после работы 1: те же ручки, контракт
не менялся, новых миграций нет. Изменилось устройство кода внутри.

Сделано:

- код разложен по слоям: домен, usecase, транспорт, репозиторий,
  инфраструктура (раздел «Архитектура»);
- доменная модель с переходом статусов `Trip.Finish` и доменными ошибками,
  перевод ошибки в HTTP-ответ собран в одном месте (`internal/http/domainerror`);
- интерфейсы зависимостей объявлены на стороне потребителя, все зависимости
  собираются вручную в `internal/container`, `main` только запускает;
- `golangci-lint` с конфигом курса, правила слоёв проверяет `depguard`;
- тесты: unit на домен, usecase (`gomock`), маппинг ошибок, хендлеры
  (`httptest`) и middleware; интеграционные на репозиторий с настоящим
  PostgreSQL (`testcontainers-go`), включая откат транзакции;
- OpenTelemetry: трейсы HTTP и запросов в БД, метрики из каталога курса,
  экспорт по OTLP в коллектор; логи `slog` в JSON с `trace_id` и `span_id`;
- middleware `Recover → Tracing → Logging → Metrics`.

Задание со звёздочкой (мутационное тестирование) не делалось.

## Лабораторная работа 1: HTTP API и PostgreSQL

- `POST /api/v1/trips`, `GET /api/v1/trips/{tripId}`,
  `POST /api/v1/trips/{tripId}/finish`, `GET /health`, `GET /ready` по контракту
  `contracts/openapi/trip-service.openapi.yaml`, код сгенерирован `oapi-codegen`;
- миграции `goose` для `trips` и `trip_status_history`;
- `pgxpool`, `squirrel` и собственный менеджер транзакций;
- ошибки в формате `application/problem+json` (RFC 9457) с полем `code`;
- graceful shutdown по `SIGINT` и `SIGTERM`;
- со звёздочкой: многостадийный `Dockerfile`. `Idempotency-Key` сервис
  принимает, но игнорирует.

## Требования

- Go (версия указана в `go.mod`)
- Docker: нужен `tripgoctl` и тестам (`testcontainers` поднимает PostgreSQL)
- `tripgoctl`: PostgreSQL, коллектор OpenTelemetry, Jaeger, Prometheus, Grafana
- `make`, `golangci-lint` v2
- `curl`, `jq`, `psql` для ручной проверки (по желанию)

`oapi-codegen`, `goose` и `mockgen` ставить не нужно: это tool-зависимости из
`go.mod`, они запускаются через `go tool`.

## Как запустить

```bash
tripgoctl cluster start          # один раз на машине
tripgoctl environment start      # поднимает окружение лабы и создаёт .env
tripgoctl connect                # адреса и ссылки на Jaeger, Grafana, Prometheus

make migrate                     # накатить миграции
make run                         # запустить сервис
```

`make` сам читает настройки из `.env`. Остановить окружение:

```bash
tripgoctl environment stop
```

Команды:

| Команда | Что делает |
|---|---|
| `make run` | запускает сервис |
| `make migrate` / `make migrate-down` | накатывает / откатывает все миграции |
| `make generate` | вся кодогенерация: `api/api.gen.go` из OpenAPI и моки `mocks_test.go` (`go generate`) |
| `make lint` | `golangci-lint run ./...` с конфигом `.golangci.yml` |
| `make test` | все тесты: unit и интеграционные, с `-race`; в конце печатает общее покрытие и покрытие `domain` + `usecase` |
| `make test-integration` | только интеграционные тесты репозитория |

## Тесты

`make test` запускает всё: `go test -race -tags integration ./...`. Для этого
нужен запущенный Docker, `tripgoctl` не нужен: интеграционные тесты сами
поднимают PostgreSQL 16 в контейнере (`testcontainers-go`) и накатывают на него
миграции из `migrations/`. Первый запуск скачивает образ `postgres:16-alpine`.

Интеграционные тесты помечены `//go:build integration` и лежат в
`internal/repository/trip/*_integration_test.go`. Без тега (`go test ./...`)
запускаются только unit-тесты, без Docker. `make test-integration` гоняет
только интеграционные.

Каждый интеграционный тест создаёт свои данные с новыми UUID и удаляет их в
`t.Cleanup`, поэтому тесты не зависят друг от друга и проходят при повторном
запуске (`-count=2`).

Где какие сценарии:

| Сценарий | Тест |
|---|---|
| создание поездки, успех | `usecase/trip/create`: `TestUsecase_Create_Success`; `http/trip/create`: `TestHandler_CreateTrip_Created` |
| водитель занят → `ErrDriverBusy` → `409` | `TestUsecase_Create_DriverBusy`; `TestHandler_CreateTrip_UsecaseErrors`; репозиторий: `TestSaveSecondActiveTripForDriverReturnsDriverBusy` |
| завершение активной поездки | `TestUsecase_Finish_ActiveTrip_Success`; `TestHandler_FinishTrip_Success`; `TestFinishFlowInTransaction` |
| завершение завершённой → `ErrTripCompleted` → `409` | `TestUsecase_Finish_AlreadyCompleted_ErrTripCompleted`; `TestHandler_FinishTrip_Errors`; домен: `TestFinish` |
| несуществующая → `ErrTripNotFound` → `404` | `TestUsecase_Get_Errors`; `TestHandler_GetTrip_Errors`; `TestGetUnknownTripReturnsNotFound` |
| невалидное тело → `400 invalid_request` | `TestHandler_CreateTrip_InvalidBody_400InvalidRequest`; домен: `TestParamsValidate` |
| ошибка репозитория → `500` без деталей в ответе | `TestUsecase_Create_RepositoryError`, `TestUsecase_Finish_RepositoryError`, `TestUsecase_Get_Errors`; `TestHandler_*_Errors`, `TestHandler_CreateTrip_UsecaseErrors`; `domainerror`: `TestProblem_UnknownErrorHidesDetails`, `TestWrite` |
| откат обоих `INSERT` при ошибке | `TestCreateRollsBackBothInsertsOnHistoryFailure` |

Покрытие `internal/domain` и `internal/usecase`: 100%.

## Архитектура

### Слои

```
cmd/trip-service/        main: конфиг → container.New → запуск → сигнал → остановка
internal/
  config/                разбор и проверка переменных окружения
  container/             создаёт все реализации и связывает их
  domain/trip/           поездка, статусы, переход Finish, правила, доменные ошибки
  model/                 структуры между слоями (CreateTripInput)
  usecase/trip/          create, get, finish: бизнес-сценарии
  http/
    trip/                create, get, finish: пакет на ручку
    service/             /health, /ready
    server/              роутер chi, http.Server, таймауты
    domainerror/         ошибка → problem+json, одно место
    middleware/          Recover, Tracing, Logging, Metrics
    response/            запись JSON и перевод поездки в тип контракта
  repository/trip/       SQL на squirrel + pgx
  infra/
    postgres/            пул pgx и инструментирование запросов
      transaction/       менеджер транзакций
    telemetry/           OpenTelemetry: трейсы, метрики, логгер
api/                     код, сгенерированный из OpenAPI
```

Как идёт запрос на создание поездки:

```
HTTP → middleware → http/trip/create (разбор JSON → model.CreateTripInput)
     → usecase/trip/create (trip.New: правила домена; tx.Do: Save + AddStatusHistory)
     → repository/trip (SQL) → pgxpool
ошибка из любого слоя → http/domainerror → problem+json
```

### Кто от кого зависит

Зависимости направлены внутрь:

```
http ─────────► usecase ─────► domain ◄──── repository ◄──── infra
  │               │                              ▲
  └──► domain     └──► model ──► domain          │
container ──► всё (единственное место сборки)  ──┘
```

- `domain` не импортирует ничего из сервиса и никакой инфраструктуры: ни
  `net/http`, ни `chi`, ни `pgx`, ни `squirrel`, ни `otel`, ни `api`. Это видно
  так:
  ```bash
  go list -deps ./internal/domain/... | grep -E 'jackc/pgx|go-chi|net/http'
  ```
- `usecase` знает домен и `model`, но не транспорт, не бд и не сгенерированный
  `api`.
- `http` знает usecase только через свои интерфейсы, переводит запрос в
  `model` и не импортирует `repository` и `infra`.
- `repository` знает домен и менеджер транзакций из `infra`.

Правила закреплены в `.golangci.yml` (`depguard`): нарушение ломает
`make lint`. В выданный курсом конфиг добавлены правила из таблицы задания,
которых в нём не было: `domain` и `usecase` не импортируют `api`, `http` не
импортирует `repository` и `infra`.

### Где объявлены интерфейсы и почему там

Интерфейсы объявляет потребитель, в `contract.go` своего пакета, и они
неэкспортируемые. Например, `usecase/trip/finish/contract.go` описывает
`tripRepository` с тремя методами, которые нужны именно завершению поездки, и
`txManager` с одним `Do`. Реализация (`repository/trip.Repository`) про эти
интерфейсы не знает, Go проверяет совпадение методов при сборке в `container`.

Почему так:

- usecase формулирует, что ему нужно, а не подстраивается под реализацию.
  Интерфейсы узкие: у `get` один метод, у `finish` три, вместо одного общего
  `Storage`. Их легко мокать, и они точно показывают зависимость;
- моки (`mockgen` через `go:generate`) генерируются рядом в `mocks_test.go`:
  они нужны только тестам этого пакета;
- пакеты не мешают друг другу: изменение интерфейса `create` не трогает `get`.

Так же устроены остальные слои: хендлеры объявляют интерфейс `usecase`,
`http/server` собирает ручки через интерфейсы, `http/service` зависит от
`pinger`, репозиторий от `executorProvider`, usecase от `metrics`.

Все конкретные реализации создаются в одном месте, `internal/container`:
только там вызывается `postgres.New` (пул) и конструкторы репозитория, usecase,
хендлеров и сервера. DI-фреймворка нет, глобальных синглтонов нет: логгер,
провайдеры телеметрии и пул передаются аргументами конструкторов.

### Второй транспорт в работе 4

В работе 4 появится gRPC. Домен, usecase и репозиторий не меняются. Добавится:

- `internal/grpc/trip/{create,get,finish}`: свои хендлеры, каждый объявит в
  `contract.go` тот же узкий интерфейс usecase и переведёт protobuf в
  `model.CreateTripInput`;
- `internal/grpc/domainerror`: свой маппинг доменных ошибок в gRPC-коды
  (`ErrTripNotFound` → `NOT_FOUND` и т.д.), тоже в одном месте;
- в `container`: создание gRPC-сервера из тех же экземпляров usecase, в
  `main`: его запуск и остановка.

Именно поэтому usecase не принимает типы `api` и не знает про HTTP-коды: иначе
второй транспорт пришлось бы встраивать в бизнес-логику.

### Отступления от раскладки из задания

Где задание и сквозные требования курса (`docs/conventions.md`) расходятся,
сделано по сквозным требованиям:

- точка входа `cmd/trip-service/`, а не `cmd/service/`: так в разделе 8
  `conventions.md`;
- сгенерированный код в `api/`, а не в `internal/generated/`: так в разделе 8
  `conventions.md`, и именно `api/` исключён из линтера в конфиге курса;
- `container.New(ctx, cfg)` принимает `ctx`: при сборке сервис подключается к
  бд, а по разделу 2 `conventions.md` каждая функция с I/O принимает `ctx`;
- добавлен пакет `internal/http/response`: запись JSON и перевод поездки в
  `api.Trip` общие для трёх ручек, поэтому у `get` и `finish` нет своих
  `convert.go`;
- `make test` запускает и интеграционные тесты: по разделу 6 `conventions.md`
  `make test` запускает всё.

## Observability

Сервис отдаёт метрики и трейсы по OTLP в коллектор из окружения `tripgoctl`,
дальше они идут в Prometheus (смотреть в Grafana) и Jaeger. В коде нет ни
Prometheus, ни Jaeger, только OpenTelemetry. Ссылки на интерфейсы печатает
`tripgoctl connect`.

**Логи.** `log/slog`, JSON в stdout, уровень из `LOG_LEVEL`. В каждой записи
`time`, `level`, `msg`, `service`; если запись сделана внутри запроса, то и
`trace_id`, `span_id`. Логгер передаётся зависимостью.

- На каждый HTTP-запрос одна запись `request handled` после ответа: `method`,
  `route` (шаблон, без `tripId`), `status`, `duration_ms`. Тела запроса и
  ответа не пишем. `Info` для 2xx и 4xx, `Error` для 5xx.
- Бизнес-события пишет usecase: `trip created`, `trip completed` с `trip_id`.
- Внутренняя ошибка (500) логируется один раз, в `domainerror.Write`; клиенту
  уходит только `internal_error`.
- Паника: `panic recovered` со стектрейсом, клиенту `500 internal_error`.
- Старт и остановка: `service started`, `service stopped`.

**Трейсы.**

- Корневой спан на запрос создаёт `otelhttp`, имя `{method} {http.route}`,
  например `POST /api/v1/trips/{tripId}/finish`. Контекст трейса принимается из
  заголовков W3C TraceContext.
- Каждый запрос в БД — дочерний спан (`otelpgx` на уровне пула) с
  `db.operation.name` и `db.collection.name`. Репозиторий про телеметрию не
  знает.
- Ошибка помечает спан `codes.Error` и пишет `error.type`: для ответа 5xx код
  статуса, для ошибки БД код SQLSTATE (например, `23505` у занятого водителя).
- Сэмплирование задаёт `OTEL_TRACES_SAMPLER`.

**Метрики.**

| Метрика | Тип | Лейблы |
|---|---|---|
| `http.server.request.duration` | histogram, с | `http.request.method`, `http.route`, `http.response.status_code` |
| `http.server.active_requests` | up-down counter | `http.request.method`, `http.route` |
| `db.client.operation.duration` | histogram, с | `db.operation.name`, `db.collection.name`, `error.type` |
| `tripgo.trips.created` | counter | — |
| `tripgo.trips.completed` | counter | — |
| `tripgo.trips.active` | gauge | — |

В `http.route` только шаблон маршрута: если класть туда путь с `tripId`, на
каждую поездку заведётся своя временная серия, метрика перестанет
агрегироваться и раздует хранилище. Запрос на несуществующий путь получает
метрику без `http.route`. В Prometheus имена с точками превращаются в
`http_server_request_duration_seconds`, `tripgo_trips_created_total` и т.д.

**Без коллектора** сервис работает: экспорт идёт фоном, при старте к коллектору
не подключаемся. Ошибки отправки пишутся в лог как `Warn`, при остановке
неотправленный остаток теряется (тоже `Warn`), код выхода 0.

Как проверить:

1. `make run`, создать поездку (раздел «Как проверить» ниже).
2. Jaeger: сервис `trip-service`, трейс `POST /api/v1/trips` со спанами
   `BEGIN`, `INSERT` в `trips`, `INSERT` в `trip_status_history`, `COMMIT`.
3. Взять `trace_id` из записи `request handled` и найти трейс в Jaeger.
4. Grafana → Explore → Prometheus: `http_server_request_duration_seconds_count`,
   `tripgo_trips_created_total`.

## Docker

`deploy/Dockerfile`: многостадийная сборка. Первая стадия (`golang:1.27.1-alpine`)
собирает статический бинарь, вторая (`gcr.io/distroless/static-debian12:nonroot`)
содержит только его. В итоговом образе нет исходников, Go, кэша сборки и shell,
процесс запускается от пользователя `nonroot` (uid 65532). `.dockerignore` не
пускает в контекст сборки `.git`, `.env*`, `.tripgo`, тесты, документацию и
локальные файлы.

Размер итогового образа: **35.6 МБ** (в работе 1 был 20.8 МБ, добавился
OpenTelemetry SDK с экспортёрами).

Сборка из корня репозитория, `VERSION` попадает в атрибут `service.version`:

```bash
docker build -f deploy/Dockerfile --build-arg VERSION=lab2 -t trip-service:lab2 .
```

Запуск рядом с окружением `tripgoctl`. Контейнер подключается к Docker-сети
`kind`, где живёт узел кластера, а PostgreSQL и коллектор доступны на его
NodePort. Из `.env` берём все переменные, а адреса подменяем: `localhost`
внутри контейнера означает сам контейнер. NodePort у каждой лабы свой, для
лабы 2: PostgreSQL `30203`, OTLP HTTP `30206`.

```bash
NODE_IP=$(docker inspect tripgo-local-control-plane \
  --format '{{ .NetworkSettings.Networks.kind.IPAddress }}')

docker run --rm --name trip-service --network kind -p 8080:8080 \
  --env-file .env \
  -e DATABASE_URL="postgres://tripgo:tripgo@${NODE_IP}:30203/tripgo?sslmode=disable" \
  -e OTEL_EXPORTER_OTLP_ENDPOINT="http://${NODE_IP}:30206" \
  trip-service:lab2

curl -sS -i localhost:8080/ready
```

Миграции контейнер не накатывает, их выполняет `make migrate`.

## Переменные окружения

Список и значения по умолчанию для локального запуска: `.env.example`.
Реальный `.env` создаёт `tripgoctl environment start`, в git он не попадает.

| Переменная | Что задаёт |
|---|---|
| `HTTP_ADDR` | адрес HTTP-сервера, например `:8080` |
| `LOG_LEVEL` | уровень логов: `debug`, `info`, `warn`, `error` |
| `SHUTDOWN_TIMEOUT` | общий бюджет на остановку: текущие запросы, отправка телеметрии |
| `DATABASE_URL` | строка подключения к PostgreSQL |
| `DATABASE_MAX_CONNS`, `DATABASE_MIN_CONNS` | размер пула |
| `DATABASE_MAX_CONN_LIFETIME` | время жизни соединения |
| `DATABASE_CONNECT_TIMEOUT` | таймаут подключения и `Ping` на старте |
| `DATABASE_QUERY_TIMEOUT` | таймаут каждого запроса в БД, включая проверку в `/ready` и подсчёт `tripgo.trips.active` |
| `OTEL_SERVICE_NAME` | `service.name` в телеметрии и поле `service` в логах |
| `OTEL_EXPORTER_OTLP_ENDPOINT` | адрес коллектора |
| `OTEL_EXPORTER_OTLP_PROTOCOL` | `http/protobuf` или `grpc` |
| `OTEL_RESOURCE_ATTRIBUTES` | `service.namespace`, `deployment.environment` |
| `OTEL_TRACES_SAMPLER` | сэмплер трейсов, например `parentbased_always_on` |

`OTEL_*` — стандартные переменные OpenTelemetry: адрес, атрибуты и сэмплер
SDK читает из окружения сам, сервис только проверяет, что они заданы.

Все переменные обязательные. Если какой-то нет, её нельзя разобрать или
значение не имеет смысла (таймаут не больше 0, `DATABASE_MAX_CONNS` меньше 1,
`DATABASE_MIN_CONNS` больше максимума, неизвестный протокол OTLP), сервис не
стартует и печатает, что именно не так. Если при старте не отвечает БД
(`Ping`), сервис тоже не стартует. Недоступный коллектор старту не мешает.

## Как проверить

```bash
# создание, ожидаем 201 и заголовок Location
curl -sS -i -X POST localhost:8080/api/v1/trips \
  -H 'content-type: application/json' \
  -d '{"user_id":"5cb72c04-7650-45c9-a79b-bcdba0631e0c",
       "driver_id":"8860b315-ec86-42eb-a17c-7c163d721ff5",
       "start_point":{"latitude":59.9398,"longitude":30.3146},
       "end_point":{"latitude":59.9290,"longitude":30.3626},
       "price":1450}'

# тот же запрос ещё раз: 409 driver_busy

curl -sS localhost:8080/api/v1/trips/$TRIP_ID | jq
curl -sS -i -X POST localhost:8080/api/v1/trips/$TRIP_ID/finish   # 200
curl -sS -i -X POST localhost:8080/api/v1/trips/$TRIP_ID/finish   # 409 trip_completed
curl -sS -i localhost:8080/health
curl -sS -i localhost:8080/ready
```

Гонка на одного водителя: ровно один `201`, остальные `409`
(в `trip.json` лежит тело запроса из примера выше).

```bash
seq 20 | xargs -P20 -I{} curl -s -o /dev/null -w '%{http_code}\n' \
  -X POST localhost:8080/api/v1/trips -H 'content-type: application/json' \
  -d @trip.json | sort | uniq -c
```

Коды ошибок:

| HTTP | `code` | Когда |
|---|---|---|
| 400 | `invalid_request` | тело не прошло валидацию или `tripId` не UUID |
| 404 | `trip_not_found` | поездки нет |
| 409 | `trip_completed` | поездка уже завершена |
| 409 | `driver_busy` | у водителя уже есть активная поездка |
| 500 | `internal_error` | остальное, подробности только в логе |

## Решения

**Ошибки.** Доменные ошибки (`ErrTripNotFound`, `ErrTripCompleted`,
`ErrDriverBusy`) и тип `*ValidationError` с полем и причиной объявлены в
`internal/domain/trip/errors.go`. Про HTTP домен не знает. Usecase оборачивает
ошибки с контекстом (`fmt.Errorf("finish trip: %w", err)`), хендлер не
разбирает их, а отдаёт в `domainerror`. Там единственный `switch` по
`errors.Is` / `errors.As`; всё неизвестное становится `500 internal_error` с
общим текстом, а сама ошибка уходит в лог. Ошибки разбора на уровне
транспорта (не JSON, `tripId` не UUID) — тип `domainerror.BadRequestError`,
тоже `400 invalid_request`.

**Валидация.** Разбор JSON остаётся в транспорте: обязательные поля
(отсутствующее поле и `null` отклоняются, настоящие `0` в `price` и
координатах допустимы) и запрет неизвестных полей (клиент не может передать
свой `id`). Правила значений (непустые UUID, диапазоны координат,
`price >= 0`) проверяет домен в `trip.New`, они не зависят от транспорта и
будут работать и для gRPC. Время в ответах всегда в UTC.

**Уровень изоляции.** `Read Committed`, стандартный для PostgreSQL. Серьёзнее
изоляция здесь не нужна: от гонки при создании защищает уникальный индекс в
БД, а от двойного `finish` блокировка строки. Оба механизма работают на
`Read Committed`. `Serializable` добавил бы только ретраи при конфликтах
сериализации. Уровень задаётся один раз, в `Do`.

**Менеджер транзакций** (`internal/infra/postgres/transaction`). `Do(ctx, fn)`
открывает транзакцию, кладёт её в контекст под приватным ключом и вызывает
`fn` с этим контекстом. Если `fn` вернула `nil`, делается `COMMIT`. Если
вернула ошибку или запаниковала, делается `ROLLBACK`, ошибка или паника уходят
наружу. Репозиторий транзакцию аргументом не получает: перед каждым запросом
он спрашивает исполнителя у менеджера (`Executor(ctx)`). Есть транзакция в
контексте — работает через неё, нет — через пул. Вложенный `Do` видит
транзакцию в контексте и просто вызывает `fn` в ней. Usecase знает только
интерфейс с методом `Do`, про `pgx` он не знает.

**Атомарное создание.** Usecase `create` делает `INSERT` в `trips` и `INSERT`
в `trip_status_history` в одном `Do`. Если вторая вставка падает,
откатывается и первая; это проверяет интеграционный тест
`TestCreateRollsBackBothInsertsOnHistoryFailure`. `finish` тоже пишет в
журнал в одной транзакции со сменой статуса.

**Запрет двух активных поездок.** Частичный уникальный индекс
`trips_one_active_per_driver_idx ON trips (driver_id) WHERE status = 'active'`
(миграция `00001_create_trips.sql`). Проверка `SELECT` перед `INSERT` не
подходит: между ней и вставкой пролезет параллельный запрос. Индекс
проверяется атомарно, поэтому из параллельных `INSERT` на одного водителя
проходит один, а остальные получают ошибку PostgreSQL `23505`. Репозиторий
превращает её в `ErrDriverBusy`, транспорт отдаёт `409 driver_busy`.

**Завершение поездки и гонка.** Правило «завершить можно только активную
поездку» описано в домене, `Trip.Finish`. Usecase в одной транзакции:

1. `SELECT ... FOR UPDATE`: читает поездку и блокирует строку до конца
   транзакции (нет строки — `ErrTripNotFound`, `404`);
2. `trip.Finish(now)`: домен решает, можно ли (нет — `ErrTripCompleted`,
   `409`);
3. `UPDATE ... WHERE id = $1 AND status = 'active'` и запись в журнал.

Параллельный `finish` ждёт на блокировке, а после коммита первого читает уже
завершённую поездку и получает `409`. Условие `status = 'active'` в `UPDATE`
оставлено страховкой: если кто-то вызовет сохранение без блокировки, второй
запрос всё равно получит 0 строк и `ErrTripCompleted`. В работе 1 правило
жило только в SQL, теперь его видно в доменной модели и его можно проверить
тестом без бд.

**Бизнес-метрики через интерфейс.** Счётчики созданных и завершённых поездок
увеличивает usecase, но через свой интерфейс `metrics` с одним методом:
usecase не зависит от OpenTelemetry, а в тестах мок проверяет, что счётчик
растёт только при успехе. Gauge `tripgo.trips.active` снимается в момент
сбора метрик запросом `count(*)` с таймаутом `DATABASE_QUERY_TIMEOUT`.

**Инструментирование БД.** Спаны создаёт `otelpgx`, он подключён трейсером к
пулу, поэтому покрыт каждый запрос, включая `BEGIN` и `COMMIT`. `otelpgx` не
ставит `db.collection.name` и `error.type`, а его метрика длительности без
этих лейблов. Поэтому поверх него обёртка в `internal/infra/postgres/tracer.go`:
добавляет эти атрибуты в спан и сама пишет `db.client.operation.duration`.
Таблицу и операцию берём из SQL: запросы строит `squirrel`, у них простая
форма. Значения параметров в атрибуты не попадают, в SQL только плейсхолдеры.

**Порядок middleware.** `Recover → Tracing → Logging → Metrics`, все
подключены через `chi.Router.Use`, то есть работают внутри роутера: после
обработки запроса им известен шаблон маршрута. `Recover` снаружи всех, чтобы
поймать панику и в самих middleware; внутренние middleware при панике успевают
записать спан, лог и метрику со статусом 500 и пробрасывают её дальше.

**Глобальное состояние.** Провайдеры трейсов и метрик не регистрируются
глобально, `container` передаёт их явно в middleware и пул. Исключение одно:
обработчик ошибок экспорта OpenTelemetry (`otel.SetErrorHandler`) бывает
только глобальным; без него недоступность коллектора печаталась бы в stderr
не в JSON.

**Остановка.** По сигналу: `http.Server.Shutdown` (перестаём принимать
запросы, ждём текущие), затем отправка оставшейся телеметрии, затем закрытие
пула. Телеметрия раньше пула, потому что последний сбор метрик ходит в бд за
`tripgo.trips.active`. Общий бюджет `SHUTDOWN_TIMEOUT`.

**`/health` и `/ready`.** `/health` отвечает `200`, пока жив процесс, и не
ходит в БД: иначе оркестратор перезапускал бы здоровый сервис из-за проблем
базы. `/ready` делает `Ping` БД и отвечает `503`, если она недоступна:
оркестратор не шлёт трафик, но и не убивает процесс.

**Таймауты HTTP-сервера.** Заданы константами в `internal/http/server/server.go`:
`ReadHeaderTimeout` 5 с, `ReadTimeout` 10 с, `WriteTimeout` 10 с,
`IdleTimeout` 60 с. Отдельных переменных окружения для них в курсе нет.

**Особенность ответа.** Поля `finished_at` и `last_position_at` у активной
поездки в JSON не приходят (в примерах задания они `null`). Так генерирует
`oapi-codegen` для необязательных nullable-полей, контракт при этом не
нарушается.
