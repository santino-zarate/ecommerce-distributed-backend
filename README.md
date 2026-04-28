# E-commerce Event-Driven Backend

A production-oriented Go backend that models real-world e-commerce checkout challenges: reliable async processing, safe stock reservation under concurrency, duplicate message handling, and consistency across database and message broker boundaries.

This is not a simple CRUD project. It implements an order + inventory workflow using PostgreSQL, RabbitMQ, the SAGA pattern, and the Outbox Pattern to demonstrate backend design decisions that matter in distributed systems.

## What is this project?

This project simulates the backend of an e-commerce checkout flow where orders are created, inventory is reserved asynchronously, and the final order status is updated based on stock availability.

It focuses on the kind of backend problems that appear in real systems: reliable messaging, consistency between services, duplicate message handling, and safe concurrent stock updates.

## Key Highlights

- Event-driven backend architecture built with Go, PostgreSQL, and RabbitMQ.
- SAGA-style order workflow coordinating Order and Inventory domains asynchronously.
- Outbox Pattern to avoid losing events between database writes and message publishing.
- Idempotent consumers designed for at-least-once message delivery.
- Atomic stock reservation that protects inventory from concurrent overselling scenarios.
- Asynchronous order processing that avoids blocking the HTTP request while the SAGA completes.
- Reliability-focused flow with manual Ack/Nack, startup retries, graceful shutdown, and structured logs.
- Real deployment with API, PostgreSQL, RabbitMQ, and demo frontend running on hosted services.

## Live Demo

- **Web demo:** https://ecommerce-demo-d0vt.onrender.com
- **Backend health:** https://ecommerce-api-u14x.onrender.com/health
- **Backend root:** https://ecommerce-api-u14x.onrender.com

Quick test flow:

1. Open the web demo.
2. Click **Create order with stock**. The expected final status is `CREATED`.
3. Click **Create order without stock**. The expected final status is `FAILED`.

The demo is connected to the deployed backend, so you do not need to run anything locally to see the flow working.

## What This Project Demonstrates

This project demonstrates practical backend engineering skills:

- Designing transactional boundaries between PostgreSQL and RabbitMQ.
- Coordinating asynchronous domain workflows with eventual consistency.
- Building consumers that tolerate duplicate messages and partial failures.
- Protecting shared resources with concurrency-safe database operations.
- Exposing operational signals through structured logs, health checks, and metrics.

## Architecture

### High-Level Flow

```mermaid
flowchart LR
    User[User] --> API[Go API]
    API --> PostgreSQL[(PostgreSQL)]
    PostgreSQL --> Outbox[Outbox Table]
    Outbox --> RabbitMQ1[RabbitMQ]
    RabbitMQ1 --> Inventory[Inventory Consumer]
    Inventory --> RabbitMQ2[RabbitMQ]
    RabbitMQ2 --> Order[Order Consumer]
    Order --> DB[(PostgreSQL)]
```

In simple terms:

- The API receives an order.
- The order is stored in PostgreSQL together with an event in the `outbox` table.
- A relay reads pending outbox events and publishes them to RabbitMQ.
- Inventory tries to reserve stock.
- Inventory publishes either a success or failure event.
- Order consumes that response and updates the final order status.

This allows the system to process the workflow asynchronously instead of depending on one long HTTP request.

## How It Works

1. `POST /orders` receives `userId`, `productId`, and `quantity`.
2. Order creates the order in its initial state and stores an `order.created` event in `outbox` within the same database transaction.
3. The outbox relay reads pending events and publishes them to RabbitMQ.
4. `InventoryConsumer` consumes `order.created`.
5. Inventory tries to reserve stock using an atomic PostgreSQL operation.
6. If stock is available, Inventory publishes `inventory.reserved`; otherwise, it publishes `inventory.failed`.
7. `OrderConsumer` consumes the response and updates the order to `CREATED` or `FAILED`.
8. The demo polls `GET /orders/:id` until it receives the final status.

## Key Technical Decisions

### Why RabbitMQ Instead of Kafka?

RabbitMQ fits this project well because the workflow needs command/event messaging with simple routing, manual Ack/Nack, and clearly defined queues.

Kafka would also be valid for event streaming, audit logs, or very high-volume scenarios. For this use case, however, it would add unnecessary operational complexity.

### Why the Outbox Pattern?

Creating an order in the database and publishing an event to RabbitMQ are two separate operations. If the database commit succeeds but publishing fails, the order exists but stock reservation is never processed.

The outbox reduces that risk by storing the order and the event in the same transaction. A relay later publishes pending events to RabbitMQ.

### Why Idempotency?

RabbitMQ can deliver the same message more than once. This is normal in at-least-once messaging systems.

Because of that, consumers cannot assume that each message is processed only once. The system stores processed results so repeated events do not corrupt state or reserve stock twice.

### Why Not Separate Microservices Yet?

The project has separated domains such as `order` and `inventory`, but it currently runs as a single Go backend.

This is intentional. It demonstrates SAGA, Outbox, messaging, and domain separation without adding the deployment complexity of multiple services too early. The logical separation already exists, and splitting the domains into separate processes would be a natural next step.

## Problems Solved

### Overselling

Problem: two concurrent orders could try to buy the same stock.

Solution: stock is reserved with an atomic PostgreSQL update that only succeeds when enough stock is available. If there is not enough stock, the update does not apply.

### Duplicate Messages

Problem: in real messaging systems, a consumer can receive the same event more than once.

Solution: consumers use idempotency by event/consumer and persist processed results.

### Publish Failures

Problem: saving data in PostgreSQL and publishing to RabbitMQ is not a distributed atomic operation.

Solution: the Outbox Pattern persists the event in the database first. The relay then publishes it to RabbitMQ.

### Eventual Consistency

Problem: the order cannot immediately know whether Inventory reserved stock because the process is asynchronous.

Solution: the order starts in a pending state and is later updated to `CREATED` or `FAILED` when the Inventory response arrives.

## API Endpoints

### `POST /orders`

Creates an order and starts the SAGA flow.

```bash
curl -X POST https://ecommerce-api-u14x.onrender.com/orders \
  -H "Content-Type: application/json" \
  -d '{"userId":"11111111-1111-1111-1111-111111111111","productId":"22222222-2222-2222-2222-222222222222","quantity":1}'
```

### `GET /orders/:id`

Returns the current status of an order.

```bash
curl https://ecommerce-api-u14x.onrender.com/orders/<ORDER_ID>
```

> Replace `<ORDER_ID>` with the real order ID, without the `< >` symbols.

### `GET /health`

Simple backend health check.

```bash
curl https://ecommerce-api-u14x.onrender.com/health
```

### `GET /metrics`

Exposes basic operational metrics in JSON.

```bash
curl https://ecommerce-api-u14x.onrender.com/metrics
```

## Running Locally

Requirements:

- Go 1.26+
- Docker + Docker Compose

### 1. Start Infrastructure

```bash
docker compose up -d
```

This starts PostgreSQL and RabbitMQ locally.

### 2. Run the Backend

```bash
go run main.go
```

Default configuration:

```text
PORT=8080
DATABASE_URL=postgres://user:password@localhost:5432/ecommerce_db
RABBITMQ_URL=amqp://guest:guest@localhost:5672/
```

### 3. Load Demo Data

```bash
docker compose exec -T postgres psql -U user -d ecommerce_db < db/seeds/001_demo.sql
```

Demo data:

```text
userId:                  11111111-1111-1111-1111-111111111111
productId with stock:    22222222-2222-2222-2222-222222222222
productId without stock: 33333333-3333-3333-3333-333333333333
```

Optional: run the web demo locally.

```bash
cd demo
python3 -m http.server 5500
```

Open:

```text
http://localhost:5500
```

## Deployment

The project is deployed using real services:

- **Go API:** Render Web Service
- **PostgreSQL:** Render PostgreSQL
- **RabbitMQ:** CloudAMQP
- **Web demo:** Render Static Site

Environment variables used by the API:

```text
PORT=10000
DATABASE_URL=<Render PostgreSQL internal URL>
RABBITMQ_URL=<CloudAMQP AMQPS URL>
```

The deployed API does not depend on `localhost`. Infrastructure connections are configured through environment variables.

## Tradeoffs & Future Improvements

Intentional tradeoffs:

- There is no full e-commerce UI. The demo exists to test the backend flow, not to simulate a complete store.
- There is no authentication. The focus is consistency, messaging, and asynchronous processing.
- The system is not split into physical microservices yet. The current separation is domain-based inside a single Go backend to keep the project understandable and deployable.
- The health check is simple and does not deeply validate PostgreSQL or RabbitMQ.

Possible improvements:

- Add Dead Letter Queues for messages that fail repeatedly.
- Add a more advanced retry policy for the outbox with persisted attempts and backoff.
- Add a `/ready` endpoint that validates PostgreSQL and RabbitMQ.
- Add distributed tracing to follow an order end to end.
- Add authentication and real order ownership.
- Split Order and Inventory into independent processes if the project evolves.

## Tests

```bash
go test ./...
```

Relevant tests include:

- Inventory concurrency tests to prevent overselling.
- Order handler tests for invalid inputs.
- Relay/outbox tests for publishing events and removing pending records.
- Consumer idempotency tests.

## Author

**Santino Zarate**

Backend developer focused on Go, event-driven architecture, distributed consistency, and production-oriented system design.

This portfolio project highlights practical backend engineering skills: asynchronous workflows, reliable messaging, transactional boundaries, idempotent processing, and clear technical tradeoffs that can be defended in a professional engineering interview.

## Explore the Project

Try the live demo to see the full async order flow in action, then explore the codebase to review how the SAGA workflow, outbox relay, idempotent consumers, and concurrency-safe inventory logic are implemented.
