package service

import (
	"context"

	"ecommerce-order-platform/internal/notification/repository"
	"ecommerce-order-platform/pkg/events"
	sharedkafka "ecommerce-order-platform/pkg/kafka"
)

type Service struct {
	repository *repository.Repository
	producer   *sharedkafka.Producer
}

func New(repository *repository.Repository, producer *sharedkafka.Producer) *Service {
	return &Service{repository: repository, producer: producer}
}

func (s *Service) HandleEvent(ctx context.Context, event events.Event) error {
	if !shouldNotify(event.EventType) {
		return nil
	}

	task, processed, err := s.repository.CreateSentTask(ctx, event)
	if err != nil {
		return err
	}
	if !processed {
		return nil
	}

	outEvent, err := s.newDerivedEvent(event, events.TypeNotificationSent, events.NotificationSentPayload{
		NotificationID: task.ID,
		TriggeredBy:    event.EventType,
		Status:         task.Status,
	})
	if err != nil {
		return err
	}
	return s.producer.Publish(ctx, outEvent)
}

func (s *Service) newDerivedEvent(parent events.Event, eventType string, payload any) (events.Event, error) {
	causationID := parent.EventID
	return events.New(events.NewEventParams{
		EventType:     eventType,
		OrderID:       parent.OrderID,
		CorrelationID: parent.CorrelationID,
		CausationID:   &causationID,
		Source:        events.SourceNotificationService,
		Payload:       payload,
	})
}

func shouldNotify(eventType string) bool {
	switch eventType {
	case events.TypeOrderCompleted,
		events.TypeOrderFailed,
		events.TypeOrderCancelled:
		return true
	default:
		return false
	}
}
