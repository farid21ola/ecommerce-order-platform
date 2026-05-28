package saga

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"ecommerce-order-platform/internal/order/domain"
	"ecommerce-order-platform/internal/order/handlers"
	"ecommerce-order-platform/internal/order/repository"
	"ecommerce-order-platform/pkg/events"
)

type Service struct {
	repository *repository.Repository
}

func NewService(repository *repository.Repository) *Service {
	return &Service{repository: repository}
}

func (s *Service) CreateOrder(ctx context.Context, command handlers.CreateOrderCommand) (handlers.CreateOrderResult, error) {
	orderID := uuid.NewString()
	order := domain.Order{
		ID:              orderID,
		CustomerID:      command.CustomerID,
		Status:          domain.StatusCreated,
		TotalAmount:     domain.TotalAmount(command.Items),
		DeliveryAddress: command.DeliveryAddress,
		PaymentScenario: command.PaymentScenario,
		Items:           command.Items,
	}

	event, err := events.New(events.NewEventParams{
		EventType: events.TypeOrderCreated,
		OrderID:   orderID,
		Source:    events.SourceOrderService,
		Payload: events.OrderCreatedPayload{
			CustomerID:      order.CustomerID,
			Items:           toEventItems(order.Items),
			TotalAmount:     order.TotalAmount,
			DeliveryAddress: order.DeliveryAddress,
			PaymentScenario: order.PaymentScenario,
		},
	})
	if err != nil {
		return handlers.CreateOrderResult{}, err
	}

	if err := s.repository.CreateOrder(ctx, order, event, events.TopicOrders); err != nil {
		return handlers.CreateOrderResult{}, err
	}

	return handlers.CreateOrderResult{OrderID: order.ID, Status: order.Status}, nil
}

func (s *Service) CancelOrder(ctx context.Context, command handlers.CancelOrderCommand) (handlers.CancelOrderResult, error) {
	reason := "cancelled_by_customer"
	result := handlers.CancelOrderResult{OrderID: command.OrderID, Status: domain.StatusCancelled}

	err := s.repository.ProcessOrder(ctx, command.OrderID, func(ctx context.Context, tx pgx.Tx, order domain.Order) error {
		if order.Status == domain.StatusCancelled {
			return nil
		}
		if !canCancel(order.Status) {
			return handlers.ErrOrderCannotBeCancelled
		}

		if needsStockRelease(order.Status) {
			releaseEvent, err := events.New(events.NewEventParams{
				EventType: events.TypeReleaseStockRequested,
				OrderID:   order.ID,
				Source:    events.SourceOrderService,
				Payload:   events.ReleaseStockRequestedPayload{Reason: reason},
			})
			if err != nil {
				return err
			}
			if err := s.repository.AddOutboxEvent(ctx, tx, releaseEvent, events.TopicOrders); err != nil {
				return err
			}
		}

		if err := s.repository.UpdateStatus(ctx, tx, order, domain.StatusCancelled, &reason); err != nil {
			return err
		}

		cancelledEvent, err := events.New(events.NewEventParams{
			EventType: events.TypeOrderCancelled,
			OrderID:   order.ID,
			Source:    events.SourceOrderService,
			Payload: events.OrderFinalStatusPayload{
				Status: domain.StatusCancelled,
				Reason: &reason,
			},
		})
		if err != nil {
			return err
		}
		return s.repository.AddOutboxEvent(ctx, tx, cancelledEvent, events.TopicOrders)
	})
	if err != nil {
		if errors.Is(err, repository.ErrOrderNotFound) {
			return handlers.CancelOrderResult{}, handlers.ErrOrderNotFound
		}
		return handlers.CancelOrderResult{}, err
	}

	return result, nil
}

func (s *Service) HandleEvent(ctx context.Context, event events.Event) error {
	switch event.EventType {
	case events.TypeStockReserved:
		return s.handleStockReserved(ctx, event)
	case events.TypeStockReservationFailed:
		return s.handleStockReservationFailed(ctx, event)
	case events.TypePaymentCompleted:
		return s.handlePaymentCompleted(ctx, event)
	case events.TypePaymentFailed:
		return s.handlePaymentFailed(ctx, event)
	case events.TypeDeliveryCreated:
		return s.handleDeliveryCreated(ctx, event)
	case events.TypeDeliveryFailed:
		return s.handleDeliveryFailed(ctx, event)
	case events.TypeStockReleased:
		return s.handleStockReleased(ctx, event)
	default:
		return nil
	}
}

func (s *Service) handleStockReserved(ctx context.Context, event events.Event) error {
	return s.repository.ProcessEvent(ctx, event, func(ctx context.Context, tx pgx.Tx, order domain.Order) error {
		if order.Status == domain.StatusCancelled {
			outEvent, err := s.newDerivedEvent(event, events.TypeReleaseStockRequested, events.ReleaseStockRequestedPayload{Reason: "cancelled_by_customer"})
			if err != nil {
				return err
			}
			return s.repository.AddOutboxEvent(ctx, tx, outEvent, events.TopicOrders)
		}
		if isTerminal(order.Status) {
			return nil
		}

		if err := s.repository.UpdateStatus(ctx, tx, order, domain.StatusStockReserved, nil); err != nil {
			return err
		}
		order.Status = domain.StatusStockReserved
		if err := s.repository.UpdateStatus(ctx, tx, order, domain.StatusPaymentPending, nil); err != nil {
			return err
		}

		outEvent, err := s.newDerivedEvent(event, events.TypePaymentRequested, events.PaymentRequestedPayload{
			Amount:          order.TotalAmount,
			PaymentScenario: order.PaymentScenario,
		})
		if err != nil {
			return err
		}
		return s.repository.AddOutboxEvent(ctx, tx, outEvent, events.TopicOrders)
	})
}

func (s *Service) handleStockReservationFailed(ctx context.Context, event events.Event) error {
	reason := "stock_reservation_failed"
	return s.repository.ProcessEvent(ctx, event, func(ctx context.Context, tx pgx.Tx, order domain.Order) error {
		if isTerminal(order.Status) {
			return nil
		}

		if err := s.repository.UpdateStatus(ctx, tx, order, domain.StatusFailed, &reason); err != nil {
			return err
		}

		outEvent, err := s.newDerivedEvent(event, events.TypeOrderFailed, events.OrderFinalStatusPayload{
			Status: domain.StatusFailed,
			Reason: &reason,
		})
		if err != nil {
			return err
		}
		return s.repository.AddOutboxEvent(ctx, tx, outEvent, events.TopicOrders)
	})
}

func (s *Service) handlePaymentCompleted(ctx context.Context, event events.Event) error {
	return s.repository.ProcessEvent(ctx, event, func(ctx context.Context, tx pgx.Tx, order domain.Order) error {
		if isTerminal(order.Status) {
			return nil
		}

		if err := s.repository.UpdateStatus(ctx, tx, order, domain.StatusPaid, nil); err != nil {
			return err
		}
		order.Status = domain.StatusPaid
		if err := s.repository.UpdateStatus(ctx, tx, order, domain.StatusDeliveryPending, nil); err != nil {
			return err
		}

		outEvent, err := s.newDerivedEvent(event, events.TypeDeliveryRequested, events.DeliveryRequestedPayload{
			DeliveryAddress: order.DeliveryAddress,
		})
		if err != nil {
			return err
		}
		return s.repository.AddOutboxEvent(ctx, tx, outEvent, events.TopicOrders)
	})
}

func (s *Service) handlePaymentFailed(ctx context.Context, event events.Event) error {
	reason := "payment_failed"
	return s.repository.ProcessEvent(ctx, event, func(ctx context.Context, tx pgx.Tx, order domain.Order) error {
		if isTerminal(order.Status) {
			return nil
		}

		if err := s.repository.UpdateStatus(ctx, tx, order, domain.StatusCancelled, &reason); err != nil {
			return err
		}

		releaseEvent, err := s.newDerivedEvent(event, events.TypeReleaseStockRequested, events.ReleaseStockRequestedPayload{Reason: reason})
		if err != nil {
			return err
		}
		if err := s.repository.AddOutboxEvent(ctx, tx, releaseEvent, events.TopicOrders); err != nil {
			return err
		}

		cancelledEvent, err := s.newDerivedEvent(event, events.TypeOrderCancelled, events.OrderFinalStatusPayload{
			Status: domain.StatusCancelled,
			Reason: &reason,
		})
		if err != nil {
			return err
		}
		return s.repository.AddOutboxEvent(ctx, tx, cancelledEvent, events.TopicOrders)
	})
}

func (s *Service) handleDeliveryCreated(ctx context.Context, event events.Event) error {
	return s.repository.ProcessEvent(ctx, event, func(ctx context.Context, tx pgx.Tx, order domain.Order) error {
		if isTerminal(order.Status) {
			return nil
		}

		if err := s.repository.UpdateStatus(ctx, tx, order, domain.StatusInDelivery, nil); err != nil {
			return err
		}
		order.Status = domain.StatusInDelivery
		if err := s.repository.UpdateStatus(ctx, tx, order, domain.StatusCompleted, nil); err != nil {
			return err
		}

		outEvent, err := s.newDerivedEvent(event, events.TypeOrderCompleted, events.OrderFinalStatusPayload{Status: domain.StatusCompleted})
		if err != nil {
			return err
		}
		return s.repository.AddOutboxEvent(ctx, tx, outEvent, events.TopicOrders)
	})
}

func (s *Service) handleDeliveryFailed(ctx context.Context, event events.Event) error {
	reason := "delivery_failed"
	return s.repository.ProcessEvent(ctx, event, func(ctx context.Context, tx pgx.Tx, order domain.Order) error {
		if isTerminal(order.Status) {
			return nil
		}

		if err := s.repository.UpdateStatus(ctx, tx, order, domain.StatusFailed, &reason); err != nil {
			return err
		}

		outEvent, err := s.newDerivedEvent(event, events.TypeOrderFailed, events.OrderFinalStatusPayload{
			Status: domain.StatusFailed,
			Reason: &reason,
		})
		if err != nil {
			return err
		}
		return s.repository.AddOutboxEvent(ctx, tx, outEvent, events.TopicOrders)
	})
}

func (s *Service) handleStockReleased(ctx context.Context, event events.Event) error {
	return s.repository.ProcessEvent(ctx, event, func(ctx context.Context, tx pgx.Tx, order domain.Order) error {
		return nil
	})
}

func (s *Service) newDerivedEvent(parent events.Event, eventType string, payload any) (events.Event, error) {
	causationID := parent.EventID
	return events.New(events.NewEventParams{
		EventType:     eventType,
		OrderID:       parent.OrderID,
		CorrelationID: parent.CorrelationID,
		CausationID:   &causationID,
		Source:        events.SourceOrderService,
		Payload:       payload,
	})
}

func toEventItems(items []domain.Item) []events.OrderItem {
	result := make([]events.OrderItem, 0, len(items))
	for _, item := range items {
		result = append(result, events.OrderItem{
			SKU:      item.SKU,
			Quantity: item.Quantity,
			Price:    item.Price,
		})
	}
	return result
}

func canCancel(status string) bool {
	switch status {
	case domain.StatusCreated,
		domain.StatusStockReservationPending,
		domain.StatusStockReserved,
		domain.StatusPaymentPending,
		domain.StatusPaid,
		domain.StatusDeliveryPending:
		return true
	default:
		return false
	}
}

func needsStockRelease(status string) bool {
	switch status {
	case domain.StatusStockReserved,
		domain.StatusPaymentPending,
		domain.StatusPaid,
		domain.StatusDeliveryPending:
		return true
	default:
		return false
	}
}

func isTerminal(status string) bool {
	switch status {
	case domain.StatusCompleted, domain.StatusFailed, domain.StatusCancelled:
		return true
	default:
		return false
	}
}
