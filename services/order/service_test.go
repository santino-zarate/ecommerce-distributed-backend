package order

import (
	"context"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"testing"
)

func TestCreateOrder(t *testing.T) {
	// Setup
	mockRepo := new(MockRepository)
	// We pass nil for rabbitClient since we aren't testing event publishing here
	service := NewService(mockRepo, nil)

	order := &Order{
		UserID:    uuid.New().String(), // UUID real
		ProductID: uuid.New().String(), // UUID real
		Quantity:  2,
	}

	// Expectation
	mockRepo.On("Save", mock.Anything, mock.AnythingOfType("*order.Order")).Return(nil)

	// Action
	err := service.CreateOrder(context.Background(), order)

	// Assert
	assert.NoError(t, err)
	assert.NotEmpty(t, order.ID)
	assert.Equal(t, StatusPending, order.Status)
	mockRepo.AssertExpectations(t)
}
