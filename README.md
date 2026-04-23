# E-commerce Event-Driven Backend (Go)

Backend de e-commerce orientado a **arquitectura event-driven**, construido en Go con foco en patrones de consistencia distribuida.

## ✨ Highlights técnicos

- **Go + Echo + pgx + RabbitMQ**
- **SAGA orchestration** (Order inicia, Inventory responde, Order cierra estado)
- **Outbox Pattern** para consistencia entre DB y broker
- **Routing explícito** por `routing_key` (sin inferencia por JSON)
- **Channels RabbitMQ separados** por responsabilidad (topology/publish/consume)
- **Ack/Nack manual** en consumers
- **Fix anti-overselling** con reserva atómica en PostgreSQL
- **Validación robusta de input** en el borde HTTP
- **Graceful shutdown** (señales + cierre ordenado de HTTP/workers/recursos)
- **Re-suscripción automática** de consumers ante corte de canal RabbitMQ
- **Timeouts operativos** en startup y procesamiento crítico
- **Startup retry** con backoff para DB/RabbitMQ/topology
- **Logs estructurados** con `correlation_id` para trazabilidad
- **Idempotencia final** con persistencia de outcome en inventory
- **Métricas operativas básicas** en endpoint `/metrics`
- **Tests de hardening** (handler, concurrencia inventory, relay/outbox)

## 🧱 Stack

- **Lenguaje:** Go 1.26
- **HTTP:** Echo v4
- **DB:** PostgreSQL (pgx/v5)
- **Mensajería:** RabbitMQ (`amqp091-go`)
- **Infra local:** Docker Compose
- **Testing de integración:** Testcontainers

## 🧠 Arquitectura (resumen)

Flujo principal:

1. `POST /orders`
2. `OrderService` guarda orden + evento en `outbox` (misma transacción)
3. `Relay` publica `order.created` en exchange `orders`
4. `InventoryConsumer` procesa y publica `inventory.reserved` o `inventory.failed`
5. `OrderConsumer` consume respuesta y actualiza estado de la orden

## 📂 Estructura relevante

```text
.
├── main.go
├── db/
│   ├── migrations/      # schema base reproducible
│   └── seeds/           # datos demo reproducibles
├── demo/                # demo web mínima conectada al backend real (PR #14)
├── pkg/
│   ├── events/          # Contratos de eventos
│   └── rabbitmq/        # Cliente, topology setup, publish/consume
├── services/
│   ├── order/           # API + saga orchestration + outbox relay
│   ├── inventory/       # Reserva de stock + respuesta de saga
│   ├── product/
│   └── user/
└── docs/
    ├── ARQUITECTURA_RESUMIDA.md
    ├── rabbitmqclient.md
    ├── inventoryconsumer.md
    ├── orderconsumer.md
    ├── relay.md
    └── services.md
```

## 🔐 Robustez aplicada

- Reserva de stock **atómica** (`UPDATE ... WHERE quantity_available >= qty`) para evitar overselling.
- Handler de órdenes con validación de UUID y `quantity > 0`.
- `order.Service` desacoplado de implementación concreta (sin type assertion a Postgres).
- Relay con `FOR UPDATE SKIP LOCKED` + orden por `created_at`.

## ✅ Tests relevantes

- `TestTryReserveStockConcurrent_NoOversell` (concurrencia real)
- `handler_test.go` (inputs inválidos + request válida)
- `TestRelayProcessOutbox_PublishesAndDeletes` (outbox → broker → delete)

## 🚀 Cómo correr

### Requisitos

- Docker + Docker Compose
- Go 1.26+

### 1) Infra

```bash
docker-compose up -d
```

### 2) Dependencias

```bash
go mod tidy
```

### 2.1) Schema reproducible (PR deploy)

- El servicio crea automáticamente tablas core al iniciar (`orders`, `inventory`, `outbox`, `processed_events`).
- También tenés el SQL base en `db/migrations/001_init.sql` por si querés inicializar manualmente en otro entorno.

### 2.2) Demo seed reproducible (PR portfolio)

Para dejar datos demo listos (un producto con stock y otro sin stock):

```bash
docker compose exec -T postgres psql -U user -d ecommerce_db < db/seeds/001_demo.sql
```

IDs demo que podés usar:

- `userId`: `11111111-1111-1111-1111-111111111111`
- `productId` con stock: `22222222-2222-2222-2222-222222222222`
- `productId` sin stock: `33333333-3333-3333-3333-333333333333`

### 3) Ejecutar API

```bash
go run main.go
```

### 3.1) Ejecutar demo web mínima (PR #14)

En otro terminal:

```bash
cd demo
python3 -m http.server 5500
```

Abrí: `http://localhost:5500`

En la UI, dejá `Backend base URL` en `http://localhost:8080` (o la URL donde tengas deployado tu backend).

### Variables de entorno soportadas

- `PORT` (default: `8080`)
- `DATABASE_URL` (default local de desarrollo)
- `RABBITMQ_URL` (default local de desarrollo)

### Endpoints base

- `GET /health` → health check simple (`200 {"status":"ok"}`)
- `GET /metrics` → métricas operativas básicas en JSON
- `GET /orders/:id` → consultar estado de orden
- `POST /orders` → crear orden

Server local por defecto: `http://localhost:8080`

### Quick demo (flujo saga)

Crear orden con stock (esperado: termina en `CREATED`):

```bash
curl -X POST http://localhost:8080/orders \
  -H "Content-Type: application/json" \
  -d '{"userId":"11111111-1111-1111-1111-111111111111","productId":"22222222-2222-2222-2222-222222222222","quantity":1}'
```

Crear orden sin stock (esperado: termina en `FAILED`):

```bash
curl -X POST http://localhost:8080/orders \
  -H "Content-Type: application/json" \
  -d '{"userId":"11111111-1111-1111-1111-111111111111","productId":"33333333-3333-3333-3333-333333333333","quantity":1}'
```

Consultar estado final de la orden:

```bash
curl http://localhost:8080/orders/<ORDER_ID>
```

## 🧪 Ejecutar tests

```bash
go test ./...
```

> Algunos tests de integración usan Testcontainers (requieren Docker disponible).

## 👤 Autor

**Santino Zarate**  
Proyecto de aprendizaje profesional con foco en backend distribuido y buenas prácticas de producción.
