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

### 3.1) Ejecutar demo web mínima (PR #14/PR #16)

En otro terminal:

```bash
cd demo
python3 -m http.server 5500
```

Abrí: `http://localhost:5500`

En la UI, `Backend base URL` apunta por defecto al backend deployado:

```text
https://ecommerce-api-u14x.onrender.com
```

Para desarrollo local podés cambiarlo a:

```text
http://localhost:8080
```

### Variables de entorno soportadas

- `PORT` (default: `8080`)
- `DATABASE_URL` (default local de desarrollo)
- `RABBITMQ_URL` (default local de desarrollo)

## 🌐 Deploy backend en Render (PR #15)

El backend está preparado para deploy real usando variables de entorno. La idea es mantenerlo simple:

- PostgreSQL managed en Render.
- RabbitMQ managed en CloudAMQP.
- API Go como Web Service en Render.

### 1) Crear PostgreSQL

En Render:

```text
New → PostgreSQL
```

Guardá la **Internal Database URL**. Esa URL va en la API como:

```text
DATABASE_URL=<internal database url>
```

### 2) Crear RabbitMQ en CloudAMQP

Crear una instancia RabbitMQ en CloudAMQP, idealmente en una región cercana a la API y PostgreSQL.

Configuración usada para la demo:

```text
Name: ecommerce-rabbitmq
Region: us-west-2 / Oregon
```

CloudAMQP entrega una URL AMQP/AMQPS. Usá la variante TLS (`amqps://`) como variable de entorno de la API:

```text
RABBITMQ_URL=amqps://<user>:<password>@<host>.cloudamqp.com/<vhost>
```

> Importante: no usar `localhost` en deploy. Dentro del contenedor/servicio de la API, `localhost` es la API misma, no RabbitMQ ni PostgreSQL.

### 3) Crear Web Service para la API

En Render:

```text
New → Web Service → conectar repo GitHub
```

Configuración recomendada:

```text
Language: Go
Branch: main
Build Command: go build -tags netgo -ldflags '-s -w' -o app
Start Command: ./app
Health Check Path: /health
```

Variables de entorno:

```text
PORT=10000
DATABASE_URL=<internal database url de Render>
RABBITMQ_URL=<amqps url de CloudAMQP>
```

El código ya lee `PORT`, `DATABASE_URL` y `RABBITMQ_URL`, por eso no hay que cambiar lógica de negocio para deployar.

> También existe un `Dockerfile` para correr la API como imagen Docker si se quiere mover este deploy a otro proveedor o cambiar Render a runtime Docker más adelante.

### 4) Cargar datos demo

El backend crea tablas core al arrancar, pero los productos demo se cargan con:

```bash
db/seeds/001_demo.sql
```

Ejecutá ese SQL en la base de Render para poder probar el flujo con los IDs demo.

### 5) Probar API deployada

```bash
curl https://<tu-api>.onrender.com/
```

Health check:

```bash
curl https://<tu-api>.onrender.com/health
```

Crear orden con stock:

```bash
curl -X POST https://<tu-api>.onrender.com/orders \
  -H "Content-Type: application/json" \
  -d '{"userId":"11111111-1111-1111-1111-111111111111","productId":"22222222-2222-2222-2222-222222222222","quantity":1}'
```

Consultar estado:

```bash
curl https://<tu-api>.onrender.com/orders/<ORDER_ID>
```

### Qué significa este deploy

Esto no convierte el proyecto en producción enterprise. Sí demuestra algo importante para portfolio:

> La API es deployable, configurable por entorno y capaz de hablar con PostgreSQL/RabbitMQ reales fuera de tu máquina.

## 🖥️ Deploy demo web estática en Render (PR #16)

La demo web está en `demo/` y es HTML/CSS/JS puro. No necesita Node, bundler ni build.

En Render:

```text
New → Static Site → conectar repo GitHub
```

Configuración recomendada:

```text
Name: ecommerce-demo
Branch: main
Root Directory: demo
Build Command: dejar vacío
Publish Directory: .
```

La demo apunta por defecto a:

```text
https://ecommerce-api-u14x.onrender.com
```

Validación esperada:

1. Abrir la URL pública de la demo.
2. Click en **Crear orden con stock** → estado final `CREATED`.
3. Click en **Crear orden sin stock** → estado final `FAILED`.

> Si querés probar contra backend local, cambiá manualmente el input `Backend base URL` a `http://localhost:8080`.

### Endpoints base

- `GET /health` → health check simple (`200 {"status":"ok"}`)
- `GET /` → metadata mínima del backend deployado
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
