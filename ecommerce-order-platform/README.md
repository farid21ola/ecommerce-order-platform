# Ecommerce Order Platform

Прототип микросервисной e-commerce системы управления заказами на Go.

## Цель

Проект демонстрирует асинхронную обработку заказов через Kafka, Saga orchestration, Transactional Outbox и read model для чтения агрегированного состояния заказа.

## Текущий этап

Реализован рабочий прототип: API Gateway, Saga orchestration, Inventory, Payment, Delivery, Notification, Read Model, Kafka events и PostgreSQL миграции.

## Структура

```text
cmd/                  entrypoint каждого сервиса
internal/             внутренняя реализация сервисов
pkg/                  общие пакеты
migrations/           миграции БД по сервисам
Dockerfile            общий Dockerfile для сборки сервиса по SERVICE
docker-compose.yml    PostgreSQL и Kafka для локального запуска
Makefile              команды разработки
```

## Сервисы

```text
api-gateway
order-service
inventory-service
payment-service
delivery-service
notification-service
read-model-service
```

## Order Service

`order-service` доступен локально на `http://localhost:8081`.

Минимальные endpoints:

```text
GET /healthz
POST /orders
```

`POST /orders` создаёт заказ в `order_db`, записывает `OrderCreated` в `outbox_events`, а outbox worker публикует событие в `orders.events`.

Order Service также слушает:

```text
inventory.events
payments.events
delivery.events
```

и оркестрирует Saga через исходящие события в `orders.events`.

## API Gateway

`api-gateway` доступен локально на `http://localhost:8080`.

Встроенный UI доступен на `http://localhost:8080/`.

Endpoints:

```text
GET /healthz
POST /orders
POST /orders/{order_id}/cancel
GET /orders/{order_id}
GET /orders/{order_id}/history
```

Gateway не содержит бизнес-логику: write-запросы проксируются в `order-service`, read-запросы в `read-model-service`.

## Inventory Service

`inventory-service` доступен локально на `http://localhost:8082`.

Минимальный endpoint:

```text
GET /healthz
```

Inventory Service слушает `orders.events`, обрабатывает `OrderCreated` и `ReleaseStockRequested`, меняет остатки в `inventory_db` и публикует результат в `inventory.events`.

## Payment Service

`payment-service` доступен локально на `http://localhost:8083`.

Минимальный endpoint:

```text
GET /healthz
```

Payment Service слушает `orders.events`, обрабатывает `PaymentRequested`, создаёт запись в `payment_db` и публикует `PaymentCompleted` или `PaymentFailed` в `payments.events`. Для тестовой симуляции используется `payment_scenario` из события.

## Delivery Service

`delivery-service` доступен локально на `http://localhost:8084`.

Минимальный endpoint:

```text
GET /healthz
```

Delivery Service слушает `orders.events`, обрабатывает `DeliveryRequested`, создаёт доставку в `delivery_db` и публикует `DeliveryCreated` в `delivery.events`.

## Notification Service

`notification-service` доступен локально на `http://localhost:8086`.

Минимальный endpoint:

```text
GET /healthz
```

Notification Service слушает финальные и важные события (`OrderCompleted`, `OrderFailed`, `OrderCancelled`, `PaymentFailed`, `DeliveryCreated`), создаёт записи в `notification_db` и публикует `NotificationSent` в `notifications.events`.

## Read Model Service

`read-model-service` доступен локально на `http://localhost:8085`.

Endpoints:

```text
GET /healthz
GET /orders/{order_id}
GET /orders/{order_id}/history
```

Read Model Service слушает все event topics, обновляет `read_db.order_view` и пишет историю в `read_db.order_event_history`.

## Быстрый старт

```sh
docker compose up --build -d
make ps
```

Остановить инфраструктуру:

```sh
make down
```

## Команды разработки

```sh
make fmt
make test
make build
make docker-build
```

## Нагрузочная Проверка

Скрипт создаёт заказы разных типов через API Gateway и выводит сводку по статусам:

```sh
scripts/load_test.sh
```

Настраиваемые параметры:

```sh
SUCCESS_SKU1_COUNT=20 \
FAIL_PAYMENT_SKU1_COUNT=10 \
SUCCESS_SKU2_COUNT=10 \
FAIL_PAYMENT_SKU2_COUNT=5 \
WAIT_SECONDS=20 \
scripts/load_test.sh
```

HTML-отчёт с SVG-графиками для диплома:

```sh
python3 scripts/load_test_report.py \
  --success-sku1 20 \
  --fail-sku1 10 \
  --success-sku2 10 \
  --fail-sku2 5 \
  --concurrency 10 \
  --wait 20 \
  --output reports/load_test_report.html
```

Откройте `reports/load_test_report.html` в браузере и используйте графики в отчёте.

## Общие пакеты

```text
pkg/events    единый Kafka event envelope, типы событий и payload DTO
pkg/kafka     producer/consumer поверх segmentio/kafka-go
pkg/db        подключение к PostgreSQL через pgxpool
```

## Миграции

Миграции лежат отдельно по владельцам данных:

```text
migrations/order
migrations/inventory
migrations/payment
migrations/delivery
migrations/notification
migrations/readmodel
```

Файлы совместимы с `golang-migrate`: `*.up.sql` и `*.down.sql`.

При чистом запуске `docker compose up --build` PostgreSQL автоматически создаёт сервисные БД, применяет миграции и добавляет seed-данные inventory:

```text
SKU-001 available=10 reserved=0
SKU-002 available=0 reserved=0
```

## Инфраструктура

Локальный `docker-compose.yml` поднимает:

```text
api-gateway: localhost:8080
order-service: localhost:8081
inventory-service: localhost:8082
payment-service: localhost:8083
delivery-service: localhost:8084
read-model-service: localhost:8085
notification-service: localhost:8086
postgres: localhost:5432
kafka: localhost:9092
```

Kafka topics создаются автоматически через `kafka-init`.
