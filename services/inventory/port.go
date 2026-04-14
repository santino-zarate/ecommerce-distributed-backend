package inventory

import "context"

type Repository interface {
	GetByProductID(ctx context.Context, productID string) (*Inventory, error)
	Update(ctx context.Context, inventory *Inventory) error
	TryReserveStock(ctx context.Context, productID string, qty int) (bool, error)
}
