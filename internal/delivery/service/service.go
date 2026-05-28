package service

import (
	"context"

	"ecommerce-order-platform/internal/delivery/repository"
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
	if event.EventType != events.TypeDeliveryRequested {
		return nil
	}
	return s.handleDeliveryRequested(ctx, event)
}

func (s *Service) handleDeliveryRequested(ctx context.Context, event events.Event) error {
	payload, err := events.DecodePayload[events.DeliveryRequestedPayload](event)
	if err != nil {
		return err
	}

	delivery, processed, err := s.repository.CreateDelivery(ctx, event, payload.DeliveryAddress)
	if err != nil {
		return err
	}
	if !processed {
		return nil
	}

	outEvent, err := s.newDerivedEvent(event, events.TypeDeliveryCreated, events.DeliveryCreatedPayload{
		DeliveryID:     delivery.ID,
		Status:         delivery.Status,
		TrackingNumber: delivery.TrackingNumber,
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
		Source:        events.SourceDeliveryService,
		Payload:       payload,
	})
}
