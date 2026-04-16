package order

import "context"

type Repository interface {
	Save(ctx context.Context, order *Order) error
	GetByID(ctx context.Context, id string) (*Order, error)
	UpdateStatus(ctx context.Context, id string, status OrderStatus) error
	SaveTransactional(ctx context.Context, order *Order, eventType string, eventPayload interface{}) error
	ApplyInventoryResultOnce(ctx context.Context, eventID, orderID string, targetStatus OrderStatus, consumer string) (bool, error)
}
