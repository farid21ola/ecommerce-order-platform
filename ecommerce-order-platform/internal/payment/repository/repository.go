package repository

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"ecommerce-order-platform/internal/payment/domain"
	"ecommerce-order-platform/pkg/events"
)

type Repository struct {
	pool *pgxpool.Pool
}

func New(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

func (r *Repository) CreatePayment(ctx context.Context, event events.Event, amount int64, status string, reason *string) (domain.Payment, bool, error) {
	var payment domain.Payment
	var processed bool

	err := r.inTx(ctx, func(tx pgx.Tx) error {
		inserted, err := markEventProcessed(ctx, tx, event)
		if err != nil || !inserted {
			return err
		}
		processed = true

		payment, err = findPayment(ctx, tx, event.OrderID)
		if err == nil {
			return nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}

		payment = domain.Payment{
			ID:      uuid.NewString(),
			OrderID: event.OrderID,
			Amount:  amount,
			Status:  status,
			Reason:  reason,
		}

		_, err = tx.Exec(ctx, `
			INSERT INTO payments (id, order_id, amount, status, reason)
			VALUES ($1, $2, $3, $4, $5)
		`, payment.ID, payment.OrderID, payment.Amount, payment.Status, payment.Reason)
		return err
	})

	return payment, processed, err
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

func findPayment(ctx context.Context, tx pgx.Tx, orderID string) (domain.Payment, error) {
	var payment domain.Payment
	err := tx.QueryRow(ctx, `
		SELECT id, order_id, amount, status, reason
		FROM payments
		WHERE order_id = $1
		FOR UPDATE
	`, orderID).Scan(&payment.ID, &payment.OrderID, &payment.Amount, &payment.Status, &payment.Reason)
	return payment, err
}
