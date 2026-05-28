package outbox

import (
	"context"
	"encoding/json"
	"log"
	"time"

	"ecommerce-order-platform/internal/order/repository"
	"ecommerce-order-platform/pkg/events"
	sharedkafka "ecommerce-order-platform/pkg/kafka"
)

type Repository interface {
	FetchPendingOutbox(ctx context.Context, limit int) ([]repository.OutboxEvent, error)
	MarkOutboxPublished(ctx context.Context, id string) error
}

type Worker struct {
	repository Repository
	producers  map[string]*sharedkafka.Producer
	interval   time.Duration
	batchSize  int
}

func NewWorker(repository Repository, producers map[string]*sharedkafka.Producer, interval time.Duration, batchSize int) *Worker {
	if interval == 0 {
		interval = time.Second
	}
	if batchSize == 0 {
		batchSize = 50
	}

	return &Worker{
		repository: repository,
		producers:  producers,
		interval:   interval,
		batchSize:  batchSize,
	}
}

func (w *Worker) Run(ctx context.Context) {
	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()

	for {
		if err := w.publishBatch(ctx); err != nil {
			log.Printf("outbox publish batch failed: %v", err)
		}

		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (w *Worker) publishBatch(ctx context.Context) error {
	outboxEvents, err := w.repository.FetchPendingOutbox(ctx, w.batchSize)
	if err != nil {
		return err
	}

	for _, outboxEvent := range outboxEvents {
		producer, ok := w.producers[outboxEvent.Topic]
		if !ok {
			log.Printf("outbox topic %s has no producer", outboxEvent.Topic)
			continue
		}

		var event events.Event
		if err := json.Unmarshal(outboxEvent.Payload, &event); err != nil {
			log.Printf("outbox event %s has invalid payload: %v", outboxEvent.ID, err)
			continue
		}

		if err := producer.Publish(ctx, event); err != nil {
			log.Printf("outbox event %s publish failed: %v", outboxEvent.ID, err)
			continue
		}

		if err := w.repository.MarkOutboxPublished(ctx, outboxEvent.ID); err != nil {
			log.Printf("outbox event %s mark published failed: %v", outboxEvent.ID, err)
		}
	}

	return nil
}
