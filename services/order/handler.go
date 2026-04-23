package order

import (
	"e-commerce/pkg/metrics"
	"net/http"
	"strings"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
)

// Handler maneja las peticiones HTTP para el servicio de órdenes.
type Handler struct {
	service *Service
}

type CreateOrderRequest struct {
	UserID    string `json:"userId"`
	ProductID string `json:"productId"`
	Quantity  int    `json:"quantity"`
}

// NewHandler crea un nuevo handler para el servicio de órdenes.
func NewHandler(s *Service) *Handler {
	return &Handler{service: s}
}

// CreateOrder es el endpoint para crear una orden.
func (h *Handler) CreateOrder(c echo.Context) error {
	metrics.Inc("orders_create_requests_total")

	var req CreateOrderRequest
	if err := c.Bind(&req); err != nil {
		metrics.Inc("orders_create_bad_request_total")
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request"})
	}

	req.UserID = strings.TrimSpace(req.UserID)
	req.ProductID = strings.TrimSpace(req.ProductID)

	if req.UserID == "" || req.ProductID == "" {
		metrics.Inc("orders_create_bad_request_total")
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "userId and productId are required"})
	}

	if _, err := uuid.Parse(req.UserID); err != nil {
		metrics.Inc("orders_create_bad_request_total")
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid userId"})
	}

	if _, err := uuid.Parse(req.ProductID); err != nil {
		metrics.Inc("orders_create_bad_request_total")
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid productId"})
	}

	if req.Quantity <= 0 {
		metrics.Inc("orders_create_bad_request_total")
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "quantity must be greater than 0"})
	}

	o := Order{
		UserID:    req.UserID,
		ProductID: req.ProductID,
		Quantity:  req.Quantity,
	}

	if err := h.service.CreateOrder(c.Request().Context(), &o); err != nil {
		metrics.Inc("orders_create_internal_error_total")
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}

	metrics.Inc("orders_create_success_total")
	return c.JSON(http.StatusCreated, o)
}
