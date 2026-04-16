package order

import (
	"context"
	"e-commerce/pkg/events"
	"e-commerce/pkg/rabbitmq"
	"encoding/json"
	"fmt"
	"log"
)

// Consumer se encarga de escuchar eventos de RabbitMQ para el servicio de órdenes.
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
	deliveries, err := c.client.Consume(rabbitmq.OrderQueue, "order-consumer")
	if err != nil {
		log.Fatalf("failed to register consumer: %v", err)
	}

	for d := range deliveries {
		switch d.RoutingKey {
		case rabbitmq.InventoryReservedKey:
			var reserved events.InventoryReserved
			if err := json.Unmarshal(d.Body, &reserved); err != nil {
				log.Printf("failed to unmarshal inventory.reserved: %v", err)
				_ = d.Nack(false, false)
				continue
			}

			log.Printf("Received InventoryReserved event: %s", reserved.OrderID)
			eventID := fmt.Sprint(d.Headers["correlation_id"])
			if eventID == "" || eventID == "<nil>" {
				eventID = fmt.Sprintf("inventory-reserved:%s", reserved.OrderID.String())
			}

			processed, err := c.service.ApplyInventoryResultOnce(context.Background(), eventID, reserved.OrderID.String(), StatusCreated)
			if err != nil {
				log.Printf("failed to update order status to CREATED: %v", err)
				_ = d.Nack(false, true)
				continue
			}
			if !processed {
				log.Printf("duplicate inventory.reserved ignored (event=%s)", eventID)
			}

		case rabbitmq.InventoryFailedKey:
			var failed events.StockInsufficient
			if err := json.Unmarshal(d.Body, &failed); err != nil {
				log.Printf("failed to unmarshal inventory.failed: %v", err)
				_ = d.Nack(false, false)
				continue
			}

			log.Printf("Received InventoryFailed event: %s", failed.OrderID)
			eventID := fmt.Sprint(d.Headers["correlation_id"])
			if eventID == "" || eventID == "<nil>" {
				eventID = fmt.Sprintf("inventory-failed:%s", failed.OrderID.String())
			}

			processed, err := c.service.ApplyInventoryResultOnce(context.Background(), eventID, failed.OrderID.String(), StatusFailed)
			if err != nil {
				log.Printf("failed to update order status to FAILED: %v", err)
				_ = d.Nack(false, true)
				continue
			}
			if !processed {
				log.Printf("duplicate inventory.failed ignored (event=%s)", eventID)
			}

		default:
			log.Printf("unexpected routing key on order consumer: %s", d.RoutingKey)
			_ = d.Nack(false, false)
			continue
		}

		if err := d.Ack(false); err != nil {
			log.Printf("failed to ack message: %v", err)
		}
	}
}
