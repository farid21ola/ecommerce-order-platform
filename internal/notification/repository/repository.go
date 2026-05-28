package repository

import (
	"context"
	"encoding/json"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"ecommerce-order-platform/internal/notification/domain"
	"ecommerce-order-platform/pkg/events"
)

type Repository struct {
	pool *pgxpool.Pool
}

func New(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

func (r *Repository) CreateSentTask(ctx context.Context, event events.Event) (domain.Task, bool, error) {
	var task domain.Task
	var processed bool

	err := r.inTx(ctx, func(tx pgx.Tx) error {
		inserted, err := markEventProcessed(ctx, tx, event)
		if err != nil || !inserted {
			return err
		}
		processed = true

		payload, err := json.Marshal(event)
		if err != nil {
			return err
		}

		task = domain.Task{
			ID:      uuid.NewString(),
			OrderID: event.OrderID,
			EventID: event.EventID,
			Status:  domain.TaskStatusSent,
		}

		if _, err := tx.Exec(ctx, `
			INSERT INTO notification_tasks (id, order_id, event_id, status, payload)
			VALUES ($1, $2, $3, $4, $5)
		`, task.ID, task.OrderID, task.EventID, task.Status, payload); err != nil {
			return err
		}

		_, err = tx.Exec(ctx, `
			INSERT INTO sending_log (id, notification_task_id, status, message)
			VALUES ($1, $2, $3, $4)
		`, uuid.NewString(), task.ID, task.Status, "notification sent by prototype logger")
		return err
	})

	return task, processed, err
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
