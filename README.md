# E-commerce Event-Driven Backend

Backend de e-commerce en Go con procesamiento asíncrono real usando PostgreSQL, RabbitMQ, SAGA y Outbox Pattern.

No es un CRUD simple: el proyecto modela un flujo de orden + reserva de stock con consistencia eventual, idempotencia y protección contra overselling.

## Live Demo

- **Demo web:** https://ecommerce-demo-d0vt.onrender.com
- **Backend health:** https://ecommerce-api-u14x.onrender.com/health
- **Backend root:** https://ecommerce-api-u14x.onrender.com

Cómo probarlo rápido:

1. Abrí la demo web.
2. Click en **Crear orden con stock** → el estado final esperado es `CREATED`.
3. Click en **Crear orden sin stock** → el estado final esperado es `FAILED`.

La demo está conectada al backend deployado. No necesitás correr nada localmente para ver el flujo funcionando.

## What this project demonstrates

Este proyecto demuestra decisiones y problemas reales de backend:

- **Event-driven architecture** con RabbitMQ.
- **SAGA pattern** para coordinar el flujo Order → Inventory → Order.
- **Outbox Pattern** para no perder eventos entre PostgreSQL y RabbitMQ.
- **Idempotency** para tolerar mensajes duplicados.
- **Concurrency-safe stock reservation** para evitar overselling.
- **Async processing** con consumers, routing keys y Ack/Nack manual.
- **Graceful shutdown**, timeouts y retry de startup.
- **Logs estructurados** con `correlation_id`.
- **Métricas básicas** expuestas vía `/metrics`.

## Architecture

Flujo principal:

```text
User
  ↓
Demo Web
  ↓
Go API
  ↓
PostgreSQL: orders + outbox
  ↓
Outbox Relay
  ↓
RabbitMQ
  ↓
Inventory Consumer
  ↓
RabbitMQ
  ↓
Order Consumer
  ↓
PostgreSQL: order status updated
```

En simple:

- La API recibe una orden.
- La orden se guarda en PostgreSQL junto con un evento en la tabla `outbox`.
- Un relay toma eventos pendientes y los publica en RabbitMQ.
- Inventory intenta reservar stock.
- Inventory responde con éxito o fallo.
- Order consume esa respuesta y actualiza el estado final de la orden.

Esto permite que el sistema procese pasos de forma asíncrona sin depender de una única request HTTP larga.

## How it works

1. `POST /orders` recibe `userId`, `productId` y `quantity`.
2. Order crea la orden en estado inicial y guarda un evento `order.created` en `outbox` dentro de la misma transacción.
3. El relay lee la outbox y publica el evento en RabbitMQ.
4. `InventoryConsumer` consume `order.created`.
5. Inventory intenta reservar stock con una operación atómica en PostgreSQL.
6. Si hay stock, publica `inventory.reserved`; si no hay stock, publica `inventory.failed`.
7. `OrderConsumer` consume la respuesta y actualiza la orden a `CREATED` o `FAILED`.
8. La demo consulta `GET /orders/:id` hasta ver el estado final.

## Key technical decisions

### Why RabbitMQ instead of Kafka?

RabbitMQ encaja mejor para este proyecto porque el flujo necesita mensajería de comandos/eventos con routing simple, Ack/Nack manual y colas bien definidas.

Kafka sería válido para event streaming, auditoría o alto volumen, pero para este caso agregaría complejidad operativa innecesaria.

### Why Outbox Pattern?

Crear la orden en DB y publicar un evento en RabbitMQ son dos operaciones distintas. Si la DB confirma pero el publish falla, la orden queda creada pero nadie procesa el stock.

La outbox reduce ese riesgo: la orden y el evento se guardan juntos en la misma transacción. Después, un relay se encarga de publicar lo pendiente.

### Why idempotency?

RabbitMQ puede entregar mensajes más de una vez. Eso es normal en sistemas at-least-once.

Por eso los consumers no pueden asumir “este mensaje llega una sola vez”. El sistema persiste resultados procesados para que repetir un evento no rompa el estado ni reserve stock dos veces.

### Why not separate microservices yet?

El proyecto usa dominios separados (`order`, `inventory`, etc.) pero corre como un backend Go único.

Esto es intencional: permite demostrar SAGA, Outbox y mensajería sin sumar la complejidad de deployar múltiples servicios desde el primer día. La separación lógica ya existe; separar procesos sería un paso posterior.

## Problems solved

### Overselling

Problema: dos órdenes concurrentes podrían comprar el mismo stock.

Solución: reserva atómica en PostgreSQL usando una condición de stock disponible. Si no hay stock suficiente, la actualización no aplica.

### Duplicate messages

Problema: en mensajería real, un consumer puede recibir el mismo evento más de una vez.

Solución: idempotencia por evento/consumer y persistencia del resultado procesado.

### Publish failures

Problema: guardar en DB y publicar en RabbitMQ no es una operación atómica distribuida.

Solución: Outbox Pattern. Primero se persiste el evento en DB; luego el relay publica a RabbitMQ.

### Eventual consistency

Problema: la orden no puede saber inmediatamente si inventory reservó stock porque el proceso es asíncrono.

Solución: la orden empieza en estado pendiente y luego se actualiza a `CREATED` o `FAILED` cuando llega la respuesta de Inventory.

## API Endpoints

### `POST /orders`

Crea una orden y dispara el flujo SAGA.

```bash
curl -X POST https://ecommerce-api-u14x.onrender.com/orders \
  -H "Content-Type: application/json" \
  -d '{"userId":"11111111-1111-1111-1111-111111111111","productId":"22222222-2222-2222-2222-222222222222","quantity":1}'
```

### `GET /orders/:id`

Consulta el estado actual de una orden.

```bash
curl https://ecommerce-api-u14x.onrender.com/orders/<ORDER_ID>
```

> Reemplazá `<ORDER_ID>` por el ID real, sin los signos `< >`.

### `GET /health`

Health check simple del backend.

```bash
curl https://ecommerce-api-u14x.onrender.com/health
```

### `GET /metrics`

Expone métricas operativas básicas en JSON.

```bash
curl https://ecommerce-api-u14x.onrender.com/metrics
```

## Running locally

Requisitos:

- Go 1.26+
- Docker + Docker Compose

### 1. Levantar infraestructura

```bash
docker compose up -d
```

Esto levanta PostgreSQL y RabbitMQ local.

### 2. Ejecutar backend

```bash
go run main.go
```

Por defecto usa:

```text
PORT=8080
DATABASE_URL=postgres://user:password@localhost:5432/ecommerce_db
RABBITMQ_URL=amqp://guest:guest@localhost:5672/
```

### 3. Cargar datos demo

```bash
docker compose exec -T postgres psql -U user -d ecommerce_db < db/seeds/001_demo.sql
```

Datos demo:

```text
userId:              11111111-1111-1111-1111-111111111111
productId con stock: 22222222-2222-2222-2222-222222222222
productId sin stock: 33333333-3333-3333-3333-333333333333
```

Opcional: correr la demo web local:

```bash
cd demo
python3 -m http.server 5500
```

Abrir:

```text
http://localhost:5500
```

## Deployment

El proyecto está deployado con servicios reales:

- **API Go:** Render Web Service
- **PostgreSQL:** Render PostgreSQL
- **RabbitMQ:** CloudAMQP
- **Demo web:** Render Static Site

Variables usadas por la API:

```text
PORT=10000
DATABASE_URL=<Render PostgreSQL internal URL>
RABBITMQ_URL=<CloudAMQP AMQPS URL>
```

La API no depende de `localhost` en deploy. Las conexiones a infraestructura se configuran por variables de entorno.

## Tradeoffs & Future Improvements

Tradeoffs intencionales:

- No hay UI completa de e-commerce. La demo existe para probar el flujo backend, no para simular una tienda real.
- No hay autenticación. El foco está en consistencia, mensajería y procesamiento asíncrono.
- No está separado en microservicios físicos todavía. La separación actual es por dominio dentro de un backend Go para mantener el proyecto entendible y deployable.
- El health check es simple; no valida DB/RabbitMQ en profundidad.

Mejoras posibles:

- Dead Letter Queues para mensajes que fallan repetidamente.
- Retry policy más avanzada para outbox con attempts/backoff persistidos.
- Endpoint `/ready` que valide PostgreSQL y RabbitMQ.
- Distributed tracing para seguir una orden de punta a punta.
- Autenticación y ownership real de órdenes.
- Separar Order e Inventory en procesos independientes si el proyecto evoluciona.

## Tests

```bash
go test ./...
```

Tests relevantes:

- Concurrencia de inventory para evitar overselling.
- Handler de orders con inputs inválidos.
- Relay/outbox publicando eventos y eliminando pendientes.
- Idempotencia en consumers.

## Author

**Santino Zarate**

Proyecto de portfolio backend enfocado en sistemas event-driven, consistencia distribuida y decisiones técnicas defendibles en entrevista.
