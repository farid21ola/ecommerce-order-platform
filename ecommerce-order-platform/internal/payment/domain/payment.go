package domain

const (
	StatusCompleted = "COMPLETED"
	StatusFailed    = "FAILED"
)

type Payment struct {
	ID      string
	OrderID string
	Amount  int64
	Status  string
	Reason  *string
}
