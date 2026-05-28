package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"ecommerce-order-platform/internal/order/domain"
)

type OrderCreator interface {
	CreateOrder(ctx context.Context, command CreateOrderCommand) (CreateOrderResult, error)
	CancelOrder(ctx context.Context, command CancelOrderCommand) (CancelOrderResult, error)
}

type CreateOrderCommand struct {
	CustomerID      string
	Items           []domain.Item
	DeliveryAddress string
	PaymentScenario string
}

type CreateOrderResult struct {
	OrderID string
	Status  string
}

type CancelOrderCommand struct {
	OrderID string
}

type CancelOrderResult struct {
	OrderID string
	Status  string
}

type OrderHandler struct {
	creator OrderCreator
}

func NewOrderHandler(creator OrderCreator) *OrderHandler {
	return &OrderHandler{creator: creator}
}

func (h *OrderHandler) Register(mux *http.ServeMux) {
	mux.HandleFunc("POST /orders", h.createOrder)
	mux.HandleFunc("POST /orders/{order_id}/cancel", h.cancelOrder)
}

func (h *OrderHandler) createOrder(w http.ResponseWriter, r *http.Request) {
	var request createOrderRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	command, err := request.toCommand()
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	result, err := h.creator.CreateOrder(r.Context(), command)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create order")
		return
	}

	writeJSON(w, http.StatusCreated, createOrderResponse{
		OrderID: result.OrderID,
		Status:  result.Status,
	})
}

func (h *OrderHandler) cancelOrder(w http.ResponseWriter, r *http.Request) {
	orderID := r.PathValue("order_id")
	if orderID == "" {
		writeError(w, http.StatusBadRequest, "order_id is required")
		return
	}

	result, err := h.creator.CancelOrder(r.Context(), CancelOrderCommand{OrderID: orderID})
	if errors.Is(err, ErrOrderNotFound) {
		writeError(w, http.StatusNotFound, "order not found")
		return
	}
	if errors.Is(err, ErrOrderCannotBeCancelled) {
		writeError(w, http.StatusConflict, "order cannot be cancelled")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to cancel order")
		return
	}

	writeJSON(w, http.StatusOK, cancelOrderResponse{
		OrderID: result.OrderID,
		Status:  result.Status,
	})
}

type createOrderRequest struct {
	CustomerID      string            `json:"customer_id"`
	Items           []createOrderItem `json:"items"`
	DeliveryAddress string            `json:"delivery_address"`
	PaymentScenario string            `json:"payment_scenario"`
}

type createOrderItem struct {
	SKU      string `json:"sku"`
	Quantity int    `json:"quantity"`
	Price    int64  `json:"price"`
}

type createOrderResponse struct {
	OrderID string `json:"order_id"`
	Status  string `json:"status"`
}

type cancelOrderResponse struct {
	OrderID string `json:"order_id"`
	Status  string `json:"status"`
}

func (r createOrderRequest) toCommand() (CreateOrderCommand, error) {
	if r.CustomerID == "" {
		return CreateOrderCommand{}, errors.New("customer_id is required")
	}
	if len(r.Items) == 0 {
		return CreateOrderCommand{}, errors.New("items are required")
	}
	if r.DeliveryAddress == "" {
		return CreateOrderCommand{}, errors.New("delivery_address is required")
	}
	if r.PaymentScenario == "" {
		r.PaymentScenario = "success"
	}
	if r.PaymentScenario != "success" && r.PaymentScenario != "fail" {
		return CreateOrderCommand{}, errors.New("payment_scenario must be success or fail")
	}

	items := make([]domain.Item, 0, len(r.Items))
	for _, item := range r.Items {
		if item.SKU == "" {
			return CreateOrderCommand{}, errors.New("item sku is required")
		}
		if item.Quantity <= 0 {
			return CreateOrderCommand{}, errors.New("item quantity must be greater than zero")
		}
		if item.Price < 0 {
			return CreateOrderCommand{}, errors.New("item price must not be negative")
		}
		items = append(items, domain.Item{
			SKU:      item.SKU,
			Quantity: item.Quantity,
			Price:    item.Price,
		})
	}

	return CreateOrderCommand{
		CustomerID:      r.CustomerID,
		Items:           items,
		DeliveryAddress: r.DeliveryAddress,
		PaymentScenario: r.PaymentScenario,
	}, nil
}

func writeJSON(w http.ResponseWriter, status int, response any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(response)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}
