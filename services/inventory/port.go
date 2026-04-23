package inventory

import "context"

type ReserveResult int

const (
	ReserveApplied ReserveResult = iota
	ReserveInsufficient
	ReserveDuplicate
)

type Repository interface {
	GetByProductID(ctx context.Context, productID string) (*Inventory, error)
	Update(ctx context.Context, inventory *Inventory) error
	TryReserveStock(ctx context.Context, productID string, qty int) (bool, error)
	ReserveStockOnce(ctx context.Context, eventID, orderID, productID string, qty int) (ReserveResult, bool, error)
}
