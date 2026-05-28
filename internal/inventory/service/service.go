package service

import (
	"context"
	"errors"

	"ecommerce-order-platform/internal/inventory/domain"
	"ecommerce-order-platform/internal/inventory/repository"
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
	switch event.EventType {
	case events.TypeOrderCreated:
		return s.handleOrderCreated(ctx, event)
	case events.TypeReleaseStockRequested:
		return s.handleReleaseStockRequested(ctx, event)
	default:
		return nil
	}
}

func (s *Service) handleOrderCreated(ctx context.Context, event events.Event) error {
	payload, err := events.DecodePayload[events.OrderCreatedPayload](event)
	if err != nil {
		return err
	}

	reservation, failedItems, processed, err := s.repository.ReserveStock(ctx, event, toDomainItems(payload.Items))
	if err != nil {
		return err
	}
	if !processed {
		return nil
	}

	if len(failedItems) > 0 {
		outEvent, err := s.newDerivedEvent(event, events.TypeStockReservationFailed, events.StockReservationFailedPayload{
			Reason:      "insufficient_stock",
			FailedItems: toEventFailedItems(failedItems),
		})
		if err != nil {
			return err
		}
		return s.producer.Publish(ctx, outEvent)
	}

	outEvent, err := s.newDerivedEvent(event, events.TypeStockReserved, events.StockReservedPayload{
		ReservationID: reservation.ID,
		Items:         toEventItems(reservation.Items),
	})
	if err != nil {
		return err
	}
	return s.producer.Publish(ctx, outEvent)
}

func (s *Service) handleReleaseStockRequested(ctx context.Context, event events.Event) error {
	payload, err := events.DecodePayload[events.ReleaseStockRequestedPayload](event)
	if err != nil {
		return err
	}

	reservation, processed, err := s.repository.ReleaseStock(ctx, event)
	if errors.Is(err, repository.ErrReservationNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	if !processed {
		return nil
	}

	outEvent, err := s.newDerivedEvent(event, events.TypeStockReleased, events.StockReleasedPayload{
		ReservationID: reservation.ID,
		Reason:        payload.Reason,
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
		Source:        events.SourceInventoryService,
		Payload:       payload,
	})
}

func toDomainItems(items []events.OrderItem) []domain.Item {
	result := make([]domain.Item, 0, len(items))
	for _, item := range items {
		result = append(result, domain.Item{SKU: item.SKU, Quantity: item.Quantity})
	}
	return result
}

func toEventItems(items []domain.Item) []events.OrderItem {
	result := make([]events.OrderItem, 0, len(items))
	for _, item := range items {
		result = append(result, events.OrderItem{SKU: item.SKU, Quantity: item.Quantity})
	}
	return result
}

func toEventFailedItems(items []domain.FailedItem) []events.FailedStockItem {
	result := make([]events.FailedStockItem, 0, len(items))
	for _, item := range items {
		result = append(result, events.FailedStockItem{
			SKU:       item.SKU,
			Requested: item.Requested,
			Available: item.Available,
		})
	}
	return result
}
