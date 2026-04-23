package inventory

import (
	"context"
	"e-commerce/pkg/events"
	"e-commerce/pkg/logx"
	"e-commerce/pkg/metrics"
	"e-commerce/pkg/rabbitmq"
	"encoding/json"
	"fmt"
	"time"
)

const reserveStockTimeout = 5 * time.Second

// Consumer se encarga de escuchar eventos de RabbitMQ.
type Consumer struct {
	client  *rabbitmq.Client
	service *Service
}

// NewConsumer crea una nueva instancia de Consumer.
func NewConsumer(rc *rabbitmq.Client, s *Service) *Consumer {
	return &Consumer{client: rc, service: s}
}

// StartListening inicia el proceso de consumo de eventos con re-suscripción automática.
func (c *Consumer) StartListening(ctx context.Context) {
	attempt := 0

	for {
		select {
		case <-ctx.Done():
			logx.Info("inventory consumer stopped by context", map[string]interface{}{
				"component": "inventory-consumer",
			})
			return
		default:
		}

		deliveries, err := c.client.Consume(rabbitmq.InventoryQueue, "inventory-consumer")
		if err != nil {
			metrics.Inc("inventory_consumer_subscribe_error_total")
			attempt++
			delay := backoffDelay(attempt)
			logx.Error("inventory consumer subscribe error", err, map[string]interface{}{
				"component": "inventory-consumer",
				"attempt":   attempt,
				"retry_in":  delay.String(),
			})
			select {
			case <-ctx.Done():
				return
			case <-time.After(delay):
				continue
			}
		}

		logx.Info("inventory consumer subscribed", map[string]interface{}{
			"component": "inventory-consumer",
			"queue":     rabbitmq.InventoryQueue,
		})
		attempt = 0

		streamClosed := false
		for !streamClosed {
			select {
			case <-ctx.Done():
				logx.Info("inventory consumer stopping", map[string]interface{}{
					"component": "inventory-consumer",
				})
				return
			case d, ok := <-deliveries:
				if !ok {
					metrics.Inc("inventory_consumer_delivery_channel_closed_total")
					logx.Info("inventory consumer delivery channel closed, re-subscribing", map[string]interface{}{
						"component": "inventory-consumer",
					})
					streamClosed = true
					continue
				}

				if d.RoutingKey != rabbitmq.OrderCreatedKey {
					metrics.Inc("inventory_consumer_invalid_routing_total")
					logx.Error("unexpected routing key on inventory consumer", nil, map[string]interface{}{
						"component":   "inventory-consumer",
						"routing_key": d.RoutingKey,
					})
					_ = d.Nack(false, false)
					continue
				}

				var event events.OrderCreated
				if err := json.Unmarshal(d.Body, &event); err != nil {
					metrics.Inc("inventory_consumer_unmarshal_error_total")
					logx.Error("failed to unmarshal order.created", err, map[string]interface{}{
						"component":   "inventory-consumer",
						"routing_key": d.RoutingKey,
					})
					_ = d.Nack(false, false)
					continue
				}

				headers := map[string]interface{}{}
				if correlationID, ok := d.Headers["correlation_id"]; ok {
					headers["correlation_id"] = fmt.Sprint(correlationID)
				} else {
					// Fallback para mantener idempotencia incluso si el header no llega.
					headers["correlation_id"] = fmt.Sprintf("order-created:%s", event.OrderID.String())
				}
				correlationID := fmt.Sprint(headers["correlation_id"])

				logx.Info("received order.created", map[string]interface{}{
					"component":      "inventory-consumer",
					"order_id":       event.OrderID,
					"correlation_id": correlationID,
					"routing_key":    d.RoutingKey,
				})
				metrics.Inc("inventory_consumer_messages_total")

				opCtx, cancel := context.WithTimeout(ctx, reserveStockTimeout)
				// Llamar a la lógica de negocio
				if err := c.service.ReserveStock(opCtx, event, headers); err != nil {
					cancel()
					metrics.Inc("inventory_consumer_process_error_total")
					logx.Error("failed to reserve stock", err, map[string]interface{}{
						"component":      "inventory-consumer",
						"order_id":       event.OrderID,
						"correlation_id": correlationID,
					})
					_ = d.Nack(false, true)
					metrics.Inc("inventory_consumer_nack_total")
					continue
				}
				cancel()

				if err := d.Ack(false); err != nil {
					metrics.Inc("inventory_consumer_ack_error_total")
					logx.Error("failed to ack message", err, map[string]interface{}{
						"component":      "inventory-consumer",
						"order_id":       event.OrderID,
						"correlation_id": correlationID,
					})
				}
				metrics.Inc("inventory_consumer_ack_total")
			}
		}
	}
}

func backoffDelay(attempt int) time.Duration {
	switch {
	case attempt <= 1:
		return 1 * time.Second
	case attempt == 2:
		return 2 * time.Second
	case attempt == 3:
		return 3 * time.Second
	default:
		return 5 * time.Second
	}
}
