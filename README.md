# E-commerce Event-Driven Microservices

Este es un proyecto de e-commerce robusto, diseñado con arquitectura hexagonal, comunicaciones asíncronas vía RabbitMQ y patrones SAGA para transacciones distribuidas.

## 🚀 Arquitectura
- **Backend:** Go (Golang).
- **Comunicación:** Event-Driven (RabbitMQ).
- **Consistencia:** Patrón SAGA para transacciones distribuidas (Orquestación en `OrderService`).
- **Persistencia:** Postgres (vía `pgx/v5`).
- **Transporte:** API REST (Echo).

## 🏗️ Estructura del Proyecto
- `services/`: Contiene los dominios (`order`, `inventory`, `product`, `user`) siguiendo Clean Architecture.
- `pkg/`: Librerías compartidas (`events` para contratos, `rabbitmq` para el cliente centralizado).

## 🛠️ Cómo correr el proyecto
1. `docker-compose up -d` para levantar infraestructura.
2. `go mod tidy` para bajar dependencias.
3. `go run main.go` para levantar el servidor.

---
*Hecho por [Santino Zarate] - Proyecto de aprendizaje distribuido.*
