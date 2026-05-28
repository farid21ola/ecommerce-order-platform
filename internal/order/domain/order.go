package domain

import "time"

const (
	StatusCreated                 = "CREATED"
	StatusStockReservationPending = "STOCK_RESERVATION_PENDING"
	StatusStockReserved           = "STOCK_RESERVED"
	StatusPaymentPending          = "PAYMENT_PENDING"
	StatusPaid                    = "PAID"
	StatusDeliveryPending         = "DELIVERY_PENDING"
	StatusInDelivery              = "IN_DELIVERY"
	StatusCompleted               = "COMPLETED"
	StatusFailed                  = "FAILED"
	StatusCancelled               = "CANCELLED"
)

type Order struct {
	ID              string
	CustomerID      string
	Status          string
	TotalAmount     int64
	DeliveryAddress string
	PaymentScenario string
	Items           []Item
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

type Item struct {
	SKU      string
	Quantity int
	Price    int64
}

func TotalAmount(items []Item) int64 {
	var total int64
	for _, item := range items {
		total += int64(item.Quantity) * item.Price
	}
	return total
}
