package events

const (
	TopicOrders        = "orders.events"
	TopicInventory     = "inventory.events"
	TopicPayments      = "payments.events"
	TopicDelivery      = "delivery.events"
	TopicNotifications = "notifications.events"
)

const (
	TypeOrderCreated          = "OrderCreated"
	TypePaymentRequested      = "PaymentRequested"
	TypeDeliveryRequested     = "DeliveryRequested"
	TypeReleaseStockRequested = "ReleaseStockRequested"
	TypeOrderStatusChanged    = "OrderStatusChanged"
	TypeOrderCompleted        = "OrderCompleted"
	TypeOrderFailed           = "OrderFailed"
	TypeOrderCancelled        = "OrderCancelled"

	TypeStockReserved          = "StockReserved"
	TypeStockReservationFailed = "StockReservationFailed"
	TypeStockReleased          = "StockReleased"

	TypePaymentCompleted = "PaymentCompleted"
	TypePaymentFailed    = "PaymentFailed"
	TypePaymentRefunded  = "PaymentRefunded"

	TypeDeliveryCreated   = "DeliveryCreated"
	TypeDeliveryFailed    = "DeliveryFailed"
	TypeDeliveryCancelled = "DeliveryCancelled"

	TypeNotificationSent   = "NotificationSent"
	TypeNotificationFailed = "NotificationFailed"
)

const (
	SourceAPIGateway          = "api-gateway"
	SourceOrderService        = "order-service"
	SourceInventoryService    = "inventory-service"
	SourcePaymentService      = "payment-service"
	SourceDeliveryService     = "delivery-service"
	SourceNotificationService = "notification-service"
	SourceReadModelService    = "read-model-service"
)
