package order

import (
	"context"
	"github.com/stretchr/testify/mock"
)

type MockRepository struct {
	mock.Mock
}

func (m *MockRepository) Save(ctx context.Context, o *Order) error {
	args := m.Called(ctx, o)
	return args.Error(0)
}

func (m *MockRepository) GetByID(ctx context.Context, id string) (*Order, error) {
	args := m.Called(ctx, id)
	return args.Get(0).(*Order), args.Error(1)
}

func (m *MockRepository) UpdateStatus(ctx context.Context, id string, status OrderStatus) error {
	args := m.Called(ctx, id, status)
	return args.Error(0)
}

func (m *MockRepository) SaveTransactional(ctx context.Context, o *Order, eventType string, eventPayload interface{}) error {
	args := m.Called(ctx, o, eventType, eventPayload)
	return args.Error(0)
}
