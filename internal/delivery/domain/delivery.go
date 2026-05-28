package domain

const StatusCreated = "CREATED"

type Delivery struct {
	ID              string
	OrderID         string
	Status          string
	DeliveryAddress string
	TrackingNumber  string
	Reason          *string
}
