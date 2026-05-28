package kafka

import (
	"context"

	kafkago "github.com/segmentio/kafka-go"

	"ecommerce-order-platform/pkg/events"
)

type Consumer struct {
	reader *kafkago.Reader
}

type ConsumerConfig struct {
	Brokers  []string
	Topic    string
	GroupID  string
	MinBytes int
	MaxBytes int
}

type Message struct {
	Event     events.Event
	Topic     string
	Partition int
	Offset    int64
	Key       []byte
}

func NewConsumer(config ConsumerConfig) *Consumer {
	minBytes := config.MinBytes
	if minBytes == 0 {
		minBytes = 1
	}

	maxBytes := config.MaxBytes
	if maxBytes == 0 {
		maxBytes = 10e6
	}

	return &Consumer{
		reader: kafkago.NewReader(kafkago.ReaderConfig{
			Brokers:  config.Brokers,
			Topic:    config.Topic,
			GroupID:  config.GroupID,
			MinBytes: minBytes,
			MaxBytes: maxBytes,
		}),
	}
}

func (c *Consumer) Fetch(ctx context.Context) (Message, error) {
	message, err := c.reader.FetchMessage(ctx)
	if err != nil {
		return Message{}, err
	}

	event, err := events.Unmarshal(message.Value)
	if err != nil {
		return Message{}, err
	}

	return Message{
		Event:     event,
		Topic:     message.Topic,
		Partition: message.Partition,
		Offset:    message.Offset,
		Key:       message.Key,
	}, nil
}

func (c *Consumer) Commit(ctx context.Context, message Message) error {
	return c.reader.CommitMessages(ctx, kafkago.Message{
		Topic:     message.Topic,
		Partition: message.Partition,
		Offset:    message.Offset,
		Key:       message.Key,
	})
}

func (c *Consumer) Close() error {
	return c.reader.Close()
}
