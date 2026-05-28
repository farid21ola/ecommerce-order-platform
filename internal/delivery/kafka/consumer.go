package kafka

import (
	"context"
	"log"
	"time"

	"ecommerce-order-platform/pkg/events"
	sharedkafka "ecommerce-order-platform/pkg/kafka"
)

type EventHandler interface {
	HandleEvent(ctx context.Context, event events.Event) error
}

type ConsumerRunner struct {
	consumer *sharedkafka.Consumer
	handler  EventHandler
	topic    string
}

func NewConsumerRunner(topic string, consumer *sharedkafka.Consumer, handler EventHandler) *ConsumerRunner {
	return &ConsumerRunner{topic: topic, consumer: consumer, handler: handler}
}

func (r *ConsumerRunner) Run(ctx context.Context) {
	for {
		message, err := r.consumer.Fetch(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			log.Printf("delivery kafka consumer %s fetch failed: %v", r.topic, err)
			time.Sleep(time.Second)
			continue
		}

		if err := r.handler.HandleEvent(ctx, message.Event); err != nil {
			log.Printf("delivery kafka consumer %s handle %s failed: %v", r.topic, message.Event.EventType, err)
			continue
		}

		if err := r.consumer.Commit(ctx, message); err != nil {
			log.Printf("delivery kafka consumer %s commit failed: %v", r.topic, err)
		}
	}
}
