package repository

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"ecommerce-order-platform/internal/inventory/domain"
	"ecommerce-order-platform/pkg/events"
)

var ErrReservationNotFound = errors.New("reservation not found")

type Repository struct {
	pool *pgxpool.Pool
}

func New(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

func (r *Repository) ReserveStock(ctx context.Context, event events.Event, items []domain.Item) (domain.Reservation, []domain.FailedItem, bool, error) {
	var reservation domain.Reservation
	var failedItems []domain.FailedItem
	var processed bool

	err := r.inTx(ctx, func(tx pgx.Tx) error {
		inserted, err := markEventProcessed(ctx, tx, event)
		if err != nil || !inserted {
			return err
		}
		processed = true

		reservation, err = findReservation(ctx, tx, event.OrderID)
		if err == nil {
			return nil
		}
		if !errors.Is(err, ErrReservationNotFound) {
			return err
		}

		for _, item := range items {
			var available int
			err := tx.QueryRow(ctx, `
				SELECT available_quantity
				FROM stock_items
				WHERE sku = $1
				FOR UPDATE
			`, item.SKU).Scan(&available)
			if errors.Is(err, pgx.ErrNoRows) {
				failedItems = append(failedItems, domain.FailedItem{SKU: item.SKU, Requested: item.Quantity, Available: 0})
				continue
			}
			if err != nil {
				return err
			}
			if available < item.Quantity {
				failedItems = append(failedItems, domain.FailedItem{SKU: item.SKU, Requested: item.Quantity, Available: available})
			}
		}

		if len(failedItems) > 0 {
			return nil
		}

		reservation = domain.Reservation{
			ID:      uuid.NewString(),
			OrderID: event.OrderID,
			Status:  domain.ReservationStatusReserved,
			Items:   items,
		}

		if _, err := tx.Exec(ctx, `
			INSERT INTO stock_reservations (id, order_id, status)
			VALUES ($1, $2, $3)
		`, reservation.ID, reservation.OrderID, reservation.Status); err != nil {
			return err
		}

		for _, item := range items {
			if _, err := tx.Exec(ctx, `
				UPDATE stock_items
				SET available_quantity = available_quantity - $2,
					reserved_quantity = reserved_quantity + $2,
					updated_at = now()
				WHERE sku = $1
			`, item.SKU, item.Quantity); err != nil {
				return err
			}

			if _, err := tx.Exec(ctx, `
				INSERT INTO stock_reservation_items (id, reservation_id, sku, quantity)
				VALUES ($1, $2, $3, $4)
			`, uuid.NewString(), reservation.ID, item.SKU, item.Quantity); err != nil {
				return err
			}
		}

		return nil
	})

	return reservation, failedItems, processed, err
}

func (r *Repository) ReleaseStock(ctx context.Context, event events.Event) (domain.Reservation, bool, error) {
	var reservation domain.Reservation
	var processed bool

	err := r.inTx(ctx, func(tx pgx.Tx) error {
		inserted, err := markEventProcessed(ctx, tx, event)
		if err != nil || !inserted {
			return err
		}
		processed = true

		reservation, err = findReservation(ctx, tx, event.OrderID)
		if err != nil {
			return err
		}

		if reservation.Status == domain.ReservationStatusReleased {
			return nil
		}

		for _, item := range reservation.Items {
			if _, err := tx.Exec(ctx, `
				UPDATE stock_items
				SET available_quantity = available_quantity + $2,
					reserved_quantity = reserved_quantity - $2,
					updated_at = now()
				WHERE sku = $1
			`, item.SKU, item.Quantity); err != nil {
				return err
			}
		}

		_, err = tx.Exec(ctx, `
			UPDATE stock_reservations
			SET status = $2, updated_at = now()
			WHERE id = $1
		`, reservation.ID, domain.ReservationStatusReleased)
		if err != nil {
			return err
		}
		reservation.Status = domain.ReservationStatusReleased

		return nil
	})

	return reservation, processed, err
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

func findReservation(ctx context.Context, tx pgx.Tx, orderID string) (domain.Reservation, error) {
	var reservation domain.Reservation
	err := tx.QueryRow(ctx, `
		SELECT id, order_id, status
		FROM stock_reservations
		WHERE order_id = $1
		FOR UPDATE
	`, orderID).Scan(&reservation.ID, &reservation.OrderID, &reservation.Status)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Reservation{}, ErrReservationNotFound
	}
	if err != nil {
		return domain.Reservation{}, err
	}

	rows, err := tx.Query(ctx, `
		SELECT sku, quantity
		FROM stock_reservation_items
		WHERE reservation_id = $1
		ORDER BY sku
	`, reservation.ID)
	if err != nil {
		return domain.Reservation{}, err
	}
	defer rows.Close()

	for rows.Next() {
		var item domain.Item
		if err := rows.Scan(&item.SKU, &item.Quantity); err != nil {
			return domain.Reservation{}, err
		}
		reservation.Items = append(reservation.Items, item)
	}

	return reservation, rows.Err()
}
