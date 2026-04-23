package order

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

const applyInventoryResultTimeout = 5 * time.Second

// Consumer se encarga de escuchar eventos de RabbitMQ para el servicio de órdenes.
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
			logx.Info("order consumer stopped by context", map[string]interface{}{
				"component": "order-consumer",
			})
			return
		default:
		}

		deliveries, err := c.client.Consume(rabbitmq.OrderQueue, "order-consumer")
		if err != nil {
			metrics.Inc("order_consumer_subscribe_error_total")
			attempt++
			delay := backoffDelay(attempt)
			logx.Error("order consumer subscribe error", err, map[string]interface{}{
				"component": "order-consumer",
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

		logx.Info("order consumer subscribed", map[string]interface{}{
			"component": "order-consumer",
			"queue":     rabbitmq.OrderQueue,
		})
		attempt = 0

		streamClosed := false
		for !streamClosed {
			select {
			case <-ctx.Done():
				logx.Info("order consumer stopping", map[string]interface{}{
					"component": "order-consumer",
				})
				return
			case d, ok := <-deliveries:
				if !ok {
					metrics.Inc("order_consumer_delivery_channel_closed_total")
					logx.Info("order consumer delivery channel closed, re-subscribing", map[string]interface{}{
						"component": "order-consumer",
					})
					streamClosed = true
					continue
				}

				switch d.RoutingKey {
				case rabbitmq.InventoryReservedKey:
					var reserved events.InventoryReserved
					if err := json.Unmarshal(d.Body, &reserved); err != nil {
						metrics.Inc("order_consumer_unmarshal_error_total")
						logx.Error("failed to unmarshal inventory.reserved", err, map[string]interface{}{
							"component":   "order-consumer",
							"routing_key": d.RoutingKey,
						})
						_ = d.Nack(false, false)
						metrics.Inc("order_consumer_nack_total")
						continue
					}
					metrics.Inc("order_consumer_messages_total")

					eventID := fmt.Sprint(d.Headers["correlation_id"])
					if eventID == "" || eventID == "<nil>" {
						eventID = fmt.Sprintf("inventory-reserved:%s", reserved.OrderID.String())
					}
					logx.Info("received inventory.reserved", map[string]interface{}{
						"component":      "order-consumer",
						"order_id":       reserved.OrderID,
						"correlation_id": eventID,
						"routing_key":    d.RoutingKey,
					})

					opCtx, cancel := context.WithTimeout(ctx, applyInventoryResultTimeout)
					processed, err := c.service.ApplyInventoryResultOnce(opCtx, eventID, reserved.OrderID.String(), StatusCreated)
					cancel()
					if err != nil {
						metrics.Inc("order_consumer_process_error_total")
						logx.Error("failed to update order status to CREATED", err, map[string]interface{}{
							"component":      "order-consumer",
							"order_id":       reserved.OrderID,
							"correlation_id": eventID,
						})
						_ = d.Nack(false, true)
						metrics.Inc("order_consumer_nack_total")
						continue
					}
					if !processed {
						metrics.Inc("order_consumer_duplicates_total")
						logx.Info("duplicate inventory.reserved ignored", map[string]interface{}{
							"component":      "order-consumer",
							"order_id":       reserved.OrderID,
							"correlation_id": eventID,
						})
					}

				case rabbitmq.InventoryFailedKey:
					var failed events.StockInsufficient
					if err := json.Unmarshal(d.Body, &failed); err != nil {
						metrics.Inc("order_consumer_unmarshal_error_total")
						logx.Error("failed to unmarshal inventory.failed", err, map[string]interface{}{
							"component":   "order-consumer",
							"routing_key": d.RoutingKey,
						})
						_ = d.Nack(false, false)
						metrics.Inc("order_consumer_nack_total")
						continue
					}
					metrics.Inc("order_consumer_messages_total")

					eventID := fmt.Sprint(d.Headers["correlation_id"])
					if eventID == "" || eventID == "<nil>" {
						eventID = fmt.Sprintf("inventory-failed:%s", failed.OrderID.String())
					}
					logx.Info("received inventory.failed", map[string]interface{}{
						"component":      "order-consumer",
						"order_id":       failed.OrderID,
						"correlation_id": eventID,
						"routing_key":    d.RoutingKey,
					})

					opCtx, cancel := context.WithTimeout(ctx, applyInventoryResultTimeout)
					processed, err := c.service.ApplyInventoryResultOnce(opCtx, eventID, failed.OrderID.String(), StatusFailed)
					cancel()
					if err != nil {
						metrics.Inc("order_consumer_process_error_total")
						logx.Error("failed to update order status to FAILED", err, map[string]interface{}{
							"component":      "order-consumer",
							"order_id":       failed.OrderID,
							"correlation_id": eventID,
						})
						_ = d.Nack(false, true)
						metrics.Inc("order_consumer_nack_total")
						continue
					}
					if !processed {
						metrics.Inc("order_consumer_duplicates_total")
						logx.Info("duplicate inventory.failed ignored", map[string]interface{}{
							"component":      "order-consumer",
							"order_id":       failed.OrderID,
							"correlation_id": eventID,
						})
					}

				default:
					metrics.Inc("order_consumer_invalid_routing_total")
					logx.Error("unexpected routing key on order consumer", nil, map[string]interface{}{
						"component":   "order-consumer",
						"routing_key": d.RoutingKey,
					})
					_ = d.Nack(false, false)
					metrics.Inc("order_consumer_nack_total")
					continue
				}

				if err := d.Ack(false); err != nil {
					metrics.Inc("order_consumer_ack_error_total")
					logx.Error("failed to ack message", err, map[string]interface{}{
						"component": "order-consumer",
					})
				}
				metrics.Inc("order_consumer_ack_total")
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
