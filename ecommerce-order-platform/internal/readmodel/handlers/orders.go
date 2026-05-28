package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"ecommerce-order-platform/internal/readmodel/repository"
)

type QueryService interface {
	ListOrders(ctx context.Context, limit int) ([]repository.OrderView, error)
	GetOrder(ctx context.Context, orderID string) (repository.OrderView, error)
	GetHistory(ctx context.Context, orderID string) ([]repository.HistoryEvent, error)
}

type Handler struct {
	service QueryService
}

func New(service QueryService) *Handler {
	return &Handler{service: service}
}

func (h *Handler) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /orders", h.listOrders)
	mux.HandleFunc("GET /orders/{order_id}", h.getOrder)
	mux.HandleFunc("GET /orders/{order_id}/history", h.getHistory)
}

func (h *Handler) listOrders(w http.ResponseWriter, r *http.Request) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	orders, err := h.service.ListOrders(r.Context(), limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list orders")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"orders": orders})
}

func (h *Handler) getOrder(w http.ResponseWriter, r *http.Request) {
	order, err := h.service.GetOrder(r.Context(), r.PathValue("order_id"))
	if errors.Is(err, repository.ErrOrderNotFound) {
		writeError(w, http.StatusNotFound, "order not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to get order")
		return
	}

	writeJSON(w, http.StatusOK, order)
}

func (h *Handler) getHistory(w http.ResponseWriter, r *http.Request) {
	orderID := r.PathValue("order_id")
	history, err := h.service.GetHistory(r.Context(), orderID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to get order history")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"order_id": orderID,
		"events":   history,
	})
}

func writeJSON(w http.ResponseWriter, status int, response any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(response)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}
