package order

import (
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
	var req CreateOrderRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request"})
	}

	req.UserID = strings.TrimSpace(req.UserID)
	req.ProductID = strings.TrimSpace(req.ProductID)

	if req.UserID == "" || req.ProductID == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "userId and productId are required"})
	}

	if _, err := uuid.Parse(req.UserID); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid userId"})
	}

	if _, err := uuid.Parse(req.ProductID); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid productId"})
	}

	if req.Quantity <= 0 {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "quantity must be greater than 0"})
	}

	o := Order{
		UserID:    req.UserID,
		ProductID: req.ProductID,
		Quantity:  req.Quantity,
	}

	if err := h.service.CreateOrder(c.Request().Context(), &o); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}

	return c.JSON(http.StatusCreated, o)
}
