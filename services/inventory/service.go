package inventory

import (
	"context"
	"e-commerce/pkg/events"
	"e-commerce/pkg/rabbitmq"
	"encoding/json"
	"fmt"
	"github.com/google/uuid"
)

// Service implementa la lógica de negocio para el inventario.
type Service struct {
	repo         Repository
	rabbitClient *rabbitmq.Client
}

// NewService crea una nueva instancia de Service.
func NewService(r Repository, rc *rabbitmq.Client) *Service {
	return &Service{repo: r, rabbitClient: rc}
}

// ReserveStock valida y reserva stock.
func (s *Service) ReserveStock(ctx context.Context, event events.OrderCreated, headers map[string]interface{}) error {
	if len(event.Items) == 0 {
		return fmt.Errorf("order.created event without items")
	}

	eventID := fmt.Sprint(headers["correlation_id"])
	if eventID == "" || eventID == "<nil>" {
		eventID = fmt.Sprintf("order-created:%s", event.OrderID.String())
		headers["correlation_id"] = eventID
	}

	item := event.Items[0]
	productID := item.ProductID.String()
	orderID := event.OrderID.String()

	// 1. Reservar stock de forma atómica e idempotente en DB
	result, duplicate, err := s.repo.ReserveStockOnce(ctx, eventID, orderID, productID, item.Quantity)
	if err != nil {
		return err
	}

	// 2. Sin stock suficiente => publicar evento de fallo
	if result == ReserveInsufficient {
		// Publicar evento de error
		errorEvent := events.StockInsufficient{
			OrderID:   event.OrderID,
			ProductID: item.ProductID,
			Reason:    "insufficient stock",
		}
		body, _ := json.Marshal(errorEvent)
		return s.rabbitClient.Publish(rabbitmq.OrdersExchange, rabbitmq.InventoryFailedKey, body, headers)
	}

	if duplicate {
		// Duplicado con resultado previo APPLIED: re-publicamos la confirmación
		// para tolerar fallos transitorios de publish del intento anterior.
	}

	// 3. Publicar evento de éxito
	successEvent := events.InventoryReserved{
		OrderID:                event.OrderID,
		InventoryReservationID: uuid.New(),
	}
	body, _ := json.Marshal(successEvent)
	return s.rabbitClient.Publish(rabbitmq.OrdersExchange, rabbitmq.InventoryReservedKey, body, headers)
}
