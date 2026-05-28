package repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"ecommerce-order-platform/internal/order/domain"
	"ecommerce-order-platform/pkg/events"
)

type Repository struct {
	pool *pgxpool.Pool
}

var ErrOrderNotFound = errors.New("order not found")

type OutboxEvent struct {
	ID          string
	EventType   string
	Topic       string
	AggregateID string
	Payload     []byte
}

func New(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

func (r *Repository) CreateOrder(ctx context.Context, order domain.Order, event events.Event, topic string) error {
	return r.inTx(ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `
			INSERT INTO orders (id, customer_id, status, total_amount, delivery_address, payment_scenario)
			VALUES ($1, $2, $3, $4, $5, $6)
		`, order.ID, order.CustomerID, order.Status, order.TotalAmount, order.DeliveryAddress, order.PaymentScenario); err != nil {
			return err
		}

		for _, item := range order.Items {
			if _, err := tx.Exec(ctx, `
				INSERT INTO order_items (id, order_id, sku, quantity, price)
				VALUES ($1, $2, $3, $4, $5)
			`, uuid.NewString(), order.ID, item.SKU, item.Quantity, item.Price); err != nil {
				return err
			}
		}

		if err := insertStatusHistory(ctx, tx, order.ID, nil, order.Status, nil); err != nil {
			return err
		}

		return insertOutboxEvent(ctx, tx, event, topic)
	})
}

func (r *Repository) ProcessEvent(ctx context.Context, event events.Event, handler func(ctx context.Context, tx pgx.Tx, order domain.Order) error) error {
	return r.inTx(ctx, func(tx pgx.Tx) error {
		inserted, err := markEventProcessed(ctx, tx, event)
		if err != nil || !inserted {
			return err
		}

		order, err := lockOrder(ctx, tx, event.OrderID)
		if err != nil {
			return err
		}

		return handler(ctx, tx, order)
	})
}

func (r *Repository) ProcessOrder(ctx context.Context, orderID string, handler func(ctx context.Context, tx pgx.Tx, order domain.Order) error) error {
	return r.inTx(ctx, func(tx pgx.Tx) error {
		order, err := lockOrder(ctx, tx, orderID)
		if err != nil {
			return err
		}

		return handler(ctx, tx, order)
	})
}

func (r *Repository) UpdateStatus(ctx context.Context, tx pgx.Tx, order domain.Order, newStatus string, reason *string) error {
	if order.Status == newStatus {
		return nil
	}

	oldStatus := order.Status
	if _, err := tx.Exec(ctx, `
		UPDATE orders
		SET status = $2, updated_at = now()
		WHERE id = $1
	`, order.ID, newStatus); err != nil {
		return err
	}

	return insertStatusHistory(ctx, tx, order.ID, &oldStatus, newStatus, reason)
}

func (r *Repository) AddOutboxEvent(ctx context.Context, tx pgx.Tx, event events.Event, topic string) error {
	return insertOutboxEvent(ctx, tx, event, topic)
}

func (r *Repository) FetchPendingOutbox(ctx context.Context, limit int) ([]OutboxEvent, error) {
	if limit <= 0 {
		limit = 50
	}

	rows, err := r.pool.Query(ctx, `
		SELECT id, event_type, topic, aggregate_id, payload
		FROM outbox_events
		WHERE status = 'pending'
		ORDER BY created_at
		LIMIT $1
	`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []OutboxEvent
	for rows.Next() {
		var event OutboxEvent
		if err := rows.Scan(&event.ID, &event.EventType, &event.Topic, &event.AggregateID, &event.Payload); err != nil {
			return nil, err
		}
		result = append(result, event)
	}

	return result, rows.Err()
}

func (r *Repository) MarkOutboxPublished(ctx context.Context, id string) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE outbox_events
		SET status = 'published', published_at = now()
		WHERE id = $1
	`, id)
	return err
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

func lockOrder(ctx context.Context, tx pgx.Tx, orderID string) (domain.Order, error) {
	var order domain.Order
	err := tx.QueryRow(ctx, `
		SELECT id, customer_id, status, total_amount, delivery_address, payment_scenario, created_at, updated_at
		FROM orders
		WHERE id = $1
		FOR UPDATE
	`, orderID).Scan(
		&order.ID,
		&order.CustomerID,
		&order.Status,
		&order.TotalAmount,
		&order.DeliveryAddress,
		&order.PaymentScenario,
		&order.CreatedAt,
		&order.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Order{}, fmt.Errorf("%w: %s", ErrOrderNotFound, orderID)
	}
	if err != nil {
		return domain.Order{}, err
	}

	rows, err := tx.Query(ctx, `
		SELECT sku, quantity, price
		FROM order_items
		WHERE order_id = $1
		ORDER BY sku
	`, orderID)
	if err != nil {
		return domain.Order{}, err
	}
	defer rows.Close()

	for rows.Next() {
		var item domain.Item
		if err := rows.Scan(&item.SKU, &item.Quantity, &item.Price); err != nil {
			return domain.Order{}, err
		}
		order.Items = append(order.Items, item)
	}

	return order, rows.Err()
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

func insertStatusHistory(ctx context.Context, tx pgx.Tx, orderID string, oldStatus *string, newStatus string, reason *string) error {
	_, err := tx.Exec(ctx, `
		INSERT INTO status_history (id, order_id, old_status, new_status, reason)
		VALUES ($1, $2, $3, $4, $5)
	`, uuid.NewString(), orderID, oldStatus, newStatus, reason)
	return err
}

func insertOutboxEvent(ctx context.Context, tx pgx.Tx, event events.Event, topic string) error {
	payload, err := events.Marshal(event)
	if err != nil {
		return err
	}

	_, err = tx.Exec(ctx, `
		INSERT INTO outbox_events (id, event_type, topic, aggregate_id, payload)
		VALUES ($1, $2, $3, $4, $5)
	`, event.EventID, event.EventType, topic, event.OrderID, payload)
	return err
}
