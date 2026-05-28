package events

type OrderItem struct {
	SKU      string `json:"sku"`
	Quantity int    `json:"quantity"`
	Price    int64  `json:"price,omitempty"`
}

type OrderCreatedPayload struct {
	CustomerID      string      `json:"customer_id"`
	Items           []OrderItem `json:"items"`
	TotalAmount     int64       `json:"total_amount"`
	DeliveryAddress string      `json:"delivery_address"`
	PaymentScenario string      `json:"payment_scenario"`
}

type StockReservedPayload struct {
	ReservationID string      `json:"reservation_id"`
	Items         []OrderItem `json:"items"`
}

type FailedStockItem struct {
	SKU       string `json:"sku"`
	Requested int    `json:"requested"`
	Available int    `json:"available"`
}

type StockReservationFailedPayload struct {
	Reason      string            `json:"reason"`
	FailedItems []FailedStockItem `json:"failed_items"`
}

type PaymentRequestedPayload struct {
	Amount          int64  `json:"amount"`
	PaymentScenario string `json:"payment_scenario"`
}

type PaymentCompletedPayload struct {
	PaymentID string `json:"payment_id"`
	Amount    int64  `json:"amount"`
	Status    string `json:"status"`
}

type PaymentFailedPayload struct {
	PaymentID string `json:"payment_id"`
	Amount    int64  `json:"amount"`
	Status    string `json:"status"`
	Reason    string `json:"reason"`
}

type ReleaseStockRequestedPayload struct {
	Reason string `json:"reason"`
}

type StockReleasedPayload struct {
	ReservationID string `json:"reservation_id"`
	Reason        string `json:"reason"`
}

type DeliveryRequestedPayload struct {
	DeliveryAddress string `json:"delivery_address"`
}

type DeliveryCreatedPayload struct {
	DeliveryID     string `json:"delivery_id"`
	Status         string `json:"status"`
	TrackingNumber string `json:"tracking_number"`
}

type DeliveryFailedPayload struct {
	Reason string `json:"reason"`
}

type OrderStatusChangedPayload struct {
	OldStatus string `json:"old_status"`
	NewStatus string `json:"new_status"`
}

type OrderFinalStatusPayload struct {
	Status string  `json:"status"`
	Reason *string `json:"reason"`
}

type NotificationSentPayload struct {
	NotificationID string `json:"notification_id"`
	TriggeredBy    string `json:"triggered_by"`
	Status         string `json:"status"`
}

type NotificationFailedPayload struct {
	NotificationID string `json:"notification_id"`
	TriggeredBy    string `json:"triggered_by"`
	Reason         string `json:"reason"`
}
