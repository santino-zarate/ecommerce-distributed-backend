package inventory

import (
	"context"
	"e-commerce/pkg/events"
	"e-commerce/pkg/rabbitmq"
	"encoding/json"
	"fmt"
	"log"
)

// Consumer se encarga de escuchar eventos de RabbitMQ.
type Consumer struct {
	client  *rabbitmq.Client
	service *Service
}

// NewConsumer crea una nueva instancia de Consumer.
func NewConsumer(rc *rabbitmq.Client, s *Service) *Consumer {
	return &Consumer{client: rc, service: s}
}

// StartListening inicia el proceso de consumo de eventos.
func (c *Consumer) StartListening() {
	deliveries, err := c.client.Consume(rabbitmq.InventoryQueue, "inventory-consumer")
	if err != nil {
		log.Fatalf("failed to register consumer: %v", err)
	}

	for d := range deliveries {
		if d.RoutingKey != rabbitmq.OrderCreatedKey {
			log.Printf("unexpected routing key on inventory consumer: %s", d.RoutingKey)
			_ = d.Nack(false, false)
			continue
		}

		var event events.OrderCreated
		if err := json.Unmarshal(d.Body, &event); err != nil {
			log.Printf("failed to unmarshal event: %v", err)
			_ = d.Nack(false, false)
			continue
		}

		log.Printf("Received OrderCreated event: %s", event.OrderID)

		headers := map[string]interface{}{}
		if correlationID, ok := d.Headers["correlation_id"]; ok {
			headers["correlation_id"] = fmt.Sprint(correlationID)
		} else {
			// Fallback para mantener idempotencia incluso si el header no llega.
			headers["correlation_id"] = fmt.Sprintf("order-created:%s", event.OrderID.String())
		}

		// Llamar a la lógica de negocio
		if err := c.service.ReserveStock(context.Background(), event, headers); err != nil {
			log.Printf("failed to reserve stock: %v", err)
			_ = d.Nack(false, true)
			continue
		}

		if err := d.Ack(false); err != nil {
			log.Printf("failed to ack message: %v", err)
		}
	}
}
