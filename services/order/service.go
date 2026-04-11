package order

import (
	"context"
	"e-commerce/pkg/events"
	"e-commerce/pkg/rabbitmq"
	"github.com/google/uuid"
)

// Service implementa la lógica de negocio para órdenes.
type Service struct {
	repo         Repository
	rabbitClient *rabbitmq.Client
}

// NewService crea una nueva instancia de Service inyectando el repo y el cliente de rabbit.
func NewService(r Repository, rc *rabbitmq.Client) *Service {
	return &Service{repo: r, rabbitClient: rc}
}

// CreateOrder crea una nueva orden, la persiste y publica un evento.
func (s *Service) CreateOrder(ctx context.Context, o *Order) error {
	if o.ID == "" {
		o.ID = uuid.New().String()
	}
	o.Status = StatusPending

	// 1. Persistir + Outbox (Transaccional)
	event := events.OrderCreated{
		OrderID: uuid.MustParse(o.ID),
		UserID:  uuid.MustParse(o.UserID),
		Items: []events.OrderItem{
			{ProductID: uuid.MustParse(o.ProductID), Quantity: o.Quantity},
		},
		TotalAmount: 0.0, // Placeholder
	}

	return s.repo.(*PostgresRepository).SaveTransactional(ctx, o, "order.created", event)
}

// UpdateOrderStatus actualiza el estado de la orden basado en eventos.
func (s *Service) UpdateOrderStatus(ctx context.Context, id string, status OrderStatus) error {
	return s.repo.UpdateStatus(ctx, id, status)
}
