package service

import (
	"context"

	"ecommerce-order-platform/internal/payment/domain"
	"ecommerce-order-platform/internal/payment/repository"
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
	if event.EventType != events.TypePaymentRequested {
		return nil
	}
	return s.handlePaymentRequested(ctx, event)
}

func (s *Service) handlePaymentRequested(ctx context.Context, event events.Event) error {
	payload, err := events.DecodePayload[events.PaymentRequestedPayload](event)
	if err != nil {
		return err
	}

	status := domain.StatusCompleted
	var reason *string
	if payload.PaymentScenario == "fail" {
		status = domain.StatusFailed
		paymentDeclined := "payment_declined"
		reason = &paymentDeclined
	}

	payment, processed, err := s.repository.CreatePayment(ctx, event, payload.Amount, status, reason)
	if err != nil {
		return err
	}
	if !processed {
		return nil
	}

	if payment.Status == domain.StatusFailed {
		outEvent, err := s.newDerivedEvent(event, events.TypePaymentFailed, events.PaymentFailedPayload{
			PaymentID: payment.ID,
			Amount:    payment.Amount,
			Status:    payment.Status,
			Reason:    deref(payment.Reason),
		})
		if err != nil {
			return err
		}
		return s.producer.Publish(ctx, outEvent)
	}

	outEvent, err := s.newDerivedEvent(event, events.TypePaymentCompleted, events.PaymentCompletedPayload{
		PaymentID: payment.ID,
		Amount:    payment.Amount,
		Status:    payment.Status,
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
		Source:        events.SourcePaymentService,
		Payload:       payload,
	})
}

func deref(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
