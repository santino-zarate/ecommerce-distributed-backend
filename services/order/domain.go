package order

import "errors"

type OrderStatus string

const (
	StatusPending OrderStatus = "PENDING"
	StatusCreated OrderStatus = "CREATED"
	StatusFailed  OrderStatus = "FAILED"
)

var ErrOrderNotFound = errors.New("order not found")

type Order struct {
	ID        string
	UserID    string
	ProductID string
	Quantity  int
	Status    OrderStatus
}
