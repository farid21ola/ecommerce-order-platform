package kafka

import (
	"context"
	"time"

	kafkago "github.com/segmentio/kafka-go"

	"ecommerce-order-platform/pkg/events"
)

type Producer struct {
	writer *kafkago.Writer
}

type ProducerConfig struct {
	Brokers      []string
	Topic        string
	BatchTimeout time.Duration
}

func NewProducer(config ProducerConfig) *Producer {
	batchTimeout := config.BatchTimeout
	if batchTimeout == 0 {
		batchTimeout = 10 * time.Millisecond
	}

	return &Producer{
		writer: &kafkago.Writer{
			Addr:         kafkago.TCP(config.Brokers...),
			Topic:        config.Topic,
			Balancer:     &kafkago.LeastBytes{},
			BatchTimeout: batchTimeout,
		},
	}
}

func (p *Producer) Publish(ctx context.Context, event events.Event) error {
	value, err := events.Marshal(event)
	if err != nil {
		return err
	}

	return p.writer.WriteMessages(ctx, kafkago.Message{
		Key:   []byte(event.OrderID),
		Value: value,
		Time:  event.OccurredAt,
	})
}

func (p *Producer) Close() error {
	return p.writer.Close()
}
