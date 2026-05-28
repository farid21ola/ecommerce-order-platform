package service

import (
	"context"

	"ecommerce-order-platform/internal/readmodel/repository"
	"ecommerce-order-platform/pkg/events"
)

type Service struct {
	repository *repository.Repository
}

func New(repository *repository.Repository) *Service {
	return &Service{repository: repository}
}

func (s *Service) HandleEvent(ctx context.Context, event events.Event) error {
	return s.repository.ApplyEvent(ctx, event)
}

func (s *Service) GetOrder(ctx context.Context, orderID string) (repository.OrderView, error) {
	return s.repository.GetOrder(ctx, orderID)
}

func (s *Service) ListOrders(ctx context.Context, limit int) ([]repository.OrderView, error) {
	return s.repository.ListOrders(ctx, limit)
}

func (s *Service) GetHistory(ctx context.Context, orderID string) ([]repository.HistoryEvent, error) {
	return s.repository.GetHistory(ctx, orderID)
}
