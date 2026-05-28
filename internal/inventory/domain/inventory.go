package domain

const (
	ReservationStatusReserved = "RESERVED"
	ReservationStatusReleased = "RELEASED"
)

type Item struct {
	SKU      string
	Quantity int
}

type FailedItem struct {
	SKU       string
	Requested int
	Available int
}

type Reservation struct {
	ID      string
	OrderID string
	Status  string
	Items   []Item
}
