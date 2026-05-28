package repository

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"ecommerce-order-platform/pkg/events"
)

var ErrOrderNotFound = errors.New("order not found")

type Repository struct {
	pool *pgxpool.Pool
}

type OrderView struct {
	OrderID        string    `json:"order_id"`
	CustomerID     string    `json:"customer_id"`
	OrderStatus    string    `json:"order_status"`
	PaymentStatus  *string   `json:"payment_status"`
	DeliveryStatus *string   `json:"delivery_status"`
	TotalAmount    int64     `json:"total_amount"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

type HistoryEvent struct {
	EventID     string          `json:"event_id"`
	EventType   string          `json:"event_type"`
	ServiceName string          `json:"service_name"`
	OccurredAt  time.Time       `json:"occurred_at"`
	Payload     json.RawMessage `json:"payload"`
}

func New(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

func (r *Repository) ApplyEvent(ctx context.Context, event events.Event) error {
	return r.inTx(ctx, func(tx pgx.Tx) error {
		inserted, err := markEventProcessed(ctx, tx, event)
		if err != nil || !inserted {
			return err
		}

		if _, err := tx.Exec(ctx, `
			INSERT INTO order_event_history (event_id, order_id, event_type, service_name, occurred_at, payload)
			VALUES ($1, $2, $3, $4, $5, $6)
			ON CONFLICT (event_id) DO NOTHING
		`, event.EventID, event.OrderID, event.EventType, event.Source, event.OccurredAt, event.Payload); err != nil {
			return err
		}

		if event.EventType == events.TypeOrderCreated {
			payload, err := events.DecodePayload[events.OrderCreatedPayload](event)
			if err != nil {
				return err
			}
			if _, err = tx.Exec(ctx, `
				INSERT INTO order_view (order_id, customer_id, order_status, total_amount, created_at, updated_at)
				VALUES ($1, $2, 'CREATED', $3, $4, $4)
				ON CONFLICT (order_id) DO UPDATE
				SET customer_id = EXCLUDED.customer_id,
					total_amount = EXCLUDED.total_amount,
					updated_at = EXCLUDED.updated_at
			`, event.OrderID, payload.CustomerID, payload.TotalAmount, event.OccurredAt); err != nil {
				return err
			}
		}

		return recomputeOrderView(ctx, tx, event.OrderID)
	})
}

func (r *Repository) GetOrder(ctx context.Context, orderID string) (OrderView, error) {
	var order OrderView
	err := r.pool.QueryRow(ctx, `
		SELECT order_id, customer_id, order_status, payment_status, delivery_status, total_amount, created_at, updated_at
		FROM order_view
		WHERE order_id = $1
	`, orderID).Scan(
		&order.OrderID,
		&order.CustomerID,
		&order.OrderStatus,
		&order.PaymentStatus,
		&order.DeliveryStatus,
		&order.TotalAmount,
		&order.CreatedAt,
		&order.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return OrderView{}, ErrOrderNotFound
	}
	if err != nil {
		return OrderView{}, err
	}
	return order, nil
}

func (r *Repository) ListOrders(ctx context.Context, limit int) ([]OrderView, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}

	rows, err := r.pool.Query(ctx, `
		SELECT order_id, customer_id, order_status, payment_status, delivery_status, total_amount, created_at, updated_at
		FROM order_view
		ORDER BY created_at DESC
		LIMIT $1
	`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var orders []OrderView
	for rows.Next() {
		var order OrderView
		if err := rows.Scan(
			&order.OrderID,
			&order.CustomerID,
			&order.OrderStatus,
			&order.PaymentStatus,
			&order.DeliveryStatus,
			&order.TotalAmount,
			&order.CreatedAt,
			&order.UpdatedAt,
		); err != nil {
			return nil, err
		}
		orders = append(orders, order)
	}
	return orders, rows.Err()
}

func (r *Repository) GetHistory(ctx context.Context, orderID string) ([]HistoryEvent, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT event_id, event_type, service_name, occurred_at, payload
		FROM order_event_history
		WHERE order_id = $1
		ORDER BY occurred_at, event_id
	`, orderID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var history []HistoryEvent
	for rows.Next() {
		var event HistoryEvent
		if err := rows.Scan(&event.EventID, &event.EventType, &event.ServiceName, &event.OccurredAt, &event.Payload); err != nil {
			return nil, err
		}
		history = append(history, event)
	}
	return history, rows.Err()
}

func (r *Repository) inTx(ctx context.Context, fn func(tx pgx.Tx) error) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if err := fn(tx); err != nil {
		return err
	}

	return tx.Commit(ctx)
}

func markEventProcessed(ctx context.Context, tx pgx.Tx, event events.Event) (bool, error) {
	commandTag, err := tx.Exec(ctx, `
		INSERT INTO processed_events (event_id, event_type)
		VALUES ($1, $2)
		ON CONFLICT (event_id) DO NOTHING
	`, event.EventID, event.EventType)
	if err != nil {
		return false, err
	}
	return commandTag.RowsAffected() == 1, nil
}

func updateOrderStatus(ctx context.Context, tx pgx.Tx, orderID string, status string) error {
	_, err := tx.Exec(ctx, `
		UPDATE order_view
		SET order_status = $2, updated_at = now()
		WHERE order_id = $1
	`, orderID, status)
	return err
}

func updatePaymentStatus(ctx context.Context, tx pgx.Tx, orderID string, status string) error {
	_, err := tx.Exec(ctx, `
		UPDATE order_view
		SET payment_status = $2, updated_at = now()
		WHERE order_id = $1
	`, orderID, status)
	return err
}

func updateDeliveryStatus(ctx context.Context, tx pgx.Tx, orderID string, status string) error {
	_, err := tx.Exec(ctx, `
		UPDATE order_view
		SET delivery_status = $2, updated_at = now()
		WHERE order_id = $1
	`, orderID, status)
	return err
}

func recomputeOrderView(ctx context.Context, tx pgx.Tx, orderID string) error {
	rows, err := tx.Query(ctx, `
		SELECT event_type, payload
		FROM order_event_history
		WHERE order_id = $1
		ORDER BY occurred_at, event_id
	`, orderID)
	if err != nil {
		return err
	}
	defer rows.Close()

	orderStatus := "CREATED"
	var paymentStatus *string
	var deliveryStatus *string

	for rows.Next() {
		var eventType string
		var payload json.RawMessage
		if err := rows.Scan(&eventType, &payload); err != nil {
			return err
		}

		switch eventType {
		case events.TypeStockReserved:
			if !isFinalOrderStatus(orderStatus) {
				orderStatus = "STOCK_RESERVED"
			}
		case events.TypePaymentRequested:
			if !isFinalOrderStatus(orderStatus) {
				orderStatus = "PAYMENT_PENDING"
			}
		case events.TypePaymentCompleted:
			if !isFinalOrderStatus(orderStatus) {
				completed := "COMPLETED"
				paymentStatus = &completed
			}
		case events.TypeDeliveryRequested:
			if !isFinalOrderStatus(orderStatus) {
				orderStatus = "DELIVERY_PENDING"
			}
		case events.TypeDeliveryCreated:
			if !isFinalOrderStatus(orderStatus) {
				created := "CREATED"
				deliveryStatus = &created
			}
		case events.TypeOrderCompleted:
			if orderStatus != "CANCELLED" && orderStatus != "FAILED" {
				orderStatus = "COMPLETED"
			}
		case events.TypeStockReservationFailed, events.TypeOrderFailed:
			orderStatus = "FAILED"
		case events.TypePaymentFailed:
			failed := "FAILED"
			paymentStatus = &failed
		case events.TypeDeliveryFailed:
			failed := "FAILED"
			deliveryStatus = &failed
			orderStatus = "FAILED"
		case events.TypeOrderCancelled:
			orderStatus = "CANCELLED"
		case events.TypeOrderStatusChanged:
			var statusPayload events.OrderStatusChangedPayload
			if err := json.Unmarshal(payload, &statusPayload); err != nil {
				return err
			}
			orderStatus = statusPayload.NewStatus
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}

	_, err = tx.Exec(ctx, `
		UPDATE order_view
		SET order_status = $2,
			payment_status = $3,
			delivery_status = $4,
			updated_at = now()
		WHERE order_id = $1
	`, orderID, orderStatus, paymentStatus, deliveryStatus)
	return err
}

func isFinalOrderStatus(status string) bool {
	switch status {
	case "COMPLETED", "FAILED", "CANCELLED":
		return true
	default:
		return false
	}
}
