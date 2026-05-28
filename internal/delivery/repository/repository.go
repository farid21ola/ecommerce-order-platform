package repository

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"ecommerce-order-platform/internal/delivery/domain"
	"ecommerce-order-platform/pkg/events"
)

type Repository struct {
	pool *pgxpool.Pool
}

func New(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

func (r *Repository) CreateDelivery(ctx context.Context, event events.Event, address string) (domain.Delivery, bool, error) {
	var delivery domain.Delivery
	var processed bool

	err := r.inTx(ctx, func(tx pgx.Tx) error {
		inserted, err := markEventProcessed(ctx, tx, event)
		if err != nil || !inserted {
			return err
		}
		processed = true

		delivery, err = findDelivery(ctx, tx, event.OrderID)
		if err == nil {
			return nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}

		deliveryID := uuid.NewString()
		delivery = domain.Delivery{
			ID:              deliveryID,
			OrderID:         event.OrderID,
			Status:          domain.StatusCreated,
			DeliveryAddress: address,
			TrackingNumber:  trackingNumber(deliveryID),
		}

		_, err = tx.Exec(ctx, `
			INSERT INTO deliveries (id, order_id, status, delivery_address, tracking_number, reason)
			VALUES ($1, $2, $3, $4, $5, $6)
		`, delivery.ID, delivery.OrderID, delivery.Status, delivery.DeliveryAddress, delivery.TrackingNumber, delivery.Reason)
		return err
	})

	return delivery, processed, err
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

func findDelivery(ctx context.Context, tx pgx.Tx, orderID string) (domain.Delivery, error) {
	var delivery domain.Delivery
	err := tx.QueryRow(ctx, `
		SELECT id, order_id, status, delivery_address, tracking_number, reason
		FROM deliveries
		WHERE order_id = $1
		FOR UPDATE
	`, orderID).Scan(
		&delivery.ID,
		&delivery.OrderID,
		&delivery.Status,
		&delivery.DeliveryAddress,
		&delivery.TrackingNumber,
		&delivery.Reason,
	)
	return delivery, err
}

func trackingNumber(deliveryID string) string {
	shortID := strings.ReplaceAll(deliveryID, "-", "")
	if len(shortID) > 12 {
		shortID = shortID[:12]
	}
	return fmt.Sprintf("TRACK-%s", strings.ToUpper(shortID))
}
