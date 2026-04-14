package order

import (
	"context"
	"e-commerce/pkg/events"
	"e-commerce/pkg/rabbitmq"
	"fmt"
	"github.com/google/uuid"
)

// Service implementa la lógica de negocio para órdenes.
type Service struct {
	repo Repository
}

// NewService crea una nueva instancia de Service inyectando el repo y el cliente de rabbit.
func NewService(r Repository, _ *rabbitmq.Client) *Service {
	return &Service{repo: r}
}

// CreateOrder crea una nueva orden, la persiste y publica un evento.
func (s *Service) CreateOrder(ctx context.Context, o *Order) error {
	if o.ID == "" {
		o.ID = uuid.New().String()
	}
	o.Status = StatusPending

	// 1. Persistir + Outbox (Transaccional)
	orderUUID, err := uuid.Parse(o.ID)
	if err != nil {
		return fmt.Errorf("invalid order id: %w", err)
	}
	userUUID, err := uuid.Parse(o.UserID)
	if err != nil {
		return fmt.Errorf("invalid user id: %w", err)
	}
	productUUID, err := uuid.Parse(o.ProductID)
	if err != nil {
		return fmt.Errorf("invalid product id: %w", err)
	}

	event := events.OrderCreated{
		OrderID: orderUUID,
		UserID:  userUUID,
		Items: []events.OrderItem{
			{ProductID: productUUID, Quantity: o.Quantity},
		},
		TotalAmount: 0.0, // Placeholder
	}

	return s.repo.SaveTransactional(ctx, o, "order.created", event)
}

// UpdateOrderStatus actualiza el estado de la orden basado en eventos.
func (s *Service) UpdateOrderStatus(ctx context.Context, id string, status OrderStatus) error {
	return s.repo.UpdateStatus(ctx, id, status)
}
