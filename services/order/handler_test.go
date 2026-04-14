package order

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestCreateOrder_InvalidJSON(t *testing.T) {
	e := echo.New()
	mockRepo := new(MockRepository)
	service := NewService(mockRepo, nil)
	handler := NewHandler(service)

	req := httptest.NewRequest(http.MethodPost, "/orders", bytes.NewBufferString("{invalid-json"))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	err := handler.CreateOrder(c)
	assert.NoError(t, err)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestCreateOrder_InvalidUserID(t *testing.T) {
	e := echo.New()
	mockRepo := new(MockRepository)
	service := NewService(mockRepo, nil)
	handler := NewHandler(service)

	body := `{"userId":"not-a-uuid","productId":"660e8400-e29b-41d4-a716-446655440000","quantity":1}`
	req := httptest.NewRequest(http.MethodPost, "/orders", bytes.NewBufferString(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	err := handler.CreateOrder(c)
	assert.NoError(t, err)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestCreateOrder_InvalidProductID(t *testing.T) {
	e := echo.New()
	mockRepo := new(MockRepository)
	service := NewService(mockRepo, nil)
	handler := NewHandler(service)

	body := `{"userId":"550e8400-e29b-41d4-a716-446655440000","productId":"bad-uuid","quantity":1}`
	req := httptest.NewRequest(http.MethodPost, "/orders", bytes.NewBufferString(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	err := handler.CreateOrder(c)
	assert.NoError(t, err)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestCreateOrder_InvalidQuantity(t *testing.T) {
	e := echo.New()
	mockRepo := new(MockRepository)
	service := NewService(mockRepo, nil)
	handler := NewHandler(service)

	body := `{"userId":"550e8400-e29b-41d4-a716-446655440000","productId":"660e8400-e29b-41d4-a716-446655440000","quantity":0}`
	req := httptest.NewRequest(http.MethodPost, "/orders", bytes.NewBufferString(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	err := handler.CreateOrder(c)
	assert.NoError(t, err)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestCreateOrder_ValidRequest(t *testing.T) {
	e := echo.New()
	mockRepo := new(MockRepository)
	service := NewService(mockRepo, nil)
	handler := NewHandler(service)

	body := `{"userId":"550e8400-e29b-41d4-a716-446655440000","productId":"660e8400-e29b-41d4-a716-446655440000","quantity":2}`
	req := httptest.NewRequest(http.MethodPost, "/orders", bytes.NewBufferString(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	mockRepo.
		On("SaveTransactional", mock.Anything, mock.AnythingOfType("*order.Order"), "order.created", mock.Anything).
		Return(nil).
		Once()

	err := handler.CreateOrder(c)
	assert.NoError(t, err)
	assert.Equal(t, http.StatusCreated, rec.Code)

	var got Order
	decodeErr := json.Unmarshal(rec.Body.Bytes(), &got)
	assert.NoError(t, decodeErr)
	assert.NotEmpty(t, got.ID)
	assert.Equal(t, StatusPending, got.Status)
	assert.Equal(t, 2, got.Quantity)

	mockRepo.AssertExpectations(t)
}
