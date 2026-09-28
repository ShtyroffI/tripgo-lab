# TripGo: Trip Service

Учебный сервис такси (курс «Разработка микросервисов на Go»). В этом README:
как поднять сервис, какие переменные окружения он читает, что сделано в
текущей работе и какие приняты решения.

## Лабораторная работа 1: HTTP API и PostgreSQL

Сделано:

- HTTP-сервер на `net/http` + `chi`: `POST /api/v1/trips`,
  `GET /api/v1/trips/{tripId}`, `POST /api/v1/trips/{tripId}/finish`,
  `GET /health`, `GET /ready` по контракту
  `contracts/openapi/trip-service.openapi.yaml`;
- типы и серверные интерфейсы сгенерированы из OpenAPI (`make generate`);
- миграции `goose` для `trips` и `trip_status_history`;
- `pgxpool`, `squirrel` и собственный менеджер транзакций;
- ошибки в формате `application/problem+json` (RFC 9457) с полем `code`;
- graceful shutdown по `SIGINT` и `SIGTERM`.

Не сделано: задания со звёздочкой (`Idempotency-Key`, `Dockerfile`).
Заголовок `Idempotency-Key` сервис принимает, но игнорирует.

## Требования

- Go (версия указана в `go.mod`)
- Docker и `tripgoctl` (поднимают PostgreSQL)
- `make`
- `curl`, `jq`, `psql` для ручной проверки (по желанию)

## Как запустить

```bash
tripgoctl cluster start          # один раз на машине
tripgoctl environment start      # поднимает PostgreSQL и создаёт .env
tripgoctl connect                # показывает адреса

make migrate                     # накатить миграции
make run                         # запустить сервис
```

`make` сам читает настройки из `.env`. Остановить окружение:

```bash
tripgoctl environment stop
```

Другие команды:

| Команда | Что делает |
|---|---|
| `make generate` | генерирует `api/api.gen.go` из OpenAPI-контракта |
| `make migrate-down` | откатывает все миграции |
| `make test` | `go test -race ./...` (тестов в этой работе нет, они появятся в работе 2) |

## Структура

```
cmd/trip-service/   точка входа, сборка зависимостей
internal/config/    чтение переменных окружения
internal/domain/    поездка, статусы, доменные ошибки
internal/txmanager/ менеджер транзакций
internal/repository/ SQL-запросы (pgx + squirrel)
internal/httpserver/ хендлеры, роутер, problem+json
api/                код, сгенерированный из OpenAPI (не правится руками)
migrations/         миграции goose
contracts/          контракты курса (не редактируются)
```

## Переменные окружения

Список и значения по умолчанию для локального запуска: `.env.example`.
Реальный `.env` создаёт `tripgoctl environment start`, в git он не попадает.

| Переменная | Что задаёт |
|---|---|
| `HTTP_ADDR` | адрес HTTP-сервера, например `:8080` |
| `LOG_LEVEL` | уровень логов: `debug`, `info`, `warn`, `error` |
| `SHUTDOWN_TIMEOUT` | сколько ждём текущие запросы при остановке |
| `DATABASE_URL` | строка подключения к PostgreSQL |
| `DATABASE_MAX_CONNS`, `DATABASE_MIN_CONNS` | размер пула |
| `DATABASE_MAX_CONN_LIFETIME` | время жизни соединения |
| `DATABASE_CONNECT_TIMEOUT` | таймаут подключения и `Ping` на старте |
| `DATABASE_QUERY_TIMEOUT` | таймаут каждого запроса в БД, включая проверку в `/ready` |

Все переменные обязательные. Если какой-то нет или её нельзя разобрать,
сервис не стартует и печатает, что именно не так. Если при старте не
отвечает БД (`Ping`), сервис тоже не стартует.

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

**Уровень изоляции.** По умолчанию `Read Committed`, стандартный для
PostgreSQL. Серьёзнее изоляция здесь не нужна: от гонки при создании защищает
уникальный индекс в БД, а от двойного `finish` условие в `UPDATE`. Оба
механизма работают на `Read Committed`. `Serializable` добавил бы только
ретраи при конфликтах сериализации. Уровень задаётся один раз, в `Do`.

**Менеджер транзакций** (`internal/txmanager`). `Do(ctx, fn)` открывает
транзакцию, кладёт её в контекст под приватным ключом и вызывает `fn` с этим
контекстом. Если `fn` вернула `nil`, делается `COMMIT`. Если вернула ошибку
или запаниковала, делается `ROLLBACK`, ошибка или паника уходят наружу.
Репозиторий транзакцию аргументом не получает: перед каждым запросом он берёт
исполнителя из контекста (`ExecutorFromContext`). Есть транзакция, работает
через неё. Нет, работает через пул, то есть вызов вне `Do` выполнится
отдельным запросом без транзакции. Вложенный `Do` видит транзакцию в
контексте и просто вызывает `fn` в ней, вторую не открывает. Хендлеры знают
только интерфейс `Do`, про `pgx` они не знают.

**Атомарное создание.** `CreateTrip` делает `INSERT` в `trips` и `INSERT` в
`trip_status_history` в одном `Do`. Если вторая вставка падает, откатывается и
первая. Проверено: при сломанной второй вставке в `trips` ничего не остаётся.
`finish` тоже пишет в журнал в одной транзакции со сменой статуса.

**Запрет двух активных поездок.** Частичный уникальный индекс
`trips_one_active_per_driver_idx ON trips (driver_id) WHERE status = 'active'`
(миграция `00001_create_trips.sql`). Проверка `SELECT` перед `INSERT` не
подходит: между ней и вставкой пролезет параллельный запрос. Индекс проверяется
атомарно, поэтому из параллельных `INSERT` на одного водителя проходит один, а
остальные получают ошибку PostgreSQL `23505`. Репозиторий превращает её в
`domain.ErrDriverBusy`, хендлер отдаёт `409 driver_busy`. После завершения
поездки водитель снова может создать новую.

**Защита `finish` от гонки.** `UPDATE trips SET status = 'completed',
finished_at = now() WHERE id = $1 AND status = 'active'`. Условие в самом
`UPDATE` не оставляет окна между проверкой и записью: из двух параллельных
`finish` статус сменит только один, второй получит 0 изменённых строк.
Ноль строк значит «поездки нет» или «уже завершена», поэтому в этом случае
репозиторий дополнительно читает поездку и различает `404 trip_not_found` и
`409 trip_completed`. Если убрать условие `status = 'active'`, оба запроса
пройдут, и `finished_at` перезапишется.

**`/health` и `/ready`.** `/health` отвечает `200`, пока жив процесс, и не
ходит в БД: иначе оркестратор перезапускал бы здоровый сервис из-за проблем
базы. `/ready` делает `Ping` БД и отвечает `503`, если она недоступна:
оркестратор не шлёт трафик, но и не убивает процесс. Оба отвечают
`application/json` со схемой `HealthResponse`.

**Таймауты HTTP-сервера.** Заданы константами в `internal/httpserver/server.go`:
`ReadHeaderTimeout` 5 с, `ReadTimeout` 10 с, `WriteTimeout` 10 с,
`IdleTimeout` 60 с. Отдельных переменных окружения для них в курсе нет.

**Валидация.** Проверяется до бизнес-логики: обязательные поля, непустые UUID,
диапазоны координат, `price >= 0`, неизвестные поля запрещены (клиент не
может передать свой `id`). Время в ответах всегда в UTC.

**Особенность ответа.** Поля `finished_at` и `last_position_at` у активной
поездки в JSON не приходят (в примерах задания они `null`). Так генерирует
`oapi-codegen` для необязательных nullable-полей, контракт при этом не
нарушается.
